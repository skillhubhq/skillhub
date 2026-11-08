# Governance

The Hub owns publication checks, independent review, audit and platform duties. Web, App and CLI use the same permission-enforced APIs. No worker or mail service is required.

## Package checks and approval

Every new submission is inspected with the bounded, offline `builtin-static/2` scanner after archive validation. Nothing in the package is executed or sent to a third party. A report records the immutable package digest, engine version, timestamp, rule ID, file and line, and fixed remediation text; matched secret text is never copied into findings.

- Private-key headers and selected credential-shaped tokens block approval. Fix the package and submit a new immutable version; even administrators cannot override this gate.
- Remote shell pipelines, local process execution/termination, external HTML scripts, destructive command patterns and instruction-override language require independent review. Binary files are explicitly uninspected. These findings force pending review even in a namespace that normally permits direct publication.
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

## Upstream attribution

A curated version distinguishes three identities:

| Identity | Meaning |
| --- | --- |
| Original author | A source-backed name and HTTPS homepage; this does not create a Hub account or imply endorsement |
| Curating maintainer | The publishing namespace and its team profile; namespace grants control who maintains the copy |
| Operator | The authenticated submitting or reviewing account, retained in audit; an external author name never grants access |

Web and App show the same source directory, full upstream Git commit, SPDX license identifier, packaged license path, modification statement and Hub version. Cards credit the original author; team and local-library views retain that attribution. Curated approval notes can be read from the version's source panel. The displayed attribution is the curator's reviewed declaration, not an upstream verification badge.

Publish the original directory with all license/copyright notices. Do not edit `SKILL.md` merely to add Hub attribution: submit metadata separately through the publishing form or `--provenance FILE`. The JSON file is explicit input; the CLI does not automatically trust package-provided attribution.

```json
{
  "author_name": "Example Authors",
  "author_url": "https://example.org/authors",
  "source_url": "https://example.org/source/0123456789abcdef0123456789abcdef01234567/skill",
  "revision": "0123456789abcdef0123456789abcdef01234567",
  "license": "Apache-2.0",
  "license_file": "LICENSE.txt",
  "modified": false,
  "changes": ""
}
```

```sh
skillhub publish ./example-skill --source community --namespace curators \
  --version 0.0.0-upstream.0123456789ab --visibility public \
  --provenance ./attribution.json
```

The Hub requires the referenced text license file to exist in the package. A reviewer still checks license applicability, source ownership, exact commit and preservation of notices; the Hub does not fetch or independently verify the upstream repository. Changed files require `modified: true` and a change description. Every attributed submission requires independent review, even in a namespace allowing direct publication. Metadata is immutable with its version and participates in idempotency checks. A correction requires a new version.

When upstream supplies no release number, use a clearly identified Hub snapshot version such as `0.0.0-upstream.<commit-prefix>`; do not invent an upstream `1.0.0`. Installation stores the attribution alongside the exact package digest for offline display. Older Hubs lacking `catalog.provenance.v1` are rejected when publishing attribution, rather than silently losing it.

## Team archive and permanent cleanup

A team is a namespace with a display profile. **Only Hub administrators** can preview, archive, restore or permanently clean up a team. Namespace administrators and platform people administrators cannot perform these operations. Web and App expose **Studio → Hub settings → Namespace → Team lifecycle**; the CLI uses the same Hub-enforced workflow.

| State/action | Content and access |
| --- | --- |
| Archive | Hides the team and all its versions from community discovery, direct downloads, contributions, evaluation views and review tasks. Publishing, reviewing, profile changes, role changes and interactions stop, including through old clients. Content, prior version states and role grants remain stored. |
| Restore | Reactivates the retained grants and prior version states. Withdrawn/rejected versions stay withdrawn/rejected; pending work returns to the queue. Review permissions are rechecked. |
| Permanently delete | Requires an archived team, its current revision and the exact namespace typed again. Deletes all its releases, discussions/replies, reactions, review notifications, scans, evaluation evidence, content reports, profile, curation and namespace grants. The name remains reserved and cannot be reused. |

Shared Hub user accounts, external-login identities, access groups, group memberships and audit events are **never deleted** by team cleanup. A personal namespace that is deleted is not recreated on the user's next login. Other teams are unaffected. Installed skill files on users' devices remain untouched; the Hub denies new downloads, updates and cache reuse for unavailable versions. A local update check reports the version as unavailable rather than silently replacing it.

The dialog previews counts before changing anything. Revisions prevent a stale archive/restore/delete request from overriding a later lifecycle change. CLI equivalents:

```sh
skillhub admin namespace-inspect research --source company
skillhub admin namespace-archive research --source company --revision 1 --yes
skillhub admin namespace-restore research --source company --revision 2 --yes
# Archive again before permanent cleanup; inspect for the current revision.
skillhub admin namespace-purge research --source company --revision 4 --confirm research --yes
```

Permanent cleanup has no product undo. Back up before deleting business content; recovery requires the operator's paired backup and must reconcile subsequent writes. Existing backups and security audit records follow their own retention policy; this is not a claim of forensic erasure. Content-bearing idempotency responses for deleted releases, team profiles and comments are removed. A minimal namespace tombstone prevents old links from identifying a different team later.

Package removal follows the committed metadata deletion. A package shared with another namespace is retained. Unreferenced packages enter a durable cleanup queue, removed under the same write lock as publication. Failed removals remain queued: resolve storage permissions and retry the same request with `--request-id`, or restart the Hub to retry. An incomplete package cleanup does not roll back an already committed team deletion or prevent the Hub from starting.

Use isolated Hubs for write-workflow acceptance. If an explicitly authorized test team exists on a serving Hub, archive, inspect and clean it up through these APIs. Do not silently delete its users or shared groups. Account retirement is a separate administrative decision. Older Hubs do not advertise `namespace.lifecycle.v1`; new clients hide the action or report that the Hub needs an upgrade.

## Version cleanup

My releases defaults to each skill’s latest submission by the current publisher, plus all pending submissions. Use **All versions** to find older attempts. A namespace maintainer or administrator can permanently delete a rejected or withdrawn version from its detail view. Published and pending versions must be withdrawn first.

```sh
skillhub review delete RELEASE_ID --source company --confirm research/example@1.0.0
```

Deletion removes the version’s files, comments, reactions, evaluations, scan reports and notifications. Audit events and a reserved coordinate remain; the same namespace/name/version cannot be reused. Installed copies remain on users’ devices. Shared package files stay until their final reference is removed. Storage failures can be retried with the same request ID. Clients expose deletion when the Hub advertises `publishing.cleanup.v1`.
