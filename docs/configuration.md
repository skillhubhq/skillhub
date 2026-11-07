# Hub configuration

SkillHub reads an explicit YAML document with `serve --config PATH` or `check-config --config PATH`. It does not discover `.env` files, recursively merge local overlays or connect to a cloud secret store. Existing command-line flags continue to work.

Precedence: built-in defaults → explicit YAML → explicitly supplied flags. False is a meaningful value. Paths are relative to the process working directory; use absolute paths in deployments. Files are limited to 64 KiB; unknown fields, duplicate keys, multiple documents and invalid types are rejected. Configuration errors do not echo YAML values.

```yaml
data: /var/lib/skillhub/data
listen: 127.0.0.1:8080
public_url: https://skills.example.com
name: Company Skills
private: true
trusted_proxies:
  - 127.0.0.1/32
```

The proxy example is only for a directly connected, same-host proxy. Containers generally listen on `0.0.0.0:8080` inside their isolated network. Restrict external access through the deployment network. Do not use loopback proxy settings for ALB/CloudFront without validating the real chain.

| YAML field | CLI flag | Default |
| --- | --- | --- |
| `database.driver` | `--database-driver` | `sqlite`; also supports `postgres` |
| `database.dsn_file` | `--database-dsn-file` | empty; alternatively `SKILLHUB_DATABASE_DSN` |
| `data` | `--data` | `.local/hub` |
| `listen` | `--listen` | `127.0.0.1:8080` |
| `public_url` | `--public-url` | `http://127.0.0.1:8080` |
| `name` | `--name` | `SkillHub` |
| `private` | `--private` | `true` |
| `trusted_proxies` (list) | `--trusted-proxies` (comma-separated) | empty |
| `oidc.issuer` | `--oidc-issuer` | empty; OIDC disabled |
| `oidc.client_id` | `--oidc-client-id` | empty |
| `oidc.name` | `--oidc-name` | `Microsoft` |
| `oidc.allow_signup` | `--oidc-allow-signup` | `false` (verified first login waits for administrator approval) |
| `oidc.client_secret_file` | YAML only | empty |

The public URL is an origin, with no deployment subpath or query. HTTPS is required outside loopback. Web is embedded; there is no frontend API URL to configure. SQLite is the default and shares the data directory with packages. PostgreSQL is selectable; package storage remains local. See [database drivers and recovery](database.md). No Redis or S3 dependency is required.

## Enterprise login and secrets

```yaml
oidc:
  issuer: https://login.microsoftonline.com/TENANT_ID/v2.0
  client_id: APPLICATION_CLIENT_ID
  name: Microsoft
  allow_signup: false
  client_secret_file: /run/secrets/skillhub-oidc
```

Replace the placeholders. A secret file contains only the secret value, optionally followed by one newline. It must be readable only by the service and trusted operators. Alternatively omit `client_secret_file` and inject `SKILLHUB_OIDC_CLIENT_SECRET`. Configuring both sources is rejected. Inline `client_secret` in YAML is deliberately unsupported. A secret has an 8 KiB bound and is never printed.

AWS ECS can inject a Secrets Manager value into that environment variable; Kubernetes or other service managers can mount a secret file. These are deployment adapters, not cloud SDK dependencies of the Hub. Changes take effect after restart. Register the callback at `PUBLIC_URL/auth/oidc/callback`. First-login provisioning must follow identity-provider access assignment; see [enterprise sign-in](enterprise-login.md).

## Validate, initialize, run

```sh
skillhub-server check-config --config /etc/skillhub/hub.yaml
# Initialize once; use the same data path as the YAML.
skillhub-server init --config /etc/skillhub/hub.yaml --admin admin --password-stdin
skillhub-server serve --config /etc/skillhub/hub.yaml
```

`check-config` checks syntax, settings and secret availability. It creates no database and performs no network calls. It does not certify storage permissions, the correctness of a provider credential or proxy routing. `serve` performs OIDC discovery and fails if the provider cannot be reached. All commands accept the same explicit configuration and database flags. Built-in backup/verify/restore/rollback supports SQLite and PostgreSQL. The controlled `upgrade` command currently supports PostgreSQL only; see matching native tools and the paired recovery procedure in [database operations](database.md).

Runtime limits, TLS termination, backup schedules and log retention are described in [operations](operations.md). The implementation intentionally uses a small typed loader rather than a general configuration framework.

## What an operator can customize

Deployment configuration owns the community name, public origin, storage/listen addresses, public/private reads, proxy trust and enterprise sign-in. Changing the name preserves the Hub's immutable registry ID and content. Web reads the current name; existing App/CLI connections currently cache the display name captured when added, so renames are not automatically synchronized to those saved labels. Client aliases are local names chosen by the user.

Hub administrators manage users, groups, namespace permissions, categories and tags through the application. Namespace administrators manage their team profile and review policy; profile owners edit their personal information. These live settings reside in the Hub database and are not YAML keys.

Custom community logos, homepage copy/layout, legal/contact pages, deployment-wide language/theme defaults and custom download destinations are not exposed as operator settings. Language and theme are per-user/device preferences. The software's GitHub project identity and download channel are separate from the hosting community's name. Forks can change the product source and distribution links under the license; that is a build-time change, not a current administration feature.
