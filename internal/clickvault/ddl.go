package clickvault

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/go-secure-stdlib/strutil"
	"github.com/hashicorp/vault/sdk/database/helper/dbutil"
)

// ErrEmptyCreationStatement is returned when NewUser is called without any
// creation_statements configured on the Vault role.
var ErrEmptyCreationStatement = dbutil.ErrEmptyCreationStatement

const (
	defaultRotateStatement = `ALTER USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}';`
	defaultDeleteStatement = `DROP USER IF EXISTS "{{username}}";`
)

// validateSubstitution rejects a username or password that could break out of
// the SQL quoting it is embedded in. ClickHouse DDL cannot be parameterized, so
// clickvault substitutes these values by literal text replacement; a value
// containing a quote, backslash, backtick or control character could terminate
// the surrounding single-quoted string literal ('...') or double-quoted
// identifier ("...") and inject arbitrary SQL.
//
// The value is rejected rather than escaped on purpose: escaping would depend on
// which quote style the operator chose in their statement template (which this
// code never sees resolved), whereas rejection is unambiguous and surfaces a
// misconfigured password policy loudly instead of silently emitting broken or
// injectable SQL. All other printable characters remain allowed, so strong
// passwords using ordinary symbols are unaffected. The offending value is never
// included in the error, so a bad password is not leaked.
func validateSubstitution(field, value string) error {
	for _, r := range value {
		if r == '\'' || r == '"' || r == '`' || r == '\\' || r < 0x20 || r == 0x7f {
			return fmt.Errorf(
				"%s contains a character that cannot be safely embedded in ClickHouse DDL "+
					"(quotes, backslash, backtick and control characters are not allowed); "+
					"tighten the password policy / username template to exclude them",
				field,
			)
		}
	}
	return nil
}

// buildStatements splits each raw (possibly multi-statement, semicolon
// separated) template string in rawStatements, substitutes the values in
// queryMap into each one, and — when cluster is non-empty — inserts an
// `ON CLUSTER '<cluster>'` clause at the grammatically correct position of any
// statement that doesn't already specify one.
//
// This is the single place cluster vs. single-node branching happens; callers
// in clickvault.go always call this function regardless of whether a cluster
// is configured.
func buildStatements(cluster string, rawStatements []string, queryMap map[string]string) ([]string, error) {
	if len(rawStatements) == 0 {
		return nil, errors.New("no statements to build")
	}

	var statements []string
	for _, raw := range rawStatements {
		for _, query := range strutil.ParseArbitraryStringSlice(raw, ";") {
			query = strings.TrimSpace(query)
			if query == "" {
				continue
			}

			query = dbutil.QueryHelper(query, queryMap)

			clustered, err := withCluster(query, cluster)
			if err != nil {
				return nil, err
			}

			statements = append(statements, clustered)
		}
	}

	if len(statements) == 0 {
		return nil, errors.New("no statements to build")
	}

	return statements, nil
}

// rotateStatements returns the statements to run to rotate a user's
// password, falling back to defaultRotateStatement when the operator hasn't
// configured rotation_statements on the static role.
func rotateStatements(cluster string, rawStatements []string, username, newPassword string) ([]string, error) {
	if err := validateSubstitution("username", username); err != nil {
		return nil, err
	}
	if err := validateSubstitution("password", newPassword); err != nil {
		return nil, err
	}

	if len(rawStatements) == 0 {
		rawStatements = []string{defaultRotateStatement}
	}

	queryMap := map[string]string{
		"name":     username,
		"username": username,
		"password": newPassword,
	}

	return buildStatements(cluster, rawStatements, queryMap)
}

// deleteStatements returns the statements to run to drop a user, falling
// back to defaultDeleteStatement when the operator hasn't configured
// revocation_statements on the role.
func deleteStatements(cluster string, rawStatements []string, username string) ([]string, error) {
	if err := validateSubstitution("username", username); err != nil {
		return nil, err
	}

	if len(rawStatements) == 0 {
		rawStatements = []string{defaultDeleteStatement}
	}

	queryMap := map[string]string{
		"name":     username,
		"username": username,
	}

	return buildStatements(cluster, rawStatements, queryMap)
}

// creationStatements returns the statements to run to create a new dynamic
// user. Unlike rotate/delete, there is no default — creation_statements must
// always be supplied by the operator on the Vault role, since only they know
// which database(s)/role(s) the new user should be granted.
func creationStatements(cluster string, rawStatements []string, username, password, expiration string) ([]string, error) {
	if len(rawStatements) == 0 {
		return nil, ErrEmptyCreationStatement
	}

	if err := validateSubstitution("username", username); err != nil {
		return nil, err
	}
	if err := validateSubstitution("password", password); err != nil {
		return nil, err
	}

	queryMap := map[string]string{
		"name":       username,
		"username":   username,
		"password":   password,
		"expiration": expiration,
	}

	return buildStatements(cluster, rawStatements, queryMap)
}

