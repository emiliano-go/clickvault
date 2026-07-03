---
seo:
  title: Architecture - ClickVault Documentation
  canonical: https://clickvault.emiliano-go.com/architecture
  robots: index,follow
  og:
    type: website
    title: Architecture - ClickVault Documentation
    description: Plugin structure
    url: https://clickvault.emiliano-go.com/architecture
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:width: 1376
    image:height: 768
    image:alt: ClickVault documentation
    site_name: ClickVault Documentation
    locale: en_US
  twitter:
    card: summary_large_image
    title: Architecture - ClickVault Documentation
    description: Plugin structure
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:alt: ClickVault documentation
    site: '@emiliano_go_'
  description: Plugin structure
  schema_jsonld:
  - '@context': https://schema.org
    '@type': WebPage
    name: Architecture - ClickVault Documentation
    url: https://clickvault.emiliano-go.com/architecture
    description: Plugin structure
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
      name: Architecture
      item: https://clickvault.emiliano-go.com/architecture
seo_html: "<title>Architecture - ClickVault Documentation</title>\n<meta name=\"description\"\
  \ content=\"Plugin structure\">\n<link rel=\"canonical\" href=\"https://clickvault.emiliano-go.com/architecture\"\
  >\n<meta name=\"robots\" content=\"index,follow\">\n<meta property=\"og:type\" content=\"\
  website\">\n<meta property=\"og:title\" content=\"Architecture - ClickVault Documentation\"\
  >\n<meta property=\"og:description\" content=\"Plugin structure\">\n<meta property=\"\
  og:url\" content=\"https://clickvault.emiliano-go.com/architecture\">\n<meta property=\"\
  og:image\" content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  >\n<meta property=\"og:image:width\" content=\"1376\">\n<meta property=\"og:image:height\"\
  \ content=\"768\">\n<meta property=\"og:image:alt\" content=\"ClickVault documentation\"\
  >\n<meta property=\"og:site_name\" content=\"ClickVault Documentation\">\n<meta\
  \ property=\"og:locale\" content=\"en_US\">\n<meta name=\"twitter:card\" content=\"\
  summary_large_image\">\n<meta name=\"twitter:title\" content=\"Architecture - ClickVault\
  \ Documentation\">\n<meta name=\"twitter:description\" content=\"Plugin structure\"\
  >\n<meta name=\"twitter:image\" content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  >\n<meta name=\"twitter:image:alt\" content=\"ClickVault documentation\">\n<meta\
  \ name=\"twitter:site\" content=\"@emiliano_go_\">\n<script type=\"application/ld+json\"\
  >\n[\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"WebPage\"\
  ,\n    \"name\": \"Architecture - ClickVault Documentation\",\n    \"url\": \"https://clickvault.emiliano-go.com/architecture\"\
  ,\n    \"description\": \"Plugin structure\",\n    \"image\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  ,\n    \"publisher\": {\n      \"@type\": \"Organization\",\n      \"name\": \"\
  Emiliano Gandini Outeda\",\n      \"logo\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  \n    }\n  },\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"\
  BreadcrumbList\",\n    \"itemListElement\": [\n      {\n        \"@type\": \"ListItem\"\
  ,\n        \"position\": 1,\n        \"name\": \"Architecture\",\n        \"item\"\
  : \"https://clickvault.emiliano-go.com/architecture\"\n      }\n    ]\n  }\n]\n\
  </script>\n"
---

# Architecture

## Plugin structure

ClickVault implements the Vault database plugin SDK interface
(`sdk/database/dbplugin/v5`). It registers itself with Vault as plugin
type `clickvault` and communicates over the plugin RPC boundary.

```
┌─────────────┐     RPC      ┌─────────────────────────────┐
│   Vault     │ ◄─────────►  │      clickvault plugin      │
│  (database  │              │                             │
│   secrets   │              │  ┌───────────────────────┐  │
│   engine)   │              │  │   clickvault.go       │  │
└─────────────┘              │  │   (6 interface methods)│  │
                             │  └───────┬───────────────┘  │
                             │          │ calls            │
                             │  ┌───────▼───────────────┐  │
                             │  │   ddl.go              │  │
                             │  │   (SQL construction,  │  │
                             │  │   cluster branching)  │  │
                             │  └───────┬───────────────┘  │
                             │          │ executes         │
                             │  ┌───────▼───────────────┐  │
                             │  │   ClickHouse server   │  │
                             │  └───────────────────────┘  │
                             └─────────────────────────────┘
```

## DDL construction

All SQL is built in one place, `internal/clickvault/ddl.go`. The rest of the
plugin never constructs SQL strings itself; it only calls into `ddl.go` and
executes whatever statements come back.

Key design decisions:

- **Cluster handling is centralized.** If the connection is configured with a
  `cluster`, every generated statement gets `ON CLUSTER '<cluster>'` appended
  automatically (unless the statement already has one). Callers do not branch
  on cluster themselves.

- **ON CLUSTER placement follows ClickHouse grammar.** It goes immediately
  after the entity name for `CREATE`/`ALTER`/`DROP USER`, and immediately
  after the verb for `GRANT`/`REVOKE`.

- **SQL injection prevention.** Generated values are checked for dangerous
  characters (quotes, backticks, backslash, control characters) before
  substitution.

## Concurrency

`ClickvaultPlugin` holds its ClickHouse connection and config behind a single
`sync.RWMutex`, so one plugin instance is safe for the concurrent calls
Vault's plugin framework makes.

## Error handling

Every error returned from the six interface methods is wrapped with
`fmt.Errorf("clickvault <Method>: %w", err)` so failures are traceable back
to which call produced them.
