//go:build integration

// Package tests contains integration tests that exercise clickvault against
// a real ClickHouse server. They're gated behind the "integration" build tag
// since they require Docker:
//
//	go test -tags=integration ./tests/...
package tests

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/hashicorp/vault/sdk/database/dbplugin/v5"
	"github.com/hashicorp/vault/sdk/helper/docker"
	"github.com/hashicorp/vault/sdk/helper/template"
	"github.com/stretchr/testify/require"

	"github.com/emiliano-go/clickvault/internal/clickvault"
)

const (
	adminUser     = "admin"
	adminPassword = "admin_password"
)

// startClickHouse launches the same ClickHouse image/config described in
// testdata/docker-compose.yml as a throwaway container and returns a cleanup
// func plus the reachable host:port.
func startClickHouse(t *testing.T) (func(), string) {
	t.Helper()

	runner, err := docker.NewServiceRunner(docker.RunOptions{
		ImageRepo:     "clickhouse/clickhouse-server",
		ImageTag:      "24.8-alpine",
		ContainerName: "clickvault-it",
		Env: []string{
			"CLICKHOUSE_USER=" + adminUser,
			"CLICKHOUSE_PASSWORD=" + adminPassword,
			"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT=1",
		},
		Ports:           []string{"9000/tcp"},
		DoNotAutoRemove: false,
	})
	require.NoError(t, err)

	svc, err := runner.StartService(context.Background(), func(ctx context.Context, host string, port int) (docker.ServiceConfig, error) {
		hostPort := docker.NewServiceHostPort(host, port)

		db := clickhouse.OpenDB(&clickhouse.Options{
			Addr: []string{hostPort.Address()},
			Auth: clickhouse.Auth{Username: adminUser, Password: adminPassword},
		})
		defer db.Close()

		if err := db.PingContext(ctx); err != nil {
			return nil, err
		}

		return hostPort, nil
	})
	require.NoError(t, err)

	hostPort := svc.Config.(*docker.ServiceHostPort)

	return svc.Cleanup, hostPort.Address()
}

func connectAs(t *testing.T, addr, username, password string) *sql.DB {
	t.Helper()

	db := clickhouse.OpenDB(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{Username: username, Password: password},
	})
	t.Cleanup(func() { db.Close() })

	return db
}

func requireCanConnect(t *testing.T, addr, username, password string) {
	t.Helper()

	db := connectAs(t, addr, username, password)
	require.NoError(t, db.PingContext(context.Background()))
}

func requireCannotConnect(t *testing.T, addr, username, password string) {
	t.Helper()

	db := connectAs(t, addr, username, password)
	require.Error(t, db.PingContext(context.Background()))
}