// withCluster injects an `ON CLUSTER '<cluster>'` clause into stmt at the
// position ClickHouse's grammar requires, unless cluster is empty or stmt
// already carries an ON CLUSTER clause. Single quotes in the cluster name are
// doubled to keep it inside the string literal.
//
// ClickHouse requires ON CLUSTER at a statement-specific position, NOT at the
// end of the statement:
//
//	GRANT  ON CLUSTER c ...           -- immediately after the verb
//	REVOKE ON CLUSTER c ...
//	CREATE USER name ON CLUSTER c ... -- immediately after the entity name
//	ALTER  USER name ON CLUSTER c ...
//	DROP   USER name ON CLUSTER c ...
//
// The previous implementation appended the clause at the very end of the
// statement, which is valid only for DROP USER and produces a syntax error for
// CREATE USER, ALTER USER and GRANT — the exact statements clickvault issues.
// Statements passed in here are already split on ";" by buildStatements, so
// there is no trailing semicolon to work around. For any statement whose shape
// clickvault cannot confidently place the clause in, an error is returned rather
// than emitting broken SQL, so the operator can add ON CLUSTER explicitly.
func withCluster(stmt, cluster string) (string, error) {
	if cluster == "" {
		return stmt, nil
	}
	if hasOnClusterClause(stmt) {
		return stmt, nil
	}

	clause := fmt.Sprintf("ON CLUSTER '%s'", strings.ReplaceAll(cluster, "'", "''"))

	tokens := tokenizeLeading(stmt)
	if len(tokens) == 0 {
		return "", errors.New("cannot inject ON CLUSTER into empty statement")
	}

	switch strings.ToUpper(tokens[0].text) {
	case "GRANT", "REVOKE":
		// ON CLUSTER goes immediately after the verb.
		return spliceAfter(stmt, tokens[0].end, clause), nil
	case "CREATE", "ALTER", "DROP":
		// ON CLUSTER goes immediately after the entity name.
		nameEnd, err := entityNameEnd(tokens)
		if err != nil {
			return "", fmt.Errorf("cannot inject ON CLUSTER into %q: %w", stmt, err)
		}
		return spliceAfter(stmt, nameEnd, clause), nil
	default:
		return "", fmt.Errorf(
			"cannot inject ON CLUSTER into %q: unrecognized statement; add an explicit ON CLUSTER clause",
			stmt,
		)
	}
}

// hasOnClusterClause reports whether stmt already contains an `ON CLUSTER`
// clause, matched case-insensitively on the two adjacent keywords regardless of
// the whitespace between them.
func hasOnClusterClause(stmt string) bool {
	upper := strings.ToUpper(stmt)
	i := 0
	for {
		idx := strings.Index(upper[i:], "ON")
		if idx < 0 {
			return false
		}
		j := i + idx
		rest := strings.TrimLeft(upper[j+2:], " \t\n\r")
		if strings.HasPrefix(rest, "CLUSTER") {
			return true
		}
		i = j + 2
	}
}

// token is a lexical token of a DDL statement together with the byte offset of
// its end in the original string, so a clause can be spliced in after it.
type token struct {
	text string
	end  int
}

// tokenizeLeading splits the leading portion of stmt into whitespace-separated
// tokens, treating a double-quoted ("...", with "" escaping) or backtick-quoted
// (`...`) identifier as a single token even if it contains spaces. It stops once
// it has collected a handful of tokens — enough to locate an entity name, which
// is all withCluster needs; it never has to tokenize the whole statement.
func tokenizeLeading(stmt string) []token {
	var tokens []token
	i := 0
	n := len(stmt)
	for i < n && len(tokens) < 8 {
		// Skip whitespace.
		for i < n && isSpace(stmt[i]) {
			i++
		}
		if i >= n {
			break
		}
		start := i
		switch stmt[i] {
		case '"', '`':
			quote := stmt[i]
			i++
			for i < n {
				if stmt[i] == quote {
					// A doubled quote ("") is an escaped quote, not the end.
					if quote == '"' && i+1 < n && stmt[i+1] == '"' {
						i += 2
						continue
					}
					i++ // consume closing quote
					break
				}
				i++
			}
		default:
			for i < n && !isSpace(stmt[i]) {
				i++
			}
		}
		tokens = append(tokens, token{text: stmt[start:i], end: i})
	}
	return tokens
}

// nameLeadingKeywords are the tokens that can appear between the verb and the
// entity name in the CREATE/ALTER/DROP USER|ROLE statements clickvault issues:
// the entity keyword itself plus the OR REPLACE / IF [NOT] EXISTS modifiers,
// which may appear either side of the entity keyword. Any token that is not one
// of these is taken to be the entity name — safe here because clickvault always
// quotes the name ("..."), so it can never collide with a bare keyword.
var nameLeadingKeywords = map[string]bool{
	"USER": true, "ROLE": true,
	"OR": true, "REPLACE": true,
	"IF": true, "NOT": true, "EXISTS": true,
}

// entityNameEnd returns the byte offset just past the entity name in a
// CREATE/ALTER/DROP statement, skipping the verb and any entity keyword /
// OR REPLACE / IF [NOT] EXISTS modifiers that precede the name. It errors on
// shapes clickvault does not issue (e.g. a comma-separated list of names), so
// withCluster can refuse rather than mangle them.
func entityNameEnd(tokens []token) (int, error) {
	// tokens[0] is the verb (CREATE/ALTER/DROP); the name follows the leading
	// keywords.
	for i := 1; i < len(tokens); i++ {
		if nameLeadingKeywords[strings.ToUpper(tokens[i].text)] {
			continue
		}
		name := tokens[i].text
		if name == "" || strings.Contains(name, ",") {
			return 0, errors.New("multiple comma-separated names are not supported; add an explicit ON CLUSTER clause")
		}
		return tokens[i].end, nil
	}
	return 0, errors.New("could not locate entity name")
}

// spliceAfter inserts clause into stmt immediately after byte offset pos,
// separated by single spaces and preserving whatever followed.
func spliceAfter(stmt string, pos int, clause string) string {
	head := stmt[:pos]
	tail := strings.TrimLeft(stmt[pos:], " \t\n\r")
	if tail == "" {
		return head + " " + clause
	}
	return head + " " + clause + " " + tail
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}
