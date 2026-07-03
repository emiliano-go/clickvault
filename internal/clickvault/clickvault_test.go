package clickvault

import (
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	dbplugin "github.com/hashicorp/vault/sdk/database/dbplugin/v5"
	"github.com/hashicorp/vault/sdk/helper/template"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestPlugin(t *testing.T, cluster string) (*ClickvaultPlugin, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	up, err := template.NewTemplate(template.Template(DefaultUsernameTemplate))
	require.NoError(t, err)

	return &ClickvaultPlugin{
		db:               db,
		cfg:              config{Cluster: cluster, Username: "admin", Password: "adminpw"},
		usernameProducer: up,
	}, mock
}

func TestType(t *testing.T) {
	p := &ClickvaultPlugin{}
	got, err := p.Type()
	require.NoError(t, err)
	assert.Equal(t, "clickvault", got)
}

func TestInitialize_Validation(t *testing.T) {
	tests := map[string]struct {
		config    map[string]interface{}
		expectErr string
	}{
		"missing connection_url": {
			config:    map[string]interface{}{"username": "admin", "password": "pw"},
			expectErr: "connection_url is required",
		},
		"missing username": {
			config:    map[string]interface{}{"connection_url": "clickhouse://host:9000", "password": "pw"},
			expectErr: "username is required",
		},
		"missing password": {
			config:    map[string]interface{}{"connection_url": "clickhouse://host:9000", "username": "admin"},
			expectErr: "password is required",
		},
		"invalid username template": {
			config: map[string]interface{}{
				"connection_url":    "clickhouse://host:9000",
				"username":          "admin",
				"password":          "pw",
				"username_template": "{{.FieldThatDoesNotExist}}",
			},
			expectErr: "invalid username template",
		},
		"malformed username template": {
			config: map[string]interface{}{
				"connection_url":    "clickhouse://host:9000",
				"username":          "admin",
				"password":          "pw",
				"username_template": "{{ .DisplayName",
			},
			expectErr: "invalid username template",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			p := &ClickvaultPlugin{}
			_, err := p.Initialize(t.Context(), dbplugin.InitializeRequest{Config: tt.config})
			require.Error(t, err)
			assert.ErrorContains(t, err, tt.expectErr)

			p.mu.RLock()
			dbSet := p.db != nil
			p.mu.RUnlock()
			assert.False(t, dbSet, "db should not be set on a failed Initialize")
		})
	}
}

func TestInitialize_WithoutVerifyConnection(t *testing.T) {
	p := &ClickvaultPlugin{}

	resp, err := p.Initialize(t.Context(), dbplugin.InitializeRequest{
		Config: map[string]interface{}{
			"connection_url": "clickhouse://127.0.0.1:1",
			"username":       "admin",
			"password":       "adminpw",
		},
		VerifyConnection: false,
	})
	require.NoError(t, err)
	assert.Equal(t, "clickhouse://127.0.0.1:1", resp.Config["connection_url"])

	p.mu.RLock()
	dbSet := p.db != nil
	p.mu.RUnlock()
	assert.True(t, dbSet)
	assert.Equal(t, map[string]string{"adminpw": "[password]"}, p.secretValues())
}

func TestNewUser_EmptyStatements(t *testing.T) {
	p, _ := newTestPlugin(t, "")

	_, err := p.NewUser(t.Context(), dbplugin.NewUserRequest{})
	require.ErrorIs(t, err, ErrEmptyCreationStatement)
}

func TestNewUser_NotInitialized(t *testing.T) {
	p := &ClickvaultPlugin{}

	_, err := p.NewUser(t.Context(), dbplugin.NewUserRequest{
		Statements: dbplugin.Statements{Commands: []string{`CREATE USER "{{username}}"`}},
	})
	require.ErrorContains(t, err, "not initialized")
}

