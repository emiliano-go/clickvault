---
seo:
  title: ClickVault - ClickVault Documentation
  canonical: https://clickvault.emiliano-go.com/
  robots: index,follow
  og:
    type: website
    title: ClickVault - ClickVault Documentation
    description: ClickVault is a HashiCorp Vault database secrets engine plugin for
      ClickHouse. It lets Vault create short-lived, ephemeral ClickHouse users on
      demand dynamic...
    url: https://clickvault.emiliano-go.com/
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:width: 1376
    image:height: 768
    image:alt: ClickVault documentation
    site_name: ClickVault Documentation
    locale: en_US
  twitter:
    card: summary_large_image
    title: ClickVault - ClickVault Documentation
    description: ClickVault is a HashiCorp Vault database secrets engine plugin for
      ClickHouse. It lets Vault create short-lived, ephemeral ClickHouse users on
      demand dynamic...
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:alt: ClickVault documentation
    site: '@emiliano_go_'
  description: ClickVault is a HashiCorp Vault database secrets engine plugin for
    ClickHouse. It lets Vault create short-lived, ephemeral ClickHouse users on demand
    dynamic...
  schema_jsonld:
    '@context': https://schema.org
    '@type': WebPage
    name: ClickVault - ClickVault Documentation
    url: https://clickvault.emiliano-go.com/
    description: ClickVault is a HashiCorp Vault database secrets engine plugin for
      ClickHouse. It lets Vault create short-lived, ephemeral ClickHouse users on
      demand dynamic...
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    publisher:
      '@type': Organization
      name: Emiliano Gandini Outeda
      logo: https://clickvault.emiliano-go.com/assets/images/og-image.png
seo_html: "<title>ClickVault - ClickVault Documentation</title>\n<meta name=\"description\"\
  \ content=\"ClickVault is a HashiCorp Vault database secrets engine plugin for ClickHouse.\
  \ It lets Vault create short-lived, ephemeral ClickHouse users on demand dynamic...\"\
  >\n<link rel=\"canonical\" href=\"https://clickvault.emiliano-go.com/\">\n<meta\
  \ name=\"robots\" content=\"index,follow\">\n<meta property=\"og:type\" content=\"\
  website\">\n<meta property=\"og:title\" content=\"ClickVault - ClickVault Documentation\"\
  >\n<meta property=\"og:description\" content=\"ClickVault is a HashiCorp Vault database\
  \ secrets engine plugin for ClickHouse. It lets Vault create short-lived, ephemeral\
  \ ClickHouse users on demand dynamic...\">\n<meta property=\"og:url\" content=\"\
  https://clickvault.emiliano-go.com/\">\n<meta property=\"og:image\" content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  >\n<meta property=\"og:image:width\" content=\"1376\">\n<meta property=\"og:image:height\"\
  \ content=\"768\">\n<meta property=\"og:image:alt\" content=\"ClickVault documentation\"\
  >\n<meta property=\"og:site_name\" content=\"ClickVault Documentation\">\n<meta\
  \ property=\"og:locale\" content=\"en_US\">\n<meta name=\"twitter:card\" content=\"\
  summary_large_image\">\n<meta name=\"twitter:title\" content=\"ClickVault - ClickVault\
  \ Documentation\">\n<meta name=\"twitter:description\" content=\"ClickVault is a\
  \ HashiCorp Vault database secrets engine plugin for ClickHouse. It lets Vault create\
  \ short-lived, ephemeral ClickHouse users on demand dynamic...\">\n<meta name=\"\
  twitter:image\" content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  >\n<meta name=\"twitter:image:alt\" content=\"ClickVault documentation\">\n<meta\
  \ name=\"twitter:site\" content=\"@emiliano_go_\">\n<script type=\"application/ld+json\"\
  >\n{\n  \"@context\": \"https://schema.org\",\n  \"@type\": \"WebPage\",\n  \"name\"\
  : \"ClickVault - ClickVault Documentation\",\n  \"url\": \"https://clickvault.emiliano-go.com/\"\
  ,\n  \"description\": \"ClickVault is a HashiCorp Vault database secrets engine\
  \ plugin for ClickHouse. It lets Vault create short-lived, ephemeral ClickHouse\
  \ users on demand dynamic...\",\n  \"image\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  ,\n  \"publisher\": {\n    \"@type\": \"Organization\",\n    \"name\": \"Emiliano\
  \ Gandini Outeda\",\n    \"logo\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  \n  }\n}\n</script>\n"
---

# ClickVault

ClickVault is a **HashiCorp Vault database secrets engine plugin for ClickHouse**.
It lets Vault create short-lived, ephemeral ClickHouse users on demand (**dynamic
secrets**) and rotate the password of long-lived ClickHouse users on a schedule
(**static roles**), so applications and operators never handle a standing
ClickHouse credential directly.

It implements the Vault database plugin SDK interface (`sdk/database/dbplugin/v5`)
and registers itself with Vault as plugin type `clickvault`.

## How it works

Vault's database secrets engine talks to ClickVault over the plugin RPC boundary
and calls six methods:

| Method | Called when | What ClickVault does |
|---|---|---|
| `Initialize` | `vault write database/config/<name>` | Parses the connection config, builds a ClickHouse connection, verifies the admin user holds the `ACCESS MANAGEMENT` privilege |
| `NewUser` | A lease is requested against a dynamic role | Generates a username from the template, runs `creation_statements` |
| `UpdateUser` | A static role's rotation period elapses | Runs `rotation_statements` to change the user's password |
| `DeleteUser` | A dynamic secret's lease expires or is revoked | Runs `revocation_statements` (defaults to `DROP USER IF EXISTS`) |
| `Type` | Internal to Vault | Returns `"clickvault"` |
| `Close` | Plugin shutdown / reload | Closes the ClickHouse connection |

All SQL is built in one place, `internal/clickvault/ddl.go`. The rest of the
plugin never constructs SQL strings itself. That file also handles single-node
vs. clustered ClickHouse: if a `cluster` is configured, every generated
statement gets `ON CLUSTER '<cluster>'` appended automatically.

## Key features

- **Dynamic credentials** - ephemeral ClickHouse users created on demand, automatically cleaned up
- **Static role rotation** - scheduled password rotation for existing long-lived users
- **Cluster-aware DDL** - automatic `ON CLUSTER` insertion for multi-node ClickHouse
- **SQL injection prevention** - rejects values containing quotes, backticks, or control characters
- **Concurrency-safe** - single `sync.RWMutex` protects the connection and config

## Repository

```text
clickvault/
├── main.go                    plugin entrypoint
├── internal/clickvault/
│   ├── clickvault.go           six dbplugin.Database methods
│   ├── clickvault_test.go      unit tests (go-sqlmock)
│   ├── ddl.go                  SQL construction and cluster branching
│   └── ddl_test.go             table-driven tests
├── testdata/docker-compose.yml single-node ClickHouse for manual testing
├── tests/integration_test.go   integration tests against real ClickHouse
└── scripts/setup_vault.sh      registers and configures the plugin
```
