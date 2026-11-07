# Deployment checklist

Use this checklist when installing a private, single-instance Hub. Keep environment addresses, credentials, operators and completed reports outside the source repository. Test the exact candidate revision on the target environment.

This is an operator's environment-integration checklist, not the software release gate. Real Entra, server and managed-device checks may remain pending while repository-level acceptance is completed. The release criteria are the code, security, packaging and supported-platform checks in [testing](testing.md); do not report an untested environment as validated.

## Real Entra application and identity lifecycle

- [ ] Assign an application owner and Hub access approver; create the single-tenant application through the approved administrative process.
- [ ] Record the tenant-specific issuer, client ID and exact HTTPS callback; deliver the secret through controlled storage. Verify no secret reaches Web bundles, CLI/App, logs or Git.
- [ ] Require application assignment; define employee/guest eligibility and applicable MFA policy. Verify assigned and unassigned test users separately.
- [ ] Complete verified login → pending Hub approval → ordinary membership → new login. Confirm approval does not grant publishing/review/platform duties.
- [ ] Verify Web login and CLI/App device authorization against the actual reverse proxy. Exercise expired/cancelled authorization, failed callback and replay without logging sensitive tokens.
- [ ] Verify Hub disable invalidates browser/CLI/App sessions. Entra-only disable does not currently immediately revoke existing Hub sessions: record the person/process that disables both. If automatic immediate revocation is mandatory, directory synchronization is a blocker.
- [ ] Retain a controlled emergency local administrator, test access with the IdP unavailable, and document custody/rotation. Do not merge local and SSO accounts by email.

## Target server, isolation and recovery

- [ ] Provision the approved dedicated PostgreSQL and encrypted durable volume; select a tested database major and matching dump/restore tools. Do not share another service's database/schema.
- [ ] Validate least-privilege service credentials, secret-file permissions, HTTPS, database TLS or reviewed same-host isolation, and the exact trusted proxy boundary.
- [ ] Confirm stable registry identity/origin and durable package storage across process/host restart. Use stop-old/start-new replacement, not concurrent rolling instances.
- [ ] Configure process restart and authenticated catalog/download probes. Verify distinct users retain independent rate limits behind the real proxy.
- [ ] Assign disk/backup-failure/service-failure alert receivers. Test disk-full and unavailable-database behavior using isolated target fixtures.
- [ ] Define RPO, RTO, backup cadence, retention and an independent encrypted copy. PostgreSQL paired backups require the application stopped; measure this maintenance window before accepting the schedule.
- [ ] Restore a paired backup into fresh isolated database/storage. Verify registry identity, package digests, roles, scan reports, notifications and featured selections; old sessions must fail and fresh login must work.
- [ ] Exercise candidate upgrade and rollback with the exact previous binary/configuration. Reconcile post-snapshot user disable/role/password changes before reopening access. Rollback restores a pre-upgrade snapshot, not a down migration.

## Product and candidate acceptance

- [ ] Build two realistic namespaces and independent access groups with approved synthetic content. Verify anonymous, ordinary member, restricted Reader, maintainer, independent reviewer, people administrator, auditor and Hub administrator.
- [ ] Verify that search, recent/featured lists, detail, comments, downloads and review-task totals respect access; revoked roles and withdrawn versions disappear. Own submissions must never become self-approval tasks.
- [ ] Complete submission → review task → changes requested → corrected version → approval notification → exact-version installation → update → withdrawal/unavailability warning. Preserve local files throughout errors and revocation.
- [ ] Measure realistic package sizes, browser page behavior and concurrent publication/download latency. Measure against your own latency and capacity requirements.
- [ ] Fix a clean candidate revision and run the full CI/platform matrix before publishing artifacts. Record hashes/provenance and the exact supported client/Hub versions.
- [ ] On a clean macOS device, verify browser-downloaded manual DMG installation and replacement with normal security policy, preserving sources, drafts, preferences and installed files. Signing/notarization/automatic installation remain outside the free manual-preview route.
- [ ] Use the actual distribution App identity and normal browser quarantine path, not the isolated smoke bundle. After replacement and restart, authenticate two connections from saved OS credentials, reopen saved and incomplete drafts, and compare global/project installations and locally edited files. Confirm the manual installation policy is acceptable to the trial cohort's IT administrators.
- [ ] If Windows is in the first cohort, complete real Windows/WebView2 installation, credential-store and update acceptance; otherwise explicitly scope the deployment to supported verified platforms.

## Operating responsibilities

Name the support owner, backup owner, access approver and independent reviewer before inviting users. Publish the maintenance window and escalation route. Keep a small trial cohort until login, permissions, recovery, distribution and one full publishing/install cycle have recorded evidence. Email reminders, SCIM, custom role builders, public-community moderation and automatic App installation are not silently included in this supported product scope.
