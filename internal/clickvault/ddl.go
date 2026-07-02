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

// buildStatements splits each raw (possibly multi-statement, semicolon
// separated) template string in rawStatements, substitutes the values in
// queryMap into each one, and — when cluster is non-empty — appends
// `ON CLUSTER '<cluster>'` to any statement that doesn't already specify one.
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
			query = withCluster(query, cluster)

			statements = append(statements, query)
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

	queryMap := map[string]string{
		"name":       username,
		"username":   username,
		"password":   password,
		"expiration": expiration,
	}

	return buildStatements(cluster, rawStatements, queryMap)
}

// withCluster appends `ON CLUSTER '<cluster>'` to stmt unless cluster is
// empty or stmt already contains an ON CLUSTER clause. Single quotes in the
// cluster name are escaped to prevent SQL injection. Statements passed in are
// already split on ";" by buildStatements, so no trailing punctuation is added
// or removed here.
func withCluster(stmt, cluster string) string {
	if cluster == "" {
		return stmt
	}
	if strings.Contains(strings.ToUpper(stmt), "ON CLUSTER") {
		return stmt
	}

	safeCluster := strings.ReplaceAll(cluster, "'", "''")
	return fmt.Sprintf("%s ON CLUSTER '%s'", stmt, safeCluster)
}