func TestNewUser_SingleNode(t *testing.T) {
	p, mock := newTestPlugin(t, "")

	mock.ExpectExec(`CREATE USER "v-token-[A-Za-z0-9]+-[0-9]+" IDENTIFIED WITH sha256_password BY 'pw'`).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`GRANT analytics ON default\.\* TO "v-token-[A-Za-z0-9]+-[0-9]+"`).WillReturnResult(sqlmock.NewResult(0, 0))

	resp, err := p.NewUser(t.Context(), dbplugin.NewUserRequest{
		UsernameConfig: dbplugin.UsernameMetadata{DisplayName: "token", RoleName: "testrole"},
		Statements: dbplugin.Statements{
			Commands: []string{`CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}'; GRANT analytics ON default.* TO "{{username}}";`},
		},
		Password:   "pw",
		Expiration: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	assert.Regexp(t, `^v-token-`, resp.Username)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestNewUser_ClusterAppendsOnCluster(t *testing.T) {
	p, mock := newTestPlugin(t, "prod")

	mock.ExpectExec(`CREATE USER "bob" ON CLUSTER 'prod' IDENTIFIED WITH sha256_password BY 'pw'`).WillReturnResult(sqlmock.NewResult(0, 0))

	_, err := p.NewUser(t.Context(), dbplugin.NewUserRequest{
		UsernameConfig: dbplugin.UsernameMetadata{DisplayName: "token", RoleName: "testrole"},
		Statements: dbplugin.Statements{
			Commands: []string{`CREATE USER "bob" IDENTIFIED WITH sha256_password BY '{{password}}';`},
		},
		Password:   "pw",
		Expiration: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

// TestNewUser_SanitizesPasswordAndCleansUpOnPartialFailure covers a
// CREATE USER; GRANT ...; batch where CREATE USER succeeds but GRANT fails
// (a real ClickHouse syntax error can echo back a fragment of the failing
// statement, including a literal password). NewUser must:
//  1. not leak the plaintext password into the error it returns, and
//  2. best-effort DROP the now-orphaned user it just created, since Vault
//     will never record a lease for a NewUser call that returned an error.
func TestNewUser_SanitizesPasswordAndCleansUpOnPartialFailure(t *testing.T) {
	p, mock := newTestPlugin(t, "")

	const usernamePattern = `v-token-[A-Za-z0-9]+-[0-9]+`

	mock.ExpectExec(`CREATE USER "` + usernamePattern + `" IDENTIFIED WITH sha256_password BY 'sekret-pw'`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`GRANT analytics ON default\.\* TO "` + usernamePattern + `"`).
		WillReturnError(errors.New(`code: 62, message: Syntax error: failed at position 12 ('sekret-pw')`))
	mock.ExpectExec(`DROP USER IF EXISTS "` + usernamePattern + `"`).
		WillReturnResult(sqlmock.NewResult(0, 0))

	_, err := p.NewUser(t.Context(), dbplugin.NewUserRequest{
		UsernameConfig: dbplugin.UsernameMetadata{DisplayName: "token", RoleName: "testrole"},
		Statements: dbplugin.Statements{
			Commands: []string{
				`CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}'; GRANT analytics ON default.* TO "{{username}}";`,
			},
		},
		Password:   "sekret-pw",
		Expiration: time.Now().Add(time.Hour),
	})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "sekret-pw", "plaintext password must not leak into the returned error")
	assert.Contains(t, err.Error(), "[password]")
	require.NoError(t, mock.ExpectationsWereMet(), "DROP USER IF EXISTS should have been issued to clean up the orphaned user")
}

// TestNewUser_ReportsCleanupFailureAlongsideOriginalError covers the case
// where the best-effort cleanup DROP also fails: both the original failure
// and the cleanup failure must be visible in the returned error so an
// operator knows a live, untracked ClickHouse user may still exist.
func TestNewUser_ReportsCleanupFailureAlongsideOriginalError(t *testing.T) {
	p, mock := newTestPlugin(t, "")

	const usernamePattern = `v-token-[A-Za-z0-9]+-[0-9]+`

	mock.ExpectExec(`CREATE USER "` + usernamePattern + `" IDENTIFIED WITH sha256_password BY 'pw'`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`GRANT analytics ON default\.\* TO "` + usernamePattern + `"`).
		WillReturnError(errors.New("grant failed"))
	mock.ExpectExec(`DROP USER IF EXISTS "` + usernamePattern + `"`).
		WillReturnError(errors.New("connection lost"))

	_, err := p.NewUser(t.Context(), dbplugin.NewUserRequest{
		UsernameConfig: dbplugin.UsernameMetadata{DisplayName: "token", RoleName: "testrole"},
		Statements: dbplugin.Statements{
			Commands: []string{
				`CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}'; GRANT analytics ON default.* TO "{{username}}";`,
			},
		},
		Password:   "pw",
		Expiration: time.Now().Add(time.Hour),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "grant failed")
	assert.Contains(t, err.Error(), "connection lost")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateUser_NoChangeRequested(t *testing.T) {
	p, _ := newTestPlugin(t, "")

	_, err := p.UpdateUser(t.Context(), dbplugin.UpdateUserRequest{Username: "bob"})
	require.ErrorContains(t, err, "no change requested")
}

func TestUpdateUser_NotInitialized(t *testing.T) {
	p := &ClickvaultPlugin{}

	_, err := p.UpdateUser(t.Context(), dbplugin.UpdateUserRequest{
		Username: "bob",
		Password: &dbplugin.ChangePassword{NewPassword: "newpw"},
	})
	require.ErrorContains(t, err, "not initialized")
}

func TestUpdateUser_PasswordRotation(t *testing.T) {
	p, mock := newTestPlugin(t, "")

	mock.ExpectExec(`ALTER USER "bob" IDENTIFIED WITH sha256_password BY 'newpw'`).WillReturnResult(sqlmock.NewResult(0, 0))

	_, err := p.UpdateUser(t.Context(), dbplugin.UpdateUserRequest{
		Username: "bob",
		Password: &dbplugin.ChangePassword{NewPassword: "newpw"},
	})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteUser_NotInitialized(t *testing.T) {
	p := &ClickvaultPlugin{}

	_, err := p.DeleteUser(t.Context(), dbplugin.DeleteUserRequest{Username: "bob"})
	require.ErrorContains(t, err, "not initialized")
}

func TestDeleteUser_Idempotent(t *testing.T) {
	p, mock := newTestPlugin(t, "")

	mock.ExpectExec(`DROP USER IF EXISTS "bob"`).WillReturnResult(sqlmock.NewResult(0, 0))

	_, err := p.DeleteUser(t.Context(), dbplugin.DeleteUserRequest{Username: "bob"})
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestClose(t *testing.T) {
	p, mock := newTestPlugin(t, "")
	mock.ExpectClose()

	require.NoError(t, p.Close())
	// Close is idempotent.
	require.NoError(t, p.Close())

	p.mu.RLock()
	defer p.mu.RUnlock()
	assert.Nil(t, p.db)
}

// verifyConnectionQueryPattern matches the combined "direct grant OR grant
// via a role assigned to the user" query verifyConnection issues. It takes
// the same username argument twice (once per branch of the UNION ALL).
const verifyConnectionQueryPattern = `SELECT access_type FROM system\.grants WHERE user_name = \?\s*` +
	`UNION ALL\s*` +
	`SELECT g\.access_type FROM system\.grants g\s*` +
	`WHERE g\.role_name IN \(SELECT granted_role_name FROM system\.role_grants WHERE user_name = \?\)`

func TestVerifyConnection(t *testing.T) {
	tests := map[string]struct {
		rows      *sqlmock.Rows
		expectErr bool
	}{
		"has direct ACCESS MANAGEMENT grant": {
			rows: sqlmock.NewRows([]string{"access_type"}).AddRow("ACCESS MANAGEMENT"),
		},
		"has direct ALL grant": {
			rows: sqlmock.NewRows([]string{"access_type"}).AddRow("ALL"),
		},
		"has ACCESS MANAGEMENT via a granted role": {
			// Simulates the UNION's second branch matching: no direct grant
			// row, but a row surfaces through system.role_grants -> the
			// role's own system.grants entry. sqlmock doesn't distinguish
			// which branch of the UNION produced a row, only that the query
			// (asserted via the regex below) actually joins role_grants and
			// that a resulting row is treated as sufficient.
			rows: sqlmock.NewRows([]string{"access_type"}).AddRow("ACCESS MANAGEMENT"),
		},
		"missing privilege": {
			rows:      sqlmock.NewRows([]string{"access_type"}).AddRow("SELECT"),
			expectErr: true,
		},
		"no grants at all": {
			rows:      sqlmock.NewRows([]string{"access_type"}),
			expectErr: true,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
			require.NoError(t, err)
			defer db.Close()

			mock.ExpectPing()
			mock.ExpectQuery(verifyConnectionQueryPattern).
				WithArgs("admin", "admin").
				WillReturnRows(tt.rows)

			err = verifyConnection(t.Context(), db, "admin")
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestParseAddr(t *testing.T) {
	tests := map[string]struct {
		connectionURL string
		want          string
		expectErr     bool
	}{
		"scheme prefixed":   {connectionURL: "clickhouse://host:9000", want: "host:9000"},
		"no scheme is error": {connectionURL: "host:9000", expectErr: true},
		"tcp scheme":        {connectionURL: "tcp://host:9440", want: "host:9440"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseAddr(tt.connectionURL)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
