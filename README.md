<p align="center">
  <img src="clickvault-banner.png" alt="ClickVault">
</p>

# clickvault

clickvault is a HashiCorp Vault database secrets engine plugin for ClickHouse. It lets Vault create short lived, ephemeral ClickHouse users on demand (dynamic secrets) and rotate the password of long lived ClickHouse users on a schedule (static roles), so applications and operators never handle a standing ClickHouse credential directly.

It implements the Vault database plugin SDK interface (`sdk/database/dbplugin/v5`) and registers itself with Vault as plugin type `clickvault`.

## How it works

Vault's database secrets engine talks to clickvault over the plugin RPC boundary and calls six methods:

| Method | Called when | What clickvault does |
|---|---|---|
| `Initialize` | `vault write database/config/<name>` | Parses the connection config, builds a ClickHouse connection, verifies the admin user holds the `ACCESS MANAGEMENT` privilege (when `verify_connection=true`) |
| `NewUser` | A lease is requested against a dynamic role (`database/roles/<name>`) | Generates a username from the username template, runs the role's `creation_statements` |
| `UpdateUser` | A static role's rotation period elapses, or `vault write -f database/rotate-role/<name>` | Runs `rotation_statements` (or a default `ALTER USER` statement) to change the user's password |
| `DeleteUser` | A dynamic secret's lease expires or is revoked | Runs `revocation_statements` (or a default `DROP USER IF EXISTS` statement); safe to call on a user that is already gone |
| `Type` | Internal to Vault | Returns `"clickvault"` |
| `Close` | Plugin shutdown / reload | Closes the ClickHouse connection |

All SQL is built in one place, `internal/clickvault/ddl.go`. The rest of the plugin (`internal/clickvault/clickvault.go`) never constructs SQL strings itself, it only calls into `ddl.go` and executes whatever statements come back. That file is also where single node vs. clustered ClickHouse is handled: if the connection is configured with a `cluster`, every generated statement gets `ON CLUSTER '<cluster>'` inserted at the position ClickHouse's grammar requires (unless the statement already has an explicit one). Callers do not branch on cluster themselves.

## Connection configuration

Set with `vault write database/config/<name> plugin_name=clickvault ...`:

| Field | Required | Description |
|---|---|---|
| `connection_url` | yes | ClickHouse address, e.g. `clickhouse://host:9000`. A scheme is required - a bare `host:9000` is rejected because `host` would be parsed as the scheme. |
| `username` | yes | The Vault admin user in ClickHouse. Must have SQL driven access management enabled and the `ACCESS MANAGEMENT` grant, since it needs to create, alter and drop other users. |
| `password` | yes | Password for `username`. |
| `cluster` | no | If set, all DDL statements get `ON CLUSTER '<cluster>'` inserted at the grammatically correct position. Leave unset or empty for a single node deployment. |
| `username_template` | no | Go template used to generate usernames for dynamic users. Defaults to `DefaultUsernameTemplate` (see below). |

Pass `verify_connection=true` (the default for `vault write database/config/...`) to have `Initialize` ping ClickHouse and confirm the admin user has the `ACCESS MANAGEMENT` privilege before accepting the config. This is checked with a UNION ALL query against `system.grants` and `system.role_grants`, so the privilege is recognized whether it is granted directly or through a role:

```sql
SELECT access_type FROM system.grants WHERE user_name = ?
UNION ALL
SELECT g.access_type FROM system.grants g
WHERE g.role_name IN (SELECT granted_role_name FROM system.role_grants WHERE user_name = ?)
```

looking for a row of `ACCESS MANAGEMENT` or `ALL`. Your ClickHouse admin user needs SQL driven user management enabled (either `<access_management>1</access_management>` in `users.xml`, or a plain `GRANT ACCESS MANAGEMENT ON *.* TO <admin>` once SQL driven access control is on) before this will succeed.

## Statement templates

Vault roles supply the actual SQL clickvault runs, using the placeholders `{{username}}` (or `{{name}}`, both refer to the same value), `{{password}}` and, for `NewUser`, `{{expiration}}`. A raw statement string can contain multiple `;` separated statements; each one is templated and executed independently.

### NewUser (`creation_statements`)

There is no built in default: a dynamic role must always set `creation_statements`, since only the operator knows which database and role the new user should be granted.

Single node:

