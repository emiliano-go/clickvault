// Package clickvault implements a HashiCorp Vault database secrets engine
// plugin for ClickHouse.
package clickvault

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/ClickHouse/clickhouse-go/v2"
	dbplugin "github.com/hashicorp/vault/sdk/database/dbplugin/v5"
	"github.com/hashicorp/vault/sdk/helper/template"
	"github.com/mitchellh/mapstructure"
)

const (
	pluginType = "clickvault"

	// DefaultUsernameTemplate produces usernames like
	// v-token-myrole-a1b2c3d4-1719945600, truncated to fit ClickHouse's
	// 255-character username limit.
	DefaultUsernameTemplate = `{{ printf "v-%s-%s-%s-%s" (.DisplayName | truncate 64) (.RoleName | truncate 64) (random 8) (unix_time) | truncate 255 }}`

	// maxUsernameLength is ClickHouse's hard limit on identifier length.
	maxUsernameLength = 255

	defaultMaxOpenConnections = 4
)

var _ dbplugin.Database = (*ClickvaultPlugin)(nil)

// config holds the connection settings parsed from InitializeRequest.Config.
type config struct {
	ConnectionURL    string `mapstructure:"connection_url"`
	Username         string `mapstructure:"username"`
	Password         string `mapstructure:"password"`
	Cluster          string `mapstructure:"cluster"`
	UsernameTemplate string `mapstructure:"username_template"`
}

// ClickvaultPlugin implements dbplugin.Database for ClickHouse. All access to
// db and cfg is guarded by mu so a single instance is safe for the
// concurrent use Vault's plugin framework requires.
type ClickvaultPlugin struct {
	mu sync.RWMutex

	db  *sql.DB
	cfg config

	usernameProducer template.StringTemplate
}

// New is the dbplugin.Database factory used by main.go to serve the plugin.
func New() (interface{}, error) {
	db := &ClickvaultPlugin{}

	return dbplugin.NewDatabaseErrorSanitizerMiddleware(db, db.secretValues), nil
}

func (p *ClickvaultPlugin) secretValues() map[string]string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.cfg.Password == "" {
		return nil
	}

	return map[string]string{p.cfg.Password: "[password]"}
}

func (p *ClickvaultPlugin) Type() (string, error) {
	return pluginType, nil
}

func (p *ClickvaultPlugin) Initialize(ctx context.Context, req dbplugin.InitializeRequest) (dbplugin.InitializeResponse, error) {
	var cfg config
	if err := mapstructure.WeakDecode(req.Config, &cfg); err != nil {
		return dbplugin.InitializeResponse{}, fmt.Errorf("clickvault Initialize: unable to decode config: %w", err)
	}

	if cfg.ConnectionURL == "" {
		return dbplugin.InitializeResponse{}, errors.New("clickvault Initialize: connection_url is required")
	}
	if cfg.Username == "" {
		return dbplugin.InitializeResponse{}, errors.New("clickvault Initialize: username is required")
	}
	if cfg.Password == "" {
		return dbplugin.InitializeResponse{}, errors.New("clickvault Initialize: password is required")
	}

	usernameTemplate := cfg.UsernameTemplate
	if usernameTemplate == "" {
		usernameTemplate = DefaultUsernameTemplate
	}

	up, err := template.NewTemplate(template.Template(usernameTemplate))
	if err != nil {
		return dbplugin.InitializeResponse{}, fmt.Errorf("clickvault Initialize: invalid username template: %w", err)
	}
	if _, err := up.Generate(dbplugin.UsernameMetadata{}); err != nil {
		return dbplugin.InitializeResponse{}, fmt.Errorf("clickvault Initialize: invalid username template: %w", err)
	}

	db, err := openDB(cfg)
	if err != nil {
		return dbplugin.InitializeResponse{}, fmt.Errorf("clickvault Initialize: %w", err)
	}

	if req.VerifyConnection {
		if err := verifyConnection(ctx, db, cfg.Username); err != nil {
			db.Close()
			return dbplugin.InitializeResponse{}, fmt.Errorf("clickvault Initialize: %w", err)
		}
	}

	p.mu.Lock()
	if p.db != nil {
		p.db.Close()
	}
	p.db = db
	p.cfg = cfg
	p.usernameProducer = up
	p.mu.Unlock()

	return dbplugin.InitializeResponse{Config: req.Config}, nil
}

func (p *ClickvaultPlugin) NewUser(ctx context.Context, req dbplugin.NewUserRequest) (dbplugin.NewUserResponse, error) {
	if len(req.Statements.Commands) == 0 {
		return dbplugin.NewUserResponse{}, ErrEmptyCreationStatement
	}

	p.mu.RLock()
	db := p.db
	cluster := p.cfg.Cluster
	up := p.usernameProducer
	p.mu.RUnlock()

	if db == nil {
		return dbplugin.NewUserResponse{}, errors.New("clickvault NewUser: plugin not initialized")
	}

	username, err := up.Generate(req.UsernameConfig)
	if err != nil {
		return dbplugin.NewUserResponse{}, fmt.Errorf("clickvault NewUser: unable to generate username: %w", err)
	}
	if len(username) > maxUsernameLength {
		username = username[:maxUsernameLength]
	}

	expiration := req.Expiration.Format("2006-01-02 15:04:05-0700")

	statements, err := creationStatements(cluster, req.Statements.Commands, username, req.Password, expiration)
	if err != nil {
		return dbplugin.NewUserResponse{}, fmt.Errorf("clickvault NewUser: %w", err)
	}

	if err := execStatements(ctx, db, statements); err != nil {
		return dbplugin.NewUserResponse{}, fmt.Errorf("clickvault NewUser: %w", err)
	}

	return dbplugin.NewUserResponse{Username: username}, nil
}