// TestLifecycle drives Initialize -> NewUser -> verify -> DeleteUser ->
// verify gone -> UpdateUser (static rotation) -> verify against a real
// ClickHouse server.
func TestLifecycle(t *testing.T) {
	cleanup, addr := startClickHouse(t)
	defer cleanup()

	dbRaw, err := clickvault.New()
	require.NoError(t, err)
	db := dbRaw.(dbplugin.Database)
	defer db.Close()

	initResp, err := db.Initialize(context.Background(), dbplugin.InitializeRequest{
		Config: map[string]interface{}{
			"connection_url": fmt.Sprintf("clickhouse://%s", addr),
			"username":       adminUser,
			"password":       adminPassword,
		},
		VerifyConnection: true,
	})
	require.NoError(t, err)
	require.Equal(t, adminUser, initResp.Config["username"])

	t.Run("NewUser and DeleteUser", func(t *testing.T) {
		newUserResp, err := db.NewUser(context.Background(), dbplugin.NewUserRequest{
			UsernameConfig: dbplugin.UsernameMetadata{DisplayName: "token", RoleName: "itrole"},
			Statements: dbplugin.Statements{
				Commands: []string{
					`CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}';`,
				},
			},
			Password:   "sUp3rS3cret!",
			Expiration: time.Now().Add(time.Hour),
		})
		require.NoError(t, err)
		// DefaultUsernameTemplate intentionally omits RoleName (avoids leaking
		// internal Vault structure into ClickHouse logs), so the generated
		// name only carries DisplayName, not the "itrole" RoleName above.
		require.Regexp(t, `^v-token-`, newUserResp.Username)

		requireCanConnect(t, addr, newUserResp.Username, "sUp3rS3cret!")

		_, err = db.DeleteUser(context.Background(), dbplugin.DeleteUserRequest{Username: newUserResp.Username})
		require.NoError(t, err)

		requireCannotConnect(t, addr, newUserResp.Username, "sUp3rS3cret!")

		// DeleteUser must be idempotent: deleting an already-gone user is not an error.
		_, err = db.DeleteUser(context.Background(), dbplugin.DeleteUserRequest{Username: newUserResp.Username})
		require.NoError(t, err)
	})

	t.Run("NewUser cleans up orphaned user when a later statement fails", func(t *testing.T) {
		// Generate the expected username ourselves so we can assert
		// the orphan cleanup actually dropped it from ClickHouse.
		up, err := template.NewTemplate(template.Template(clickvault.DefaultUsernameTemplate))
		require.NoError(t, err)
		expectedUser, err := up.Generate(dbplugin.UsernameMetadata{DisplayName: "orphan", RoleName: "orphan"})
		require.NoError(t, err)

		_, err = db.NewUser(context.Background(), dbplugin.NewUserRequest{
			UsernameConfig: dbplugin.UsernameMetadata{DisplayName: "orphan", RoleName: "orphan"},
			Statements: dbplugin.Statements{
				Commands: []string{
					`CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}'; GRANT nonexistent_role_xyz ON default.* TO "{{username}}";`,
				},
			},
			Password:   "OrphanPassw0rd!",
			Expiration: time.Now().Add(time.Hour),
		})
		require.Error(t, err, "GRANT to a nonexistent role must fail")

		// If the best-effort cleanup ran, the user no longer exists.
		requireCannotConnect(t, addr, expectedUser, "OrphanPassw0rd!")
	})

	t.Run("Initialize accepts an admin whose ACCESS MANAGEMENT comes from a role", func(t *testing.T) {
		ctx := context.Background()
		rawDB := connectAs(t, addr, adminUser, adminPassword)

		for _, stmt := range []string{
			`CREATE ROLE IF NOT EXISTS clickvault_it_admin_role`,
			`GRANT ACCESS MANAGEMENT ON *.* TO clickvault_it_admin_role`,
			`GRANT SELECT ON system.grants TO clickvault_it_admin_role`,
			`GRANT SELECT ON system.role_grants TO clickvault_it_admin_role`,
			`CREATE USER IF NOT EXISTS clickvault_role_admin_it IDENTIFIED WITH sha256_password BY 'RoleAdminPassw0rd!'`,
			`GRANT clickvault_it_admin_role TO clickvault_role_admin_it`,
			`SET DEFAULT ROLE clickvault_it_admin_role TO clickvault_role_admin_it`,
		} {
			_, err := rawDB.ExecContext(ctx, stmt)
			require.NoError(t, err, "setup statement failed: %s", stmt)
		}

		roleAdminDBRaw, err := clickvault.New()
		require.NoError(t, err)
		roleAdminDB := roleAdminDBRaw.(dbplugin.Database)
		defer roleAdminDB.Close()

		_, err = roleAdminDB.Initialize(ctx, dbplugin.InitializeRequest{
			Config: map[string]interface{}{
				"connection_url": fmt.Sprintf("clickhouse://%s", addr),
				"username":       "clickvault_role_admin_it",
				"password":       "RoleAdminPassw0rd!",
			},
			VerifyConnection: true,
		})
		require.NoError(t, err, "an admin with ACCESS MANAGEMENT granted only via a role must pass verify_connection")
	})

	t.Run("UpdateUser rotates a static user's password", func(t *testing.T) {
		staticUser := "clickvault_static_it"

		_, err := db.NewUser(context.Background(), dbplugin.NewUserRequest{
			UsernameConfig: dbplugin.UsernameMetadata{DisplayName: "static", RoleName: "static"},
			Statements: dbplugin.Statements{
				Commands: []string{
					fmt.Sprintf(`CREATE USER "%s" IDENTIFIED WITH sha256_password BY '{{password}}';`, staticUser),
				},
			},
			Password:   "initialPassw0rd!",
			Expiration: time.Now().Add(time.Hour),
		})
		require.NoError(t, err)
		requireCanConnect(t, addr, staticUser, "initialPassw0rd!")

		_, err = db.UpdateUser(context.Background(), dbplugin.UpdateUserRequest{
			Username: staticUser,
			Password: &dbplugin.ChangePassword{NewPassword: "rotatedPassw0rd!"},
		})
		require.NoError(t, err)

		requireCannotConnect(t, addr, staticUser, "initialPassw0rd!")
		requireCanConnect(t, addr, staticUser, "rotatedPassw0rd!")
	})
}
