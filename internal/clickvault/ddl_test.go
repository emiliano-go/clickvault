package clickvault

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithCluster(t *testing.T) {
	tests := map[string]struct {
		stmt    string
		cluster string
		want    string
	}{
		"no cluster is a no-op": {
			stmt:    `CREATE USER "bob" IDENTIFIED WITH sha256_password BY 'pw'`,
			cluster: "",
			want:    `CREATE USER "bob" IDENTIFIED WITH sha256_password BY 'pw'`,
		},
		"cluster appends ON CLUSTER before trailing semicolon": {
			stmt:    `CREATE USER "bob" IDENTIFIED WITH sha256_password BY 'pw'`,
			cluster: "prod",
			want:    `CREATE USER "bob" IDENTIFIED WITH sha256_password BY 'pw' ON CLUSTER 'prod'`,
		},
		"cluster appends ON CLUSTER when statement has no trailing semicolon": {
			stmt:    `DROP USER IF EXISTS "bob"`,
			cluster: "prod",
			want:    `DROP USER IF EXISTS "bob" ON CLUSTER 'prod'`,
		},
		"existing ON CLUSTER clause is left untouched": {
			stmt:    `GRANT analytics ON default.* TO "bob" ON CLUSTER 'prod'`,
			cluster: "prod",
			want:    `GRANT analytics ON default.* TO "bob" ON CLUSTER 'prod'`,
		},
		"existing ON CLUSTER clause is case-insensitively detected": {
			stmt:    `GRANT analytics ON default.* TO "bob" on cluster 'prod'`,
			cluster: "prod",
			want:    `GRANT analytics ON default.* TO "bob" on cluster 'prod'`,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tt.want, withCluster(tt.stmt, tt.cluster))
		})
	}
}

func TestCreationStatements(t *testing.T) {
	tests := map[string]struct {
		cluster       string
		rawStatements []string
		username      string
		password      string
		expiration    string
		want          []string
		expectErr     bool
	}{
		"empty statements is an error": {
			rawStatements: nil,
			expectErr:     true,
		},
		"single-node create + grant": {
			rawStatements: []string{
				`CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}'; GRANT analytics ON default.* TO "{{username}}"`,
			},
			username: "v-token-role-abcd1234",
			password: "sekret",
			want: []string{
				`CREATE USER "v-token-role-abcd1234" IDENTIFIED WITH sha256_password BY 'sekret'`,
				`GRANT analytics ON default.* TO "v-token-role-abcd1234"`,
			},
		},
		"cluster create + grant appends ON CLUSTER to each statement": {
			cluster: "prod",
			rawStatements: []string{
				`CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}'; GRANT analytics ON default.* TO "{{username}}"`,
			},
			username: "v-token-role-abcd1234",
			password: "sekret",
			want: []string{
				`CREATE USER "v-token-role-abcd1234" IDENTIFIED WITH sha256_password BY 'sekret' ON CLUSTER 'prod'`,
				`GRANT analytics ON default.* TO "v-token-role-abcd1234" ON CLUSTER 'prod'`,
			},
		},
		"blank statements and stray semicolons are skipped": {
			rawStatements: []string{
				`  ; CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}';   ;  `,
			},
			username: "bob",
			password: "pw",
			want: []string{
				`CREATE USER "bob" IDENTIFIED WITH sha256_password BY 'pw'`,
			},
		},
		"name placeholder is also substituted": {
			rawStatements: []string{`CREATE USER "{{name}}" IDENTIFIED WITH sha256_password BY '{{password}}'`},
			username:      "bob",
			password:      "pw",
			want:          []string{`CREATE USER "bob" IDENTIFIED WITH sha256_password BY 'pw'`},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := creationStatements(tt.cluster, tt.rawStatements, tt.username, tt.password, tt.expiration)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRotateStatements(t *testing.T) {
	tests := map[string]struct {
		cluster       string
		rawStatements []string
		username      string
		newPassword   string
		want          []string
	}{
		"default statement, single-node": {
			username:    "bob",
			newPassword: "newpw",
			want: []string{
				`ALTER USER "bob" IDENTIFIED WITH sha256_password BY 'newpw'`,
			},
		},
		"default statement, cluster": {
			cluster:     "prod",
			username:    "bob",
			newPassword: "newpw",
			want: []string{
				`ALTER USER "bob" IDENTIFIED WITH sha256_password BY 'newpw' ON CLUSTER 'prod'`,
			},
		},
		"custom rotation statement overrides default": {
			rawStatements: []string{`ALTER USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}'`},
			username:      "bob",
			newPassword:   "newpw",
			want: []string{
				`ALTER USER "bob" IDENTIFIED WITH sha256_password BY 'newpw'`,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := rotateStatements(tt.cluster, tt.rawStatements, tt.username, tt.newPassword)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDeleteStatements(t *testing.T) {
	tests := map[string]struct {
		cluster       string
		rawStatements []string
		username      string
		want          []string
	}{
		"default statement, single-node": {
			username: "bob",
			want: []string{
				`DROP USER IF EXISTS "bob"`,
			},
		},
		"default statement, cluster": {
			cluster:  "prod",
			username: "bob",
			want: []string{
				`DROP USER IF EXISTS "bob" ON CLUSTER 'prod'`,
			},
		},
		"custom revocation statement overrides default": {
			rawStatements: []string{`DROP USER IF EXISTS "{{name}}"`},
			username:      "bob",
			want: []string{
				`DROP USER IF EXISTS "bob"`,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := deleteStatements(tt.cluster, tt.rawStatements, tt.username)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