func (p *ClickvaultPlugin) UpdateUser(ctx context.Context, req dbplugin.UpdateUserRequest) (dbplugin.UpdateUserResponse, error) {
	if req.Password == nil && req.Expiration == nil {
		return dbplugin.UpdateUserResponse{}, errors.New("clickvault UpdateUser: no change requested")
	}

	p.mu.RLock()
	db := p.db
	cluster := p.cfg.Cluster
	p.mu.RUnlock()

	if db == nil {
		return dbplugin.UpdateUserResponse{}, errors.New("clickvault UpdateUser: plugin not initialized")
	}

	if req.Password != nil {
		statements, err := rotateStatements(cluster, req.Password.Statements.Commands, req.Username, req.Password.NewPassword)
		if err != nil {
			return dbplugin.UpdateUserResponse{}, fmt.Errorf("clickvault UpdateUser: %w", err)
		}

		if err := execStatements(ctx, db, statements); err != nil {
			return dbplugin.UpdateUserResponse{}, fmt.Errorf("clickvault UpdateUser: %w", err)
		}
	}

	// Expiration changes are a no-op: ClickHouse users don't carry a TTL that
	// Vault can update independently of the password rotation above.

	return dbplugin.UpdateUserResponse{}, nil
}

func (p *ClickvaultPlugin) DeleteUser(ctx context.Context, req dbplugin.DeleteUserRequest) (dbplugin.DeleteUserResponse, error) {
	p.mu.RLock()
	db := p.db
	cluster := p.cfg.Cluster
	p.mu.RUnlock()

	if db == nil {
		return dbplugin.DeleteUserResponse{}, errors.New("clickvault DeleteUser: plugin not initialized")
	}

	statements, err := deleteStatements(cluster, req.Statements.Commands, req.Username)
	if err != nil {
		return dbplugin.DeleteUserResponse{}, fmt.Errorf("clickvault DeleteUser: %w", err)
	}

	if err := execStatements(ctx, db, statements); err != nil {
		return dbplugin.DeleteUserResponse{}, fmt.Errorf("clickvault DeleteUser: %w", err)
	}

	return dbplugin.DeleteUserResponse{}, nil
}

func (p *ClickvaultPlugin) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.db == nil {
		return nil
	}

	err := p.db.Close()
	p.db = nil

	return err
}

func execStatements(ctx context.Context, db *sql.DB, statements []string) error {
	for _, stmt := range statements {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("unable to execute statement: %w", err)
		}
	}

	return nil
}

// openDB builds a *sql.DB for cfg without connecting. connection_url is
// expected in host:port form, optionally prefixed with a clickhouse:// (or
// any other) scheme, e.g. "clickhouse://host:9000" or "host:9000".
func openDB(cfg config) (*sql.DB, error) {
	addr, err := parseAddr(cfg.ConnectionURL)
	if err != nil {
		return nil, fmt.Errorf("invalid connection_url: %w", err)
	}

	db := clickhouse.OpenDB(&clickhouse.Options{
		Addr: []string{addr},
		Auth: clickhouse.Auth{
			Username: cfg.Username,
			Password: cfg.Password,
		},
	})

	db.SetMaxOpenConns(defaultMaxOpenConnections)
	db.SetMaxIdleConns(defaultMaxOpenConnections)

	return db, nil
}

func parseAddr(connectionURL string) (string, error) {
	u, err := url.Parse(connectionURL)
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		// No recognizable scheme (e.g. "host:9000" without "clickhouse://");
		// treat the whole string as the address.
		return connectionURL, nil
	}

	return u.Host, nil
}

// verifyConnection pings the database and confirms that username holds the
// ACCESS MANAGEMENT privilege, since NewUser/UpdateUser/DeleteUser all
// require it to create, alter and drop other ClickHouse users.
func verifyConnection(ctx context.Context, db *sql.DB, username string) error {
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("error verifying connection: %w", err)
	}

	rows, err := db.QueryContext(ctx, "SELECT access_type FROM system.grants WHERE user_name = ?", username)
	if err != nil {
		return fmt.Errorf("unable to verify access_management grant: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var accessType string
		if err := rows.Scan(&accessType); err != nil {
			return fmt.Errorf("unable to verify access_management grant: %w", err)
		}
		if accessType == "ACCESS MANAGEMENT" || accessType == "ALL" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("unable to verify access_management grant: %w", err)
	}

	return fmt.Errorf("admin user %q does not have the ACCESS MANAGEMENT privilege", username)
}
