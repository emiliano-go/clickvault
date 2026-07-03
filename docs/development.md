---
seo:
  title: Development - ClickVault Documentation
  canonical: https://clickvault.emiliano-go.com/development
  robots: index,follow
  og:
    type: website
    title: Development - ClickVault Documentation
    description: Building
    url: https://clickvault.emiliano-go.com/development
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:width: 1376
    image:height: 768
    image:alt: ClickVault documentation
    site_name: ClickVault Documentation
    locale: en_US
  twitter:
    card: summary_large_image
    title: Development - ClickVault Documentation
    description: Building
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:alt: ClickVault documentation
    site: '@emiliano_go_'
  description: Building
  schema_jsonld:
  - '@context': https://schema.org
    '@type': WebPage
    name: Development - ClickVault Documentation
    url: https://clickvault.emiliano-go.com/development
    description: Building
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
      name: Development
      item: https://clickvault.emiliano-go.com/development
seo_html: "<title>Development - ClickVault Documentation</title>\n<meta name=\"description\"\
  \ content=\"Building\">\n<link rel=\"canonical\" href=\"https://clickvault.emiliano-go.com/development\"\
  >\n<meta name=\"robots\" content=\"index,follow\">\n<meta property=\"og:type\" content=\"\
  website\">\n<meta property=\"og:title\" content=\"Development - ClickVault Documentation\"\
  >\n<meta property=\"og:description\" content=\"Building\">\n<meta property=\"og:url\"\
  \ content=\"https://clickvault.emiliano-go.com/development\">\n<meta property=\"\
  og:image\" content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  >\n<meta property=\"og:image:width\" content=\"1376\">\n<meta property=\"og:image:height\"\
  \ content=\"768\">\n<meta property=\"og:image:alt\" content=\"ClickVault documentation\"\
  >\n<meta property=\"og:site_name\" content=\"ClickVault Documentation\">\n<meta\
  \ property=\"og:locale\" content=\"en_US\">\n<meta name=\"twitter:card\" content=\"\
  summary_large_image\">\n<meta name=\"twitter:title\" content=\"Development - ClickVault\
  \ Documentation\">\n<meta name=\"twitter:description\" content=\"Building\">\n<meta\
  \ name=\"twitter:image\" content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  >\n<meta name=\"twitter:image:alt\" content=\"ClickVault documentation\">\n<meta\
  \ name=\"twitter:site\" content=\"@emiliano_go_\">\n<script type=\"application/ld+json\"\
  >\n[\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"WebPage\"\
  ,\n    \"name\": \"Development - ClickVault Documentation\",\n    \"url\": \"https://clickvault.emiliano-go.com/development\"\
  ,\n    \"description\": \"Building\",\n    \"image\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  ,\n    \"publisher\": {\n      \"@type\": \"Organization\",\n      \"name\": \"\
  Emiliano Gandini Outeda\",\n      \"logo\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  \n    }\n  },\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"\
  BreadcrumbList\",\n    \"itemListElement\": [\n      {\n        \"@type\": \"ListItem\"\
  ,\n        \"position\": 1,\n        \"name\": \"Development\",\n        \"item\"\
  : \"https://clickvault.emiliano-go.com/development\"\n      }\n    ]\n  }\n]\n</script>\n"
---

# Development

## Building

```bash
make build              # bin/clickvault, current OS/arch
make build-linux-amd64  # bin/clickvault-linux-amd64
make build-linux-arm64  # bin/clickvault-linux-arm64
make sha256             # builds linux/amd64 and prints its sha256sum
```

## Testing

### Unit tests

Unit tests mock the database connection using `go-sqlmock` and do not need
Docker or a running ClickHouse:

```bash
make test
# or
go test ./...
```

### Integration tests

Integration tests spin up a real ClickHouse container (matching
`testdata/docker-compose.yml`) with the Vault SDK's Docker test helper,
and drive the full lifecycle:

1. `Initialize` - configure the connection
2. `NewUser` - create a dynamic user
3. Verify the user can connect to ClickHouse
4. `DeleteUser` - revoke the user
5. Verify the user can no longer connect
6. Repeat `DeleteUser` to confirm idempotency
7. `UpdateUser` on a static-style user with before/after connection checks

Integration tests are gated behind the `integration` build tag:

```bash
go test -tags=integration ./tests/...
```

Docker must be running locally for that command to work.

## Repository layout

```text
clickvault/
├── go.mod, go.sum                    module github.com/emiliano-go/clickvault
├── main.go                           plugin entrypoint
├── internal/clickvault/
│   ├── clickvault.go                  ClickvaultPlugin and six dbplugin methods
│   ├── clickvault_test.go             unit tests (go-sqlmock)
│   ├── ddl.go                         all SQL construction and cluster branching
│   └── ddl_test.go                    table-driven tests for ddl.go
├── testdata/docker-compose.yml        single-node ClickHouse for manual testing
├── tests/integration_test.go          integration tests against real ClickHouse
├── scripts/setup_vault.sh             registers and configures the plugin
└── .github/workflows/ci.yml           go vet, go test, build, sha256 artifact
```
