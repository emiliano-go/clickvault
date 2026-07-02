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
		require.Regexp(t, `^v-token-itrole-`, newUserResp.Username)

		requireCanConnect(t, addr, newUserResp.Username, "sUp3rS3cret!")

		_, err = db.DeleteUser(context.Background(), dbplugin.DeleteUserRequest{Username: newUserResp.Username})
		require.NoError(t, err)

		requireCannotConnect(t, addr, newUserResp.Username, "sUp3rS3cret!")

		// DeleteUser must be idempotent: deleting an already-gone user is not an error.
		_, err = db.DeleteUser(context.Background(), dbplugin.DeleteUserRequest{Username: newUserResp.Username})
		require.NoError(t, err)
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
