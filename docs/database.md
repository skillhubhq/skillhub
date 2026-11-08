# Database drivers

The Hub supports `sqlite` (default) and `postgres` through Go `database/sql`. The App and CLI's local state continues to use SQLite. Selecting PostgreSQL changes Hub metadata storage, not package storage or client behavior. MySQL and other drivers are not supported yet; additional drivers require dialect, initialization and behavior tests.

## Configure

SQLite needs no connection secret. For PostgreSQL, provision an empty dedicated database/schema and a service role that can create tables, indexes and sequences in it. Use a dedicated persistent `data` directory for immutable packages. The service still runs as **one Hub process**, even with PostgreSQL; do not put multiple Hub nodes in front of separate package directories. A PostgreSQL session advisory lease is acquired before any initialization. All Hub queries and transactions use that physical connection; it cannot reconnect through a pool after losing ownership. Loss detection rejects requests and closes the HTTP server. This serializes database operations and deliberately favors single-node correctness over query concurrency. Stop the old process before starting the new one; this is not HA or rolling failover.

```yaml
data: /var/lib/skillhub/data
public_url: https://skills.example.com
private: true
database:
  driver: postgres
  dsn_file: /run/secrets/skillhub-database
```

The secret file contains a pgx connection string, for example `postgres://USER:PASSWORD@DB_HOST/skillhub?sslmode=verify-full`. Configure trusted CA/client certificates as required by your PostgreSQL service. Alternatively omit `dsn_file` and inject `SKILLHUB_DATABASE_DSN`. Never put passwords in Git, command arguments or YAML. File/env sources are mutually exclusive and bounded to 8 KiB. Connection errors do not echo the DSN.

Run `check-config`, `init --password-stdin` and `serve` with the **same** `--config` path. An empty database receives the complete schema in one transaction. Existing databases must have the supported schema version. `check-config` does not connect to the database. Switching the driver does not transfer data: SQLite-to-PostgreSQL conversion is not implemented. Use a new database/data directory, or plan a separate verified export/import before switching an existing installation. Keep database and package backups together.

## PostgreSQL backup, upgrade and recovery

The server provides paired maintenance commands. Install `pg_dump` **and** `pg_restore` on PATH with the **same major version** as the database (15, 16 or 18 in the CI matrix). This avoids newer dumps containing SQL unsupported by older servers. Maintenance requires a `postgres://` or `postgresql://` URL in the protected DSN source; keyword DSNs and libpq service aliases are not accepted by these commands. The target must use the same PostgreSQL major version and schema name. PostgreSQL major upgrades and SQLite-to-PostgreSQL conversion are separate, unsupported workflows.

Use a dedicated Hub schema: backups refuse unrelated tables/views. Stop the Hub, disable automatic restart, and prevent external writers for the entire operation. Both the data-directory lock and database ownership lock must be acquired. Snapshot consistency covers cooperating Hub processes, not arbitrary DBA writes. Create the parent backup directory first; the destination itself must not exist.

```sh
skillhub-server backup --config /etc/skillhub/hub.yaml \
  --output /srv/backups/skillhub-before-upgrade --timeout 10m
skillhub-server verify-backup --database-driver postgres \
  --input /srv/backups/skillhub-before-upgrade --timeout 10m
```

The directory contains a custom-format `hub.dump`, referenced immutable `packages/`, and `backup.json`. The manifest records registry ID, Hub schema, PostgreSQL major, dump size/SHA-256 and a sorted package digest set. Verification checks the manifest, archive structure, every package digest and package format. It does not connect to a database or replace a restore drill. Secrets/configuration and binaries are **not included**: retain their protected recovery sources and exact binary hashes separately. Backups contain private identity and package data; encrypt, restrict and copy them to independent storage, then verify the copy. Checksums detect corruption, not malicious replacement.

### Replace the server binary

Stop the Hub and make a verified backup with the current executable. Retain that executable and configuration. Start the replacement only after checking that it supports the database schema. The current Hub initializes schema 14 and accepts that exact schema. Unsupported versions are rejected without changing their tables. There is no cross-driver conversion command. Verify readiness, login, permissions and package downloads before reopening traffic.

### Restore / rollback

Provision a **new empty database with the same schema name** and a new data directory. A separate protected recovery config selects that database and directory. Do not reuse the serving DSN. The tool refuses nonempty schemas and existing directories; it never overwrites an installation.

```sh
skillhub-server restore --config /etc/skillhub/recovery.yaml \
  --input /srv/backups/skillhub-before-upgrade --timeout 10m
```

To return to a snapshot, use `restore` into new destinations and select the matching executable and recovery configuration. Reconcile writes, passwords and permission changes since the snapshot before accepting a recovery point.

Restore verifies the copied dump, restores in one transaction, compares actual registry/schema/package references to the manifest, clears all sessions and pending device authorizations, and records `instance.restore`. Accounts, pending enterprise requests, permissions and packages survive. Rejected or disabled identities remain disabled. Filesystem and database recovery markers prevent an incomplete destination from serving, including when another data directory targets the same database. A failed restore preserves its new destination for diagnosis; retry with new destinations, **never remove markers to force startup**.

Before reopening traffic, verify login, old-token rejection, non-admin audience isolation, review workflow, package download/install and registry identity. Record recovery duration and operator checks. Never serve the source and recovered instance concurrently under the same public origin. Configure retention, off-host copies, disk/failure alerts and recovery ownership separately.

## Behavior tests

`SKILLHUB_TEST_POSTGRES_DSN` must target a disposable PostgreSQL installation with permission to create/drop test databases. The HTTP suite creates isolated schemas; maintenance tests create isolated databases. Run `go test -race -count=1 ./internal/hubhttp` and `go test -count=1 -run TestPostgres ./internal/hub` with matching native tools on PATH. CI executes the maintenance binary inside each PostgreSQL 15/16/18 service container, using its matching tools. Tests cover lease-before-DDL, disconnection, paired recovery, session invalidation, corruption/missing packages, wrong identity, failed recovery startup guards, transactional initialization failure and future-schema rejection. Restore assertions include platform duties, scan reports, notification/read state and curated selections. Ordinary `make check` runs SQLite coverage and skips PostgreSQL without an explicit DSN. See [testing](testing.md) for local fixture commands and [deployment checks](deployment-checklist.md) for operator verification.

## Schema contract

The binary, database snapshot and protocol version must match. Initialization creates the complete schema atomically; startup and restore reject unsupported schema versions without modifying them. There are no automatic conversions from other schema versions. Retain a matching executable for every recovery snapshot.