```sql
CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}';
GRANT analytics ON default.* TO "{{username}}";
```

With a `cluster` configured on the connection, clickvault turns the same statements into:

```sql
CREATE USER "{{username}}" ON CLUSTER 'prod' IDENTIFIED WITH sha256_password BY '{{password}}';
GRANT ON CLUSTER 'prod' analytics ON default.* TO "{{username}}";
```

You write the single node version in your role config; clickvault inserts the `ON CLUSTER` clause at the position ClickHouse's grammar requires (immediately after the entity name for `CREATE`/`ALTER`/`DROP USER`, and immediately after the verb for `GRANT`/`REVOKE`). If a statement already contains an explicit `ON CLUSTER`, it is left untouched.

### UpdateUser (`rotation_statements`, static roles only)

If a static role does not set `rotation_statements`, clickvault falls back to:

```sql
ALTER USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}';
```

(with `ON CLUSTER '<cluster>'` inserted after the user name when a cluster is configured).

### DeleteUser (`revocation_statements`)

If a role does not set `revocation_statements`, clickvault falls back to:

```sql
DROP USER IF EXISTS "{{username}}";
```

Because ClickHouse's `DROP USER IF EXISTS` does not error on a missing user, and clickvault always uses that form by default, deleting a user that is already gone is not an error. This holds for custom `revocation_statements` too, as long as they also use `IF EXISTS`.

## Username template

Dynamic usernames are generated with `sdk/helper/template`. The default template is:

```
{{ printf "v-%s-%s-%s" (.DisplayName | truncate 8) (random 8) (unix_time) | truncate 255 }}
```

which produces names like `v-token-a1b2c3d4-1719945600`. The Vault role name is intentionally omitted to avoid leaking internal Vault structure into ClickHouse logs. ClickHouse identifiers are limited to 255 characters; the final `truncate 255` and a hard safety truncation in code both enforce that. You can override this with `username_template` in the connection config, using any fields and functions supported by the Vault SDK template package (`.DisplayName`, `.RoleName`, `random N`, `unix_time`, `truncate N`, `uppercase`, etc).

## Password policy

ClickHouse's `sha256_password` auth type has no hard character set requirement, but Vault needs a password policy to generate credentials with. Define one and reference it from your roles, do not hardcode password rules inside the plugin:

```hcl
length = 20
rule "charset" {
  charset = "abcdefghijklmnopqrstuvwxyz"
}
rule "charset" {
  charset   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
  min-chars = 1
}
rule "charset" {
  charset   = "0123456789"
  min-chars = 1
}
```

`scripts/setup_vault.sh` creates this policy as `clickhouse-password-policy` and wires it into the example roles.

> **Security note.** ClickHouse DDL cannot be parameterized, so clickvault substitutes the generated username and password into the statement as literal text. It rejects any value containing a single quote, double quote, backtick, backslash or control character (which could otherwise break out of the surrounding SQL quoting and inject arbitrary DDL). A user create/rotate will fail rather than run unsafe SQL. All other printable characters, including ordinary symbols, are allowed, so an alphanumeric-plus-symbols policy like the one above is both strong and safe.

## Repository layout

```
clickvault/
├── go.mod, go.sum          module github.com/emiliano-go/clickvault
├── main.go                 plugin entrypoint, calls dbplugin.ServeMultiplex
├── internal/clickvault/
│   ├── clickvault.go        ClickvaultPlugin and the six dbplugin.Database methods
│   ├── clickvault_test.go   unit tests, ClickHouse calls mocked with go-sqlmock
│   ├── ddl.go                all SQL construction and cluster branching
│   └── ddl_test.go           table driven tests for ddl.go
├── testdata/docker-compose.yml   single node ClickHouse for local/manual testing
├── tests/integration_test.go     integration tests against a real ClickHouse container
├── scripts/setup_vault.sh        registers and configures the plugin against a dev Vault
└── .github/workflows/ci.yml      go vet, go test, build, sha256 artifact
```

## Building

```bash
make build              # bin/clickvault, current OS/arch
make build-linux-amd64  # bin/clickvault-linux-amd64
make build-linux-arm64  # bin/clickvault-linux-arm64
make sha256              # builds linux/amd64 and prints its sha256sum
```

## Testing

Unit tests mock the database connection and do not need Docker or a running ClickHouse:

```bash
make test
# or
go test ./...
```

