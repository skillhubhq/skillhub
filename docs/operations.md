# Single-node Hub operations

This document describes SQLite snapshot maintenance. PostgreSQL deployments use the paired database/package procedure in [Database drivers](database.md).

See [configuration](configuration.md) for explicit YAML, CLI overrides and secret-file/environment injection.

## Deployment and data ownership

Run one Hub process with a local writable data directory under a dedicated OS account. SQLite WAL and the package directory must stay on the same host; shared network filesystems and multiple serving replicas are outside the single-node deployment model. An existing reverse proxy can terminate HTTPS. Do not expose the loopback HTTP default directly on an external interface.

```sh
skillhub-server serve --data /srv/skillhub/data \
  --listen 127.0.0.1:8080 --public-url https://skills.example.com \
  --name "Company Skills" --private=true --trusted-proxies 127.0.0.1/32
```

Initialize the database once using `init --password-stdin` as described in [usage](usage.md). Keep the public URL stable: clients bind a source to its canonical URL and immutable registry ID. Public instances use `--private=false`; only published public releases become anonymous. All publication and administration still require authentication.

The data directory contains `hub.db`, SQLite WAL/SHM while running, and content-addressed `packages/*.zip`. Accounts, permissions, categories, tags, versions, reviews, idempotency records, audits, session hashes and instance secrets live in the database. Service flags, TLS certificates, reverse-proxy settings and service-manager configuration are external and need separate configuration backup. Backup data contains private packages and password/session material and should have access controls at least as strict as the live directory.

## Online backup

Create the parent backup directory first. The output directory itself must not exist, even if empty, and must be outside the live data directory. Run the backup command using the same binary version as the active Hub.

```sh
skillhub-server backup --data /srv/skillhub/data \
  --output /srv/skillhub-backups/before-upgrade --timeout 10m
skillhub-server verify-backup --input /srv/skillhub-backups/before-upgrade
```

