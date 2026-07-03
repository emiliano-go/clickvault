---
seo:
  title: Configuration - ClickVault Documentation
  canonical: https://clickvault.emiliano-go.com/configuration
  robots: index,follow
  og:
    type: website
    title: Configuration - ClickVault Documentation
    description: 'Set with vault write database/config/<name pluginname=clickvault
      ...:'
    url: https://clickvault.emiliano-go.com/configuration
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:width: 1376
    image:height: 768
    image:alt: ClickVault documentation
    site_name: ClickVault Documentation
    locale: en_US
  twitter:
    card: summary_large_image
    title: Configuration - ClickVault Documentation
    description: 'Set with vault write database/config/<name pluginname=clickvault
      ...:'
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:alt: ClickVault documentation
    site: '@emiliano_go_'
  description: 'Set with vault write database/config/<name pluginname=clickvault ...:'
  schema_jsonld:
  - '@context': https://schema.org
    '@type': WebPage
    name: Configuration - ClickVault Documentation
    url: https://clickvault.emiliano-go.com/configuration
    description: 'Set with vault write database/config/<name pluginname=clickvault
      ...:'
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
      name: Configuration
      item: https://clickvault.emiliano-go.com/configuration
seo_html: "<title>Configuration - ClickVault Documentation</title>\n<meta name=\"\
  description\" content=\"Set with vault write database/config/&lt;name pluginname=clickvault\
  \ ...:\">\n<link rel=\"canonical\" href=\"https://clickvault.emiliano-go.com/configuration\"\
  >\n<meta name=\"robots\" content=\"index,follow\">\n<meta property=\"og:type\" content=\"\
  website\">\n<meta property=\"og:title\" content=\"Configuration - ClickVault Documentation\"\
  >\n<meta property=\"og:description\" content=\"Set with vault write database/config/&lt;name\
  \ pluginname=clickvault ...:\">\n<meta property=\"og:url\" content=\"https://clickvault.emiliano-go.com/configuration\"\
  >\n<meta property=\"og:image\" content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  >\n<meta property=\"og:image:width\" content=\"1376\">\n<meta property=\"og:image:height\"\
  \ content=\"768\">\n<meta property=\"og:image:alt\" content=\"ClickVault documentation\"\
  >\n<meta property=\"og:site_name\" content=\"ClickVault Documentation\">\n<meta\
  \ property=\"og:locale\" content=\"en_US\">\n<meta name=\"twitter:card\" content=\"\
  summary_large_image\">\n<meta name=\"twitter:title\" content=\"Configuration - ClickVault\
  \ Documentation\">\n<meta name=\"twitter:description\" content=\"Set with vault\
  \ write database/config/&lt;name pluginname=clickvault ...:\">\n<meta name=\"twitter:image\"\
  \ content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\">\n<meta\
  \ name=\"twitter:image:alt\" content=\"ClickVault documentation\">\n<meta name=\"\
  twitter:site\" content=\"@emiliano_go_\">\n<script type=\"application/ld+json\"\
  >\n[\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"WebPage\"\
  ,\n    \"name\": \"Configuration - ClickVault Documentation\",\n    \"url\": \"\
  https://clickvault.emiliano-go.com/configuration\",\n    \"description\": \"Set\
  \ with vault write database/config/<name pluginname=clickvault ...:\",\n    \"image\"\
  : \"https://clickvault.emiliano-go.com/assets/images/og-image.png\",\n    \"publisher\"\
  : {\n      \"@type\": \"Organization\",\n      \"name\": \"Emiliano Gandini Outeda\"\
  ,\n      \"logo\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  \n    }\n  },\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"\
  BreadcrumbList\",\n    \"itemListElement\": [\n      {\n        \"@type\": \"ListItem\"\
  ,\n        \"position\": 1,\n        \"name\": \"Configuration\",\n        \"item\"\
  : \"https://clickvault.emiliano-go.com/configuration\"\n      }\n    ]\n  }\n]\n\
  </script>\n"
---

# Configuration

Set with `vault write database/config/<name> plugin_name=clickvault ...`:

| Field | Required | Description |
|---|---|---|
| `connection_url` | yes | ClickHouse address. Must include a scheme, e.g. `clickhouse://host:9000`. A bare `host:9000` is rejected. See note below. |
| `username` | yes | The Vault admin user in ClickHouse. Must have `ACCESS MANAGEMENT` grant. |
| `password` | yes | Password for `username`. |
| `cluster` | no | If set, all DDL statements get `ON CLUSTER '<cluster>'` inserted at the grammatically correct position. Leave unset or empty for a single node deployment. |
| `username_template` | no | Go template for dynamic usernames. See below for default. |
| `tls` | no | Enable TLS for the ClickHouse connection (default: `false`). |
| `tls_skip_verify` | no | Skip TLS certificate verification (default: `false`, only used when `tls=true`). |
| `dial_timeout_seconds` | no | Dial timeout in seconds (default: `5`). |
| `read_timeout_seconds` | no | Read timeout in seconds (default: `30`). |

> **Note on `connection_url`:** unlike some ClickHouse client libraries (e.g. `clickhouse-go`'s default DSN parsing), clickvault does **not** accept a bare `host:port` address. A scheme prefix is required. This is intentional: inferring a scheme from context (defaulting to `clickhouse://` or `tcp://`) is the kind of ambiguity that makes URL-parsing bugs security-relevant, so `parseAddr` rejects bare addresses rather than guessing. If you are migrating a `connection_url` from another tool, add the scheme explicitly.

Pass `verify_connection=true` (the default) to have `Initialize` ping ClickHouse
and confirm the admin user has the `ACCESS MANAGEMENT` privilege before
accepting the config.

## Username template

Dynamic usernames are generated with `sdk/helper/template`. The default
template is:

```text
{{ printf "v-%s-%s-%s" (.DisplayName | truncate 8) (random 8) (unix_time) | truncate 255 }}
```

This produces names like `v-token-a1b2c3d4-1719945600`. ClickHouse identifiers
are limited to 255 characters. You can override this with `username_template` in
the connection config.

> **Collision risk:** The default template truncates `DisplayName` to 8
> characters. In high-volume deployments or when many Vault entities share a
> naming prefix (e.g. `svc-payments-*`, `svc-payroll-*`), truncated display
> names can collide. clickvault checks for an existing ClickHouse user with the
> generated name before running `creation_statements` and fails with a clear
> error rather than silently overwriting or erroring opaquely. If you see this
> error frequently, use a longer `username_template`.
>
> The Vault role name is intentionally **not** included in the default
> template — only `DisplayName` — since ClickHouse usernames are visible in
> `system.users`, query logs, connection logs and `system.query_log`, and
> `RoleName` often encodes more about the credential's purpose (e.g.
> `prod-billing-admin`) than you may want exposed there.

## Password policy

ClickHouse's `sha256_password` auth type has no hard character set requirement,
but Vault needs a password policy:

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

`scripts/setup_vault.sh` creates this policy as `clickhouse-password-policy`.