Integration tests spin up a real ClickHouse container (matching `testdata/docker-compose.yml`) with the vault SDK's Docker test helper, and drive the full lifecycle: `Initialize`, `NewUser`, verify the user can connect, `DeleteUser`, verify the user can no longer connect, a repeat `DeleteUser` to confirm idempotency, and `UpdateUser` on a static style user with a before/after connection check. They are gated behind the `integration` build tag so they don't run as part of a normal `go test ./...` and don't require Docker in environments that don't have it:

```bash
go test -tags=integration ./tests/...
```

Docker must be running locally for that command to work.

## Registering with Vault

Build the plugin, register it with Vault's plugin catalog, then configure a connection and roles. `scripts/setup_vault.sh` automates all of this against a dev Vault server; read it for the exact commands, or run it directly once these env vars are set:

```bash
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=<a token with access to sys/plugins and the database engine>
export CLICKHOUSE_VAULT_ADMIN_PASSWORD=<password for the ClickHouse admin user>

# optional overrides, see the script header for defaults
export CLICKHOUSE_CONNECTION_URL=clickhouse://clickhouse:9000
export CLICKHOUSE_VAULT_ADMIN_USER=vault_admin
export CLICKVAULT_DB_NAME=pos-clickhouse

./scripts/setup_vault.sh
```

The script:

1. Builds `bin/clickvault-linux-amd64` and computes its sha256.
2. Enables the `database` secrets engine (skips if already enabled).
3. Registers the plugin: `vault plugin register -sha256=<sha> database clickvault`.
4. Creates the `clickhouse-password-policy` password policy shown above.
5. Configures a connection at `database/config/${CLICKVAULT_DB_NAME}`.
6. Creates an example dynamic role, `pos-analytics-dynamic`.
7. Creates an example static role, `pos-service-account`.

If you're doing this by hand instead, the manual equivalent is:

```bash
SHA256=$(sha256sum bin/clickvault-linux-amd64 | awk '{print $1}')

vault secrets enable database

vault plugin register \
  -sha256="$SHA256" \
  database clickvault

vault write database/config/pos-clickhouse \
  plugin_name=clickvault \
  connection_url="clickhouse://clickhouse:9000" \
  username="vault_admin" \
  password="$CLICKHOUSE_VAULT_ADMIN_PASSWORD" \
  cluster=""
```

### Dynamic role (ephemeral credentials)

```bash
vault write database/roles/pos-analytics-dynamic \
  db_name=pos-clickhouse \
  creation_statements='CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '"'"'{{password}}'"'"'; GRANT analytics ON default.* TO "{{username}}";' \
  default_ttl="24h" \
  max_ttl="48h" \
  password_policy="clickhouse-password-policy"
```

Read a lease with `vault read database/creds/pos-analytics-dynamic`. Vault creates a new ClickHouse user for that lease and drops it automatically when the lease expires or is revoked.

### Static role (managed rotation of an existing user)

```bash
vault write database/static-roles/pos-service-account \
  db_name=pos-clickhouse \
  username="pos_service" \
  rotation_period="72h" \
  password_policy="clickhouse-password-policy"
```

The ClickHouse user `pos_service` must already exist. Vault rotates its password every 72 hours (or on demand with `vault write -f database/rotate-role/pos-service-account`) and hands out the current password with `vault read database/static-creds/pos-service-account`.

### Cluster deployments

If ClickHouse is a multi node cluster, set `cluster` on the connection config instead of hand writing `ON CLUSTER` into every statement:

```bash
vault write database/config/pos-clickhouse \
  plugin_name=clickvault \
  connection_url="clickhouse://clickhouse:9000" \
  username="vault_admin" \
  password="$CLICKHOUSE_VAULT_ADMIN_PASSWORD" \
  cluster="prod"
```

Role `creation_statements`, `rotation_statements` and `revocation_statements` stay written as the single node form; clickvault inserts `ON CLUSTER 'prod'` at the grammatically correct position in each generated statement automatically.

## Concurrency and error handling

`ClickvaultPlugin` holds its ClickHouse connection and config behind a single `sync.RWMutex`, so one plugin instance is safe for the concurrent calls Vault's plugin framework makes. Every error returned from the six interface methods is wrapped with `fmt.Errorf("clickvault <Method>: %w", err)` so failures are traceable back to which call produced them without needing to inspect logs from inside the plugin process.
