# Governance

The Hub owns publication checks, independent review, audit and platform duties. Web, App and CLI use the same permission-enforced APIs. No worker or mail service is required.

## Package checks and approval

Every new submission is inspected with the bounded, offline `builtin-static/1` scanner after archive validation. Nothing in the package is executed or sent to a third party. A report records the immutable package digest, engine version, timestamp, rule ID, file and line, and fixed remediation text; matched secret text is never copied into findings.

- Private-key headers and selected credential-shaped tokens block approval. Fix the package and submit a new immutable version; even administrators cannot override this gate.
- Remote shell pipelines, destructive command patterns and instruction-override language require independent review. Binary files are explicitly uninspected. These findings force pending review even in a namespace that normally permits direct publication.
- A reviewer must record a note for warning/incomplete reports. The normal author/reviewer separation still applies. A missing, outdated or mismatched report blocks approval until a successful rescan.
- A clean report means **no built-in rules matched**, not that the skill is safe. Static patterns cannot establish intent, detect all credentials or prevent prompt injection. This is not antivirus, sandboxing or an execution permission system.
- Reports cap findings at 100. Finding overflow blocks approval rather than silently producing a clean result. Existing published packages are not retroactively withdrawn. Pending versions without a valid report require an explicit scan. Failed/cancelled scans return an error and cannot create a successful report.
- Reports are visible only to users with management/review access to that namespace; scanner operations use the same authorization boundary, two-operation expensive-request gate and per-address rate limit. No anonymous report endpoint.

Web/App show a compact expandable report above the submission body. The approval action stays disabled until the report and required review note are available. Diagnostic rule identifiers and fixed findings remain English; the surrounding UI supports English and Chinese.

## Review notifications

A review decision and the submitter’s notification commit in the same transaction. Idempotent review retries do not create duplicate notifications. The inbox is scoped to the selected connection and authenticated user, paginated, and includes approved/requested-changes outcomes. Opening one marks it read and links to the exact version’s feedback.

Both listing and marking read recheck current namespace access. Revocation hides an old notification; no copied skill body is retained in the inbox. Notifications persist across restarts and backup/restore. Email, webhooks and desktop push are not supported. The inbox also provides a live, permission-filtered review-task queue, a pending count and the oldest submission date. It excludes self-submissions and updates on read, focus and periodic visible-window refresh. It is a pull-based task list, not guaranteed push delivery.

## Platform duties

| Duty | Permitted | Explicitly excluded |
| --- | --- | --- |
| Hub administrator | Existing full administration; assign/revoke platform duties; password reset | Self-approval; bypassing scan blockers |
| People administrator | List accounts, create ordinary accounts, approve/reject enterprise requests, enable/disable ordinary accounts | Hub/platform role assignment, group grants, namespace administration, password resets, changing self or privileged platform accounts |
| Auditor | Read audit log | Account mutation, grants, additional skill content access |

Platform duties do not grant namespace access. Only a Hub administrator assigns them, and any role change revokes the affected user’s Hub sessions/device authorizations. Unknown roles fail closed. Group administration remains a Hub administrator responsibility; namespace administration remains namespace-scoped. There is no generic custom-role builder or automatic Entra group mapping.

Enterprise identity still uses verified issuer/subject and default pending approval. A people administrator can approve an ordinary member but cannot grant publishing/review authority. Microsoft-side offboarding does not automatically revoke existing Hub sessions: operations must disable the Hub account as well. Password reset is deliberately restricted to Hub administrators to avoid account takeover through a personnel duty.

## CLI

All commands use a separately authenticated source; `--json` provides structured output.

```sh
skillhub governance access --source company
skillhub governance scan-report RELEASE_ID --source company
skillhub governance scan-run RELEASE_ID --source company
skillhub governance notifications --source company
skillhub governance notification-read NOTIFICATION_ID --source company
skillhub governance role USER_ID people-admin --source company
skillhub governance role USER_ID auditor --remove --source company
```

A Hub without the required capability returns an explicit unsupported state. Missing scan evidence cannot be treated as successful inspection, and approval remains blocked regardless of client behavior.

## Discovery and curation

Review tasks include only pending versions the current user can independently review. Pagination, totals and the oldest date use the same access rules. Curation requires Hub administration; people administration or namespace review does not grant it. Curating a skill never changes its audience.

```sh
skillhub search --source company --order recent
skillhub search --source company --featured
skillhub governance review-tasks --source company
skillhub governance feature platform release-check --source company
skillhub governance feature platform release-check --source company --remove
```

Audit, scan reports, notifications and curated selections participate in the Hub backup. See [testing](testing.md), [database maintenance](database.md) and [deployment checks](deployment-checklist.md).
