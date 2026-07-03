---
seo:
  title: Quick Start - ClickVault Documentation
  canonical: https://clickvault.emiliano-go.com/quick-start
  robots: index,follow
  og:
    type: website
    title: Quick Start - ClickVault Documentation
    description: Prerequisites
    url: https://clickvault.emiliano-go.com/quick-start
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:width: 1376
    image:height: 768
    image:alt: ClickVault documentation
    site_name: ClickVault Documentation
    locale: en_US
  twitter:
    card: summary_large_image
    title: Quick Start - ClickVault Documentation
    description: Prerequisites
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:alt: ClickVault documentation
    site: '@emiliano_go_'
  description: Prerequisites
  schema_jsonld:
  - '@context': https://schema.org
    '@type': WebPage
    name: Quick Start - ClickVault Documentation
    url: https://clickvault.emiliano-go.com/quick-start
    description: Prerequisites
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    publisher:
      '@type': Organization
      name: Emiliano Gandini Outeda
      logo: https://clickvault.emiliano-go.com/assets/images/og-image.png
  - '@context': https://schema.org
    '@type': BreadcrumbList
    itemListElement:
    - '@type': ListItem
      position: 1
      name: Quick Start
      item: https://clickvault.emiliano-go.com/quick-start
seo_html: "<title>Quick Start - ClickVault Documentation</title>\n<meta name=\"description\"\
  \ content=\"Prerequisites\">\n<link rel=\"canonical\" href=\"https://clickvault.emiliano-go.com/quick-start\"\
  >\n<meta name=\"robots\" content=\"index,follow\">\n<meta property=\"og:type\" content=\"\
  website\">\n<meta property=\"og:title\" content=\"Quick Start - ClickVault Documentation\"\
  >\n<meta property=\"og:description\" content=\"Prerequisites\">\n<meta property=\"\
  og:url\" content=\"https://clickvault.emiliano-go.com/quick-start\">\n<meta property=\"\
  og:image\" content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  >\n<meta property=\"og:image:width\" content=\"1376\">\n<meta property=\"og:image:height\"\
  \ content=\"768\">\n<meta property=\"og:image:alt\" content=\"ClickVault documentation\"\
  >\n<meta property=\"og:site_name\" content=\"ClickVault Documentation\">\n<meta\
  \ property=\"og:locale\" content=\"en_US\">\n<meta name=\"twitter:card\" content=\"\
  summary_large_image\">\n<meta name=\"twitter:title\" content=\"Quick Start - ClickVault\
  \ Documentation\">\n<meta name=\"twitter:description\" content=\"Prerequisites\"\
  >\n<meta name=\"twitter:image\" content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  >\n<meta name=\"twitter:image:alt\" content=\"ClickVault documentation\">\n<meta\
  \ name=\"twitter:site\" content=\"@emiliano_go_\">\n<script type=\"application/ld+json\"\
  >\n[\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"WebPage\"\
  ,\n    \"name\": \"Quick Start - ClickVault Documentation\",\n    \"url\": \"https://clickvault.emiliano-go.com/quick-start\"\
  ,\n    \"description\": \"Prerequisites\",\n    \"image\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  ,\n    \"publisher\": {\n      \"@type\": \"Organization\",\n      \"name\": \"\
  Emiliano Gandini Outeda\",\n      \"logo\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  \n    }\n  },\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"\
  BreadcrumbList\",\n    \"itemListElement\": [\n      {\n        \"@type\": \"ListItem\"\
  ,\n        \"position\": 1,\n        \"name\": \"Quick Start\",\n        \"item\"\
  : \"https://clickvault.emiliano-go.com/quick-start\"\n      }\n    ]\n  }\n]\n</script>\n"
---

# Quick Start

## Prerequisites

- Go 1.26+
- A running ClickHouse instance
- A running Vault instance (dev mode is fine for testing)

## Build the plugin

```bash
make build              # bin/clickvault, current OS/arch
make build-linux-amd64  # bin/clickvault-linux-amd64
make sha256             # builds linux/amd64 and prints its sha256sum
```

## Register with Vault

Build the plugin, register it with Vault's plugin catalog, then configure a
connection and roles. The `scripts/setup_vault.sh` script automates all of
this against a dev Vault server:

```bash
export VAULT_ADDR=http://127.0.0.1:8200
export VAULT_TOKEN=<token with access to sys/plugins and the database engine>
export CLICKHOUSE_VAULT_ADMIN_PASSWORD=<password for ClickHouse admin user>

./scripts/setup_vault.sh
```

## Manual setup

If you prefer to do it step by step:

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

## Create a dynamic role

```bash
vault write database/roles/pos-analytics-dynamic \
  db_name=pos-clickhouse \
  creation_statements='CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '"'"'{{password}}'"'"'; GRANT analytics ON default.* TO "{{username}}";' \
  default_ttl="24h" \
  max_ttl="48h" \
  password_policy="clickhouse-password-policy"
```

Read a lease with `vault read database/creds/pos-analytics-dynamic`. Vault
creates a new ClickHouse user for that lease and drops it automatically when
the lease expires or is revoked.

## Create a static role

```bash
vault write database/static-roles/pos-service-account \
  db_name=pos-clickhouse \
  username="pos_service" \
  rotation_period="72h" \
  password_policy="clickhouse-password-policy"
```

The ClickHouse user `pos_service` must already exist. Vault rotates its
password every 72 hours and hands out the current password with
`vault read database/static-creds/pos-service-account`.
