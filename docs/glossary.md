---
seo:
  title: Glossary - ClickVault Documentation
  canonical: https://clickvault.emiliano-go.com/glossary
  robots: index,follow
  og:
    type: website
    title: Glossary - ClickVault Documentation
    description: ClickHouse - An open-source, column-oriented DBMS for online analytical
      processing OLAP. ClickVault manages users and credentials for ClickHouse clusters...
    url: https://clickvault.emiliano-go.com/glossary
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:width: 1376
    image:height: 768
    image:alt: ClickVault documentation
    site_name: ClickVault Documentation
    locale: en_US
  twitter:
    card: summary_large_image
    title: Glossary - ClickVault Documentation
    description: ClickHouse - An open-source, column-oriented DBMS for online analytical
      processing OLAP. ClickVault manages users and credentials for ClickHouse clusters...
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:alt: ClickVault documentation
    site: '@emiliano_go_'
  description: ClickHouse - An open-source, column-oriented DBMS for online analytical
    processing OLAP. ClickVault manages users and credentials for ClickHouse clusters...
  schema_jsonld:
  - '@context': https://schema.org
    '@type': WebPage
    name: Glossary - ClickVault Documentation
    url: https://clickvault.emiliano-go.com/glossary
    description: ClickHouse - An open-source, column-oriented DBMS for online analytical
      processing OLAP. ClickVault manages users and credentials for ClickHouse clusters...
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
      name: Glossary
      item: https://clickvault.emiliano-go.com/glossary
seo_html: "<title>Glossary - ClickVault Documentation</title>\n<meta name=\"description\"\
  \ content=\"ClickHouse - An open-source, column-oriented DBMS for online analytical\
  \ processing OLAP. ClickVault manages users and credentials for ClickHouse clusters...\"\
  >\n<link rel=\"canonical\" href=\"https://clickvault.emiliano-go.com/glossary\"\
  >\n<meta name=\"robots\" content=\"index,follow\">\n<meta property=\"og:type\" content=\"\
  website\">\n<meta property=\"og:title\" content=\"Glossary - ClickVault Documentation\"\
  >\n<meta property=\"og:description\" content=\"ClickHouse - An open-source, column-oriented\
  \ DBMS for online analytical processing OLAP. ClickVault manages users and credentials\
  \ for ClickHouse clusters...\">\n<meta property=\"og:url\" content=\"https://clickvault.emiliano-go.com/glossary\"\
  >\n<meta property=\"og:image\" content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  >\n<meta property=\"og:image:width\" content=\"1376\">\n<meta property=\"og:image:height\"\
  \ content=\"768\">\n<meta property=\"og:image:alt\" content=\"ClickVault documentation\"\
  >\n<meta property=\"og:site_name\" content=\"ClickVault Documentation\">\n<meta\
  \ property=\"og:locale\" content=\"en_US\">\n<meta name=\"twitter:card\" content=\"\
  summary_large_image\">\n<meta name=\"twitter:title\" content=\"Glossary - ClickVault\
  \ Documentation\">\n<meta name=\"twitter:description\" content=\"ClickHouse - An\
  \ open-source, column-oriented DBMS for online analytical processing OLAP. ClickVault\
  \ manages users and credentials for ClickHouse clusters...\">\n<meta name=\"twitter:image\"\
  \ content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\">\n<meta\
  \ name=\"twitter:image:alt\" content=\"ClickVault documentation\">\n<meta name=\"\
  twitter:site\" content=\"@emiliano_go_\">\n<script type=\"application/ld+json\"\
  >\n[\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"WebPage\"\
  ,\n    \"name\": \"Glossary - ClickVault Documentation\",\n    \"url\": \"https://clickvault.emiliano-go.com/glossary\"\
  ,\n    \"description\": \"ClickHouse - An open-source, column-oriented DBMS for\
  \ online analytical processing OLAP. ClickVault manages users and credentials for\
  \ ClickHouse clusters...\",\n    \"image\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  ,\n    \"publisher\": {\n      \"@type\": \"Organization\",\n      \"name\": \"\
  Emiliano Gandini Outeda\",\n      \"logo\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  \n    }\n  },\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"\
  BreadcrumbList\",\n    \"itemListElement\": [\n      {\n        \"@type\": \"ListItem\"\
  ,\n        \"position\": 1,\n        \"name\": \"Glossary\",\n        \"item\":\
  \ \"https://clickvault.emiliano-go.com/glossary\"\n      }\n    ]\n  }\n]\n</script>\n"
---

# Glossary

**ClickHouse** - An open-source, column-oriented DBMS for online analytical
processing (OLAP). ClickVault manages users and credentials for ClickHouse
clusters through the Vault database secrets engine.

**Cluster (ClickHouse)** - A multi-node ClickHouse deployment. When a
connection config includes a `cluster` field, ClickVault automatically
appends `ON CLUSTER '<name>'` to every generated DDL statement.

**Creation statements** - SQL statements in a dynamic role definition that
create a new database user and grant the appropriate permissions. Required
for every dynamic role; there is no built-in default.

**Database secrets engine** - A Vault secrets engine that generates
dynamic database credentials on demand and manages the lifecycle of those
credentials (creation, rotation, revocation).

**DDL** - Data Definition Language. SQL statements like `CREATE USER`,
`ALTER USER`, `DROP USER`, `GRANT`, and `REVOKE`.

**Dynamic role** - A Vault database role that creates ephemeral ClickHouse
users on demand. Each lease produces a unique user with a configurable TTL;
the user is automatically dropped when the lease expires or is revoked.

**Plugin RPC** - The protocol buffer-based remote procedure call interface
that Vault uses to communicate with database plugins. ClickVault implements
the `sdk/database/dbplugin/v5` interface.

**Revocation statements** - SQL statements that clean up a dynamic user.
Defaults to `DROP USER IF EXISTS "{{username}}"` when not set in the role
definition.

**Rotation statements** - SQL statements for static roles that change the
user's password. Defaults to `ALTER USER "{{username}}" IDENTIFIED WITH
sha256_password BY '{{password}}'`.

**Static role** - A Vault database role that manages an existing long-lived
ClickHouse user. Vault rotates the user's password on a configurable
schedule (the `rotation_period`).

**Username template** - A Go template string used to generate usernames for
dynamic users. Supports `.DisplayName`, `.RoleName`, `random N`, `unix_time`,
`truncate N`, and `uppercase` functions.

**Vault** - HashiCorp Vault, a tool for secrets management, encryption, and
access control. ClickVault integrates with Vault's database secrets engine
to manage ClickHouse credentials.
