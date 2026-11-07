# Security policy

## Reporting a vulnerability

Use [GitHub private vulnerability reporting](https://github.com/skillhubhq/skillhub/security/advisories/new) when it is enabled and available to you. If that page is unavailable, contact a repository maintainer through an existing private channel to arrange a secure report. Do not put exploit details, secrets or private skill content in a public issue. There is no guaranteed response-time SLA during preview.

Include the affected version, deployment mode, reproduction steps, impact and a minimal example with synthetic data. Never include real tokens, passwords, cookies or full private packages.

## Supported versions

The project is in preview. Security fixes target the newest maintained code and the next release; older previews do not have an independent backport commitment. A published preview is not a guarantee of production readiness. Review release notes and the current acceptance evidence before deployment.

## Security boundaries

Hub enforces publication and download permissions. CLI/App preserve local edits, check package digests and do not execute package scripts. Installing a skill does not make its instructions trustworthy; review skills before using them with an agent. A withdrawn skill cannot be remotely removed from devices that already downloaded it.

CLI/App run with the local user's permissions. The client data directory and selected project directories are operator-controlled inputs; keep client state private to that user. Archive entries and project manifests are untrusted and must pass path, symlink and integrity checks before installation. Build, packaging and test utilities are maintainer tools: their executable and output arguments intentionally select local programs and files, and are not exposed as Hub APIs.

Use HTTPS, private mode for internal content, restricted persistent storage and independent verified backups. Enterprise identity proves who a user is; Hub permissions remain separate. Entra account removal does not instantly revoke existing Hub sessions; disable the Hub user for immediate revocation. See the enterprise login and operations guides.

Release checksums detect corruption; they alone do not authenticate a publisher. GitHub build attestations, when present, provide separate provenance. Desktop signing/notarization is not configured for preview releases.