Backup uses SQLite `VACUUM INTO` for a consistent live database snapshot, then copies only immutable package blobs referenced by that snapshot, including pending/rejected/yanked versions. Versions sharing a digest need only one blob copy. Publication may continue while the snapshot is copied. The package implementation does not garbage-collect referenced blobs. A future garbage collector must coordinate with this workflow. The SQLite behavior is documented in [VACUUM INTO](https://www.sqlite.org/lang_vacuum.html#vacuum_with_an_into_clause).

`backup.json` records format/schema, registry ID, snapshot timestamp, database checksum/size and unique package count. Verification checks the database checksum, SQLite integrity, schema/identity, foreign keys, every referenced package digest and package structure. Verification opens the snapshot read-only, without replaying WAL or modifying it. A live SQLite sidecar, incomplete marker, unsupported manifest or missing/corrupt package makes verification fail.

Completed backup directories are portable. Copy the entire directory to separately managed storage and run `verify-backup` on the copied result. Checksums detect corruption; they are not signatures or proof of the backup's origin. Restore only backups from a trusted operator-controlled store. Allow free space for the database snapshot plus all referenced compressed packages; validation expands one bounded package at a time. The operation has a configurable deadline and returns nonzero on timeout/cancellation.

The tool does not choose retention or delete old backups. Schedule and retain them through the organization's existing backup system. Run periodic restore drills; a verified archive is necessary but does not prove your service configuration can be recovered.

## Restore and cutover

Restore into a **new directory** using a binary supporting the backup schema. The source backup remains unchanged. The command does not overwrite or merge into an existing Hub directory.

```sh
skillhub-server verify-backup --input /srv/skillhub-backups/before-upgrade
skillhub-server restore --input /srv/skillhub-backups/before-upgrade \
  --data /srv/skillhub/recovered-before-upgrade --timeout 10m
```

The recovery preserves registry identity, accounts/passwords, governance, package bytes and historical audit/idempotency data. It removes **all sessions and pending device authorizations** and adds an `instance.restore` audit event. Clients must log in again; previously revoked sessions cannot become live because they were present in an older backup. Changes after the snapshot, including permission changes and password changes, are absent and must be reconciled before reopening access.

1. Verify and restore into the new directory while keeping the current directory available for rollback.
2. Stop the old serving process, or otherwise ensure the restored instance cannot serve the same origin concurrently.
3. Point the service configuration at the restored directory; keep the previous `--public-url`, private/public mode and reverse-proxy configuration.
4. Start the Hub, check `/healthz`, sign in through CLI/App, verify the registry ID, private access restrictions, review history and an exact-version package download.
5. Reconcile post-snapshot governance changes, then reopen normal access. Keep the original directory and backup according to retention policy.

For an isolated drill, use another loopback port, a temporary client directory and a temporary source alias. Do not rebind existing production clients to a drill origin.

The `.skillhub-maintenance` marker blocks service startup while a destination is incomplete. Normal errors clean up only the newly created destination. A force-kill/power loss can leave that marked directory behind: preserve it if needed for diagnosis, then retry into another new directory. Do not remove the marker and attempt to serve partial data. Serving a completed backup directory directly is also refused; use `restore` first.

## Upgrade and rollback

1. Make and verify a backup using the current binary before starting an upgraded binary.
2. Stop the old process, retain the old binary/configuration, then start the new binary against the live data directory.
3. Check startup, login, catalog permissions and a download. Empty databases are initialized transactionally. Existing instance identity is preserved; unsupported database versions are rejected.
4. To roll back a schema change, stop the new process and restore the pre-upgrade backup into a new directory using a compatible binary. Do not run an old binary against a newer schema. Reconcile writes accepted after the backup before selecting the rollback point.

The Hub and shared client validate their own schema versions. The backup manifest records the actual Hub schema. Restore preserves the snapshot structure and identity; verification never rewrites a snapshot.

## Diagnostics and limits

- `/healthz` reports process availability. It is not an authorization, disk-capacity or package-integrity check; use a real authenticated catalog/download check and backup verification for those.
- The Hub writes structured JSON startup/panic diagnostics to stderr and uses request IDs. Do not enable proxy request-body or authorization-header logging. Logs are not a replacement for the authenticated governance audit API.
- HTTP defaults: 16 KiB headers, 10-second header timeout, 60-second read/write/idle timeouts, 32 MiB total request body. Connect decoding further limits identity/registry calls to 8 KiB, catalog/admin to 64 KiB and publishing to 32 MiB.
- Packages: 20 MiB ZIP, 80 MiB total expansion, 8 MiB per file, 2,048 entries, 256 KiB primary `SKILL.md`. Upload concurrency and authentication rate limiting are bounded.
- Forwarded addresses are ignored by default. Configure `--trusted-proxies` with only the actual direct proxy CIDRs. The trusted edge must overwrite `X-Forwarded-For`; the Hub walks a valid chain from right to left to the first untrusted address. Invalid chains fall back to the direct peer. Never trust `0.0.0.0/0` or a broad employee subnet. Confirm distinct clients have independent authentication limits through the actual proxy.
- For source failures, check the configured HTTPS origin, stable registry ID, current login, OS credential-store availability and required namespace/audience membership. Never export tokens or package contents with a diagnostic report.
- For local install failures, inspect the exact plan/path and preserve local edits. Do not manually delete transaction journals to bypass recovery checks.

See [testing guide](testing.md) and the executable smoke test in [usage](usage.md) for repeatable isolated verification.


## Account lifecycle and emergency recovery

Web and App **Settings → Account security** allow local users to change their password. The current password is required. Hub administrators can reset another local user's password and promote/demote existing accounts in **Studio → Hub settings → People**, including granting administrative access to existing SSO users. All password and administrator changes revoke that user's sessions and approved device grants. The last enabled administrator cannot be demoted. SSO passwords remain with the identity provider and cannot be replaced with a local password. Reset passwords must be delivered through an organization's approved private channel; the Hub does not send email or implement password expiry.

CLI equivalents: `skillhub password-change --source work --passwords-stdin` reads the current and new password on two stdin lines; `skillhub admin user-password-reset USER_ID --source work --password-stdin` reads the new password; `skillhub admin user-administrator USER_ID --source work --enabled=true` grants administrator access (`false` revokes). Supply secrets from a password manager through stdin, never shell arguments or shell history. Re-authenticate the affected connection after a successful change.

If all local administrator credentials are lost, stop the service first, make and verify a backup, then run `skillhub-server recover-admin --data /var/lib/skillhub/data --username EXISTING_LOCAL_USERNAME --password-stdin` as the service OS account. It restores an **existing local account**, enables it, grants administrator access, sets the supplied password, revokes its sessions/devices and records an audit event. It refuses SSO accounts and missing usernames. A shared exclusive filesystem lock prevents recovery while this version of the server is serving. Confirm the serving process has stopped before recovery. Restart the service and verify login. Filesystem access to the Hub data is an administrative privilege; keep it restricted.

## Candidate preparation and internal-server acceptance

`deploy/skillhub.service` and `deploy/nginx.conf.example` are templates, not installed services. Create a dedicated OS user; install a verified binary under `/opt/skillhub`; create `/var/lib/skillhub/data` owned by that user; initialize once; set the real domain/TLS/tenant configuration before enabling the service. For Entra, configure an `auth.providers` OIDC entry with the documented tenant-specific issuer/client ID and a protected secret file or explicit `client_secret_env`; do not put a secret in a checked-in unit or command line. See [enterprise sign-in](enterprise-login.md).

From an authorized, clean revision, run `python3 scripts/build_candidate.py --output .local/candidate-<revision> --arch amd64`. It builds embedded Web assets and Linux Hub/CLI, and records the Git SHA, source hash, toolchain, target and binary checksums. `--allow-dirty` explicitly produces **local-only diagnostic artifacts**, never a release candidate. Hashes provide integrity, not authenticity. App builds use `node desktop/build.mjs` on their supported host and must use the same revision. Verify checksums after copying artifacts; keep the prior binaries and verified backup for rollback.

When an operator prepares a specific installation, record the following on its actual target. These are deployment follow-up checks, not repository-level release prerequisites:

1. Candidate SHA/checksums, OS/architecture, service account, domain, proxy/TLS, data path, permissions, capacity and service restart after reboot.
2. Real Entra tenant: assigned/unassigned user, first login, repeat identity, disabled user, MFA/guest policy, Web login returning to an exact skill and CLI/App device login. Existing SSO Hub sessions last at most eight hours; disable the Hub account promptly for immediate revocation. SCIM is outside this version.
3. Distinct clients behind the real proxy: concurrent authentication/polling, bounded abuse, large valid upload, private access isolation, namespace-admin user lookup, independent review, publication, comment/reply, installation/update/removal on both supported tools, withdrawal and revoked access.
4. Scheduled verified backup, independent copy, restore into a new directory, preserved registry identity/community/avatars/permissions, invalid old sessions, exact package download, measured restore time and upgrade/rollback drill. Assign retention, disk/backup-failure alerts and an operator.

Do not treat a macOS test run, a Linux cross-build, a health check or a historical CI run as this target acceptance. The candidate manifest deliberately leaves `target_acceptance=not_run`; keep the signed-off target report alongside the manifest after execution.

## Local connection names

App **Settings → Connections → Manage connection** can edit a device-local connection name and note. Leave the name blank to use the Hub-provided name. **Refresh Hub information** anonymously checks the original address and stable registry identity before refreshing the site name/access mode; it never replaces the saved account.

CLI equivalents are `skillhub source edit work --name "Team knowledge" --note "Review account"` and `skillhub source refresh work`. Empty values clear a field. Preferences stay on the device and work offline; do not put credentials in notes.

The CLI ID (for example `work`) remains stable because commands, project manifests and existing installations reference it. The editable display name is used by App connection lists and the Hub switcher. Hub URL and registry identity are read-only; use a separate connection for another address. Site names, usernames and permissions are server-managed, separate from local preferences. Web serves one Hub and has no device connection editor.
