---
seo:
  title: Roles - ClickVault Documentation
  canonical: https://clickvault.emiliano-go.com/roles
  robots: index,follow
  og:
    type: website
    title: Roles - ClickVault Documentation
    description: Dynamic roles ephemeral credentials
    url: https://clickvault.emiliano-go.com/roles
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:width: 1376
    image:height: 768
    image:alt: ClickVault documentation
    site_name: ClickVault Documentation
    locale: en_US
  twitter:
    card: summary_large_image
    title: Roles - ClickVault Documentation
    description: Dynamic roles ephemeral credentials
    image: https://clickvault.emiliano-go.com/assets/images/og-image.png
    image:alt: ClickVault documentation
    site: '@emiliano_go_'
  description: Dynamic roles ephemeral credentials
  schema_jsonld:
  - '@context': https://schema.org
    '@type': WebPage
    name: Roles - ClickVault Documentation
    url: https://clickvault.emiliano-go.com/roles
    description: Dynamic roles ephemeral credentials
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
      name: Roles
      item: https://clickvault.emiliano-go.com/roles
seo_html: "<title>Roles - ClickVault Documentation</title>\n<meta name=\"description\"\
  \ content=\"Dynamic roles ephemeral credentials\">\n<link rel=\"canonical\" href=\"\
  https://clickvault.emiliano-go.com/roles\">\n<meta name=\"robots\" content=\"index,follow\"\
  >\n<meta property=\"og:type\" content=\"website\">\n<meta property=\"og:title\"\
  \ content=\"Roles - ClickVault Documentation\">\n<meta property=\"og:description\"\
  \ content=\"Dynamic roles ephemeral credentials\">\n<meta property=\"og:url\" content=\"\
  https://clickvault.emiliano-go.com/roles\">\n<meta property=\"og:image\" content=\"\
  https://clickvault.emiliano-go.com/assets/images/og-image.png\">\n<meta property=\"\
  og:image:width\" content=\"1376\">\n<meta property=\"og:image:height\" content=\"\
  768\">\n<meta property=\"og:image:alt\" content=\"ClickVault documentation\">\n\
  <meta property=\"og:site_name\" content=\"ClickVault Documentation\">\n<meta property=\"\
  og:locale\" content=\"en_US\">\n<meta name=\"twitter:card\" content=\"summary_large_image\"\
  >\n<meta name=\"twitter:title\" content=\"Roles - ClickVault Documentation\">\n\
  <meta name=\"twitter:description\" content=\"Dynamic roles ephemeral credentials\"\
  >\n<meta name=\"twitter:image\" content=\"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  >\n<meta name=\"twitter:image:alt\" content=\"ClickVault documentation\">\n<meta\
  \ name=\"twitter:site\" content=\"@emiliano_go_\">\n<script type=\"application/ld+json\"\
  >\n[\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"WebPage\"\
  ,\n    \"name\": \"Roles - ClickVault Documentation\",\n    \"url\": \"https://clickvault.emiliano-go.com/roles\"\
  ,\n    \"description\": \"Dynamic roles ephemeral credentials\",\n    \"image\"\
  : \"https://clickvault.emiliano-go.com/assets/images/og-image.png\",\n    \"publisher\"\
  : {\n      \"@type\": \"Organization\",\n      \"name\": \"Emiliano Gandini Outeda\"\
  ,\n      \"logo\": \"https://clickvault.emiliano-go.com/assets/images/og-image.png\"\
  \n    }\n  },\n  {\n    \"@context\": \"https://schema.org\",\n    \"@type\": \"\
  BreadcrumbList\",\n    \"itemListElement\": [\n      {\n        \"@type\": \"ListItem\"\
  ,\n        \"position\": 1,\n        \"name\": \"Roles\",\n        \"item\": \"\
  https://clickvault.emiliano-go.com/roles\"\n      }\n    ]\n  }\n]\n</script>\n"
---

# Roles

## Dynamic roles (ephemeral credentials)

Dynamic roles create short-lived ClickHouse users on demand. Each lease
produces a unique user that is automatically dropped when the lease expires
or is revoked.

### Creation statements

There is no built-in default: a dynamic role must always set
`creation_statements`, since only the operator knows which database and
role the new user should be granted.

Single node:

```sql
CREATE USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}';
GRANT analytics ON default.* TO "{{username}}";
```

With a `cluster` configured, ClickVault turns the same statements into:

```sql
CREATE USER "{{username}}" ON CLUSTER 'prod' IDENTIFIED WITH sha256_password BY '{{password}}';
GRANT ON CLUSTER 'prod' analytics ON default.* TO "{{username}}";
```

You write the single-node version; ClickVault inserts the `ON CLUSTER` clause
at the position ClickHouse's grammar requires.

### Revocation statements

If a role does not set `revocation_statements`, ClickVault falls back to:

```sql
DROP USER IF EXISTS "{{username}}";
```

Because `DROP USER IF EXISTS` does not error on a missing user, deleting a
user that is already gone is not an error. This holds for custom statements
too, as long as they also use `IF EXISTS`.

## Static roles (managed rotation)

Static roles manage an existing long-lived ClickHouse user. Vault rotates
the user's password on a configurable schedule.

### Rotation statements

If a static role does not set `rotation_statements`, ClickVault falls back to:

```sql
ALTER USER "{{username}}" IDENTIFIED WITH sha256_password BY '{{password}}';
```

(With `ON CLUSTER '<cluster>'` inserted after the user name when a cluster
is configured.)

## Statement templates

Vault roles supply the actual SQL ClickVault runs, using these placeholders:

| Placeholder | Description |
|---|---|
| `{{username}}` or `{{name}}` | The generated or managed username |
| `{{password}}` | The generated password |
| `{{expiration}}` | (NewUser only) Credential expiration timestamp |

A raw statement string can contain multiple `;`-separated statements; each
one is templated and executed independently.

## Security

ClickHouse DDL cannot be parameterized, so ClickVault substitutes the
generated username and password into the statement as literal text. It
rejects any value containing a single quote, double quote, backtick,
backslash or control character; a user create/rotate will fail rather than
run unsafe SQL.
