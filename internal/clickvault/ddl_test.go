package clickvault

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithCluster(t *testing.T) {
	tests := map[string]struct {
		stmt      string
		cluster   string
		want      string
		expectErr bool
	}{
		"no cluster is a no-op": {
			stmt:    `CREATE USER "bob" IDENTIFIED WITH sha256_password BY 'pw'`,
			cluster: "",
			want:    `CREATE USER "bob" IDENTIFIED WITH sha256_password BY 'pw'`,
		},
		"CREATE USER inserts ON CLUSTER after the name, not at the end": {
			stmt:    `CREATE USER "bob" IDENTIFIED WITH sha256_password BY 'pw'`,
			cluster: "prod",
			want:    `CREATE USER "bob" ON CLUSTER 'prod' IDENTIFIED WITH sha256_password BY 'pw'`,
		},
		"CREATE USER IF NOT EXISTS inserts after the name": {
			stmt:    `CREATE USER IF NOT EXISTS "bob" IDENTIFIED WITH sha256_password BY 'pw'`,
			cluster: "prod",
			want:    `CREATE USER IF NOT EXISTS "bob" ON CLUSTER 'prod' IDENTIFIED WITH sha256_password BY 'pw'`,
		},
		"CREATE OR REPLACE USER inserts after the name": {
			stmt:    `CREATE OR REPLACE USER "bob" IDENTIFIED WITH sha256_password BY 'pw'`,
			cluster: "prod",
			want:    `CREATE OR REPLACE USER "bob" ON CLUSTER 'prod' IDENTIFIED WITH sha256_password BY 'pw'`,
		},
		"ALTER USER inserts ON CLUSTER after the name": {
			stmt:    `ALTER USER "bob" IDENTIFIED WITH sha256_password BY 'pw'`,
			cluster: "prod",
			want:    `ALTER USER "bob" ON CLUSTER 'prod' IDENTIFIED WITH sha256_password BY 'pw'`,
		},
		"DROP USER inserts ON CLUSTER after the trailing name": {
			stmt:    `DROP USER IF EXISTS "bob"`,
			cluster: "prod",
			want:    `DROP USER IF EXISTS "bob" ON CLUSTER 'prod'`,
		},
		"GRANT inserts ON CLUSTER immediately after the verb": {
			stmt:    `GRANT analytics ON default.* TO "bob"`,
			cluster: "prod",
			want:    `GRANT ON CLUSTER 'prod' analytics ON default.* TO "bob"`,
		},
		"REVOKE inserts ON CLUSTER immediately after the verb": {
			stmt:    `REVOKE analytics ON default.* FROM "bob"`,
			cluster: "prod",
			want:    `REVOKE ON CLUSTER 'prod' analytics ON default.* FROM "bob"`,
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
		"a bare ON (grant target) is not mistaken for ON CLUSTER": {
			stmt:    `GRANT SELECT ON default.* TO "bob"`,
			cluster: "prod",
			want:    `GRANT ON CLUSTER 'prod' SELECT ON default.* TO "bob"`,
		},
		"quotes in the cluster name are doubled": {
			stmt:    `DROP USER IF EXISTS "bob"`,
			cluster: "pr'od",
			want:    `DROP USER IF EXISTS "bob" ON CLUSTER 'pr''od'`,
		},
		"unrecognized statement errors instead of emitting broken SQL": {
			stmt:      `SELECT 1`,
			cluster:   "prod",
			expectErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := withCluster(tt.stmt, tt.cluster)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateSubstitution(t *testing.T) {
	valid := []string{
		"simplepassword",
		"Str0ng-P@ssw0rd_1234",
		"v-token-role-abcd1234",
		"has spaces and symbols !#$%^&*()",
	}
	for _, v := range valid {
		require.NoError(t, validateSubstitution("password", v), "expected %q to be valid", v)
	}

	invalid := map[string]string{
		"single quote breaks the literal": "pw' OR '1'='1",
		"double quote breaks identifier":  `bo"b`,
		"backslash":                       `pw\x`,
		"backtick":                        "pw`x",
		"newline":                         "pw\nDROP USER x",
		"null byte":                       "pw\x00",
	}
	for name, v := range invalid {
		t.Run(name, func(t *testing.T) {
			err := validateSubstitution("password", v)
			require.Error(t, err)
			// The offending value must never appear in the error.
			assert.NotContains(t, err.Error(), v)
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
		"cluster create + grant inserts ON CLUSTER at each grammatical position": {
			cluster: "prod",
			rawStatements: []string{
				`CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}'; GRANT analytics ON default.* TO "{{username}}"`,
			},
			username: "v-token-role-abcd1234",
			password: "sekret",
			want: []string{
				`CREATE USER "v-token-role-abcd1234" ON CLUSTER 'prod' IDENTIFIED WITH sha256_password BY 'sekret'`,
				`GRANT ON CLUSTER 'prod' analytics ON default.* TO "v-token-role-abcd1234"`,
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
		"injection-unsafe password is rejected": {
			rawStatements: []string{`CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}'`},
			username:      "bob",
			password:      "pw'; DROP USER \"bob\"; --",
			expectErr:     true,
		},
		"injection-unsafe username is rejected": {
			rawStatements: []string{`CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}'`},
			username:      `bo"b`,
			password:      "pw",
			expectErr:     true,
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
		expectErr     bool
	}{
		"default statement, single-node": {
			username:    "bob",
			newPassword: "newpw",
			want: []string{
				`ALTER USER "bob" IDENTIFIED WITH sha256_password BY 'newpw'`,
			},
		},
		"default statement, cluster inserts ON CLUSTER after the name": {
			cluster:     "prod",
			username:    "bob",
			newPassword: "newpw",
			want: []string{
				`ALTER USER "bob" ON CLUSTER 'prod' IDENTIFIED WITH sha256_password BY 'newpw'`,
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
		"injection-unsafe new password is rejected": {
			username:    "bob",
			newPassword: "np' OR '1'='1",
			expectErr:   true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := rotateStatements(tt.cluster, tt.rawStatements, tt.username, tt.newPassword)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
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
		expectErr     bool
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
		"injection-unsafe username is rejected": {
			username:  `bo"b; DROP USER "root"`,
			expectErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := deleteStatements(tt.cluster, tt.rawStatements, tt.username)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
