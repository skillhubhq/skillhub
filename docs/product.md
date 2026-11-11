# Product scope

SkillHub is a self-hosted registry for sharing, reviewing and installing agent skills. It serves enterprise and public communities with independent identities and permissions. It manages skill files; it does not execute skills, invoke models or provide an agent runtime.

## Hub, Web, App and CLI

| Component | Responsibility |
| --- | --- |
| Hub | Identity, namespaces, access groups, taxonomy, immutable packages, reviews, community content and audit |
| Web | Browse and contribute to the hosting Hub; publish, review and administer within the user's permissions |
| App | Connect independent Hubs, browse their communities, author skills and manage installations on this device |
| CLI | Script the same remote and local workflows through the shared Go client core |

Each Hub owns its users and content. The default is private access with no public registration. Operators may enable anonymous browsing of published Public releases. Publishing and administration always require authentication. Local accounts are provisioned by authorized administrators; verified enterprise sign-in enters an approval queue by default. `auth.admission: automatic` admits new verified identities as ordinary members without granting enterprise publication or administration permissions. Community mode enables automatic third-party admission, personal publication spaces, mandatory review and quotas using the same user and permission model; see [community hosting](community-hosting.md). See [identity and access](identity-and-access.md) and [enterprise login](enterprise-login.md).

## Publication and governance

A skill archive contains `SKILL.md` and optional supporting files. A skill owns its namespace, stable name, presentation and access policy. All versions and community interactions share that policy. Published packages are immutable; changing their content requires a new version. Pending and rejected submissions can be revised under the same proposed version; each revision receives a new ID and independent review. A namespace can require independent review or permit direct publication. Security findings can require review regardless of that setting. Authors cannot approve their own submissions.

Namespaces organize ownership and publication. Access groups grant audience membership or namespace roles. Categories and tags support discovery and never confer access. Teams are namespace profiles, not a second permission system. See [governance](governance.md).

External evaluation reports bind an exact version and package digest. Maintainers can import sanitized results; readers follow the skill's current access policy. Reports are self-reported evidence, not executed or independently verified by the Hub, and do not grant approval or affect ranking. See [evaluation evidence](evaluations.md).

## Discovery and community

The catalog selects the highest readable stable semantic version for each skill, or the highest prerelease when no stable release exists. Profiles, teams and saved collections show one entry per skill. Users can search stable names, authored display names and descriptions, filter by category/tag, and browse by name, recent publication or administrator curation. Recent sorts skills by their latest publication event without changing the selected default version. Curated selection follows a stable namespace/name and never changes visibility. Likes, comments and bookmarks do not influence sorting; popularity ranking and personalized recommendations are not supported.

Profiles contain a display name, biography and optional uploaded avatar. Public profile discovery depends on readable contributions; private contributions remain filtered. Login usernames are not public display-name fallbacks. Avatars accept bounded PNG/JPEG files, normalize them to a 128-pixel PNG and strip original metadata. Arbitrary remote image fetching is not supported.

Likes, bookmarks and discussion belong to the skill and persist across releases. Comments identify the version being discussed. Replies have one level; removing a root preserves its replies. Changing skill access applies to all versions and discussion; making a skill public makes that history public. Unavailable bookmarks remain removable by their owner. Cleaning a withdrawn package preserves discussion, reactions and a minimal version record.

The compact Inbox icon shows the unread notification count. Review tasks have a separate live count. Its dialog separates live work awaiting the user's review from outcomes on their own submissions and replies to their comments. It shows pending count and oldest submission date, excludes self-submissions and links to exact versions. It refreshes on focus and periodically while visible. The two views keep work awaiting a decision separate from submission outcomes. Email, webhook and desktop push delivery are not supported.

## Skill presentation

The portable `SKILL.md` name is the stable identifier. A release may additionally have English and Simplified Chinese display names (up to 80 Unicode characters each) and a built-in icon. Display falls back from the selected language to the other authored language, then the stable name. Authored content is never machine-translated.

Curated releases separately identify the original author, the maintaining namespace and the authenticated operator. Their source commit, license and modification declaration are immutable, require review, and persist with installations. See [upstream attribution](governance.md#upstream-attribution).

Maintainers can edit skill presentation independently, with revision checks and an audit entry. Each package keeps its submission snapshot; local installations retain that snapshot for offline display. It does not modify the original archive. Publishing, cards, details, reviews and local inventory use the same display rules. Custom uploaded skill icons are not supported.

## Navigation and preferences

App navigation separates Community, My library and Studio. Studio and its publication, review and management entries follow the signed-in account’s effective Hub permissions. Native sessions renew automatically through the OS credential store, with revocation and bounded lifetimes; see [identity and access](identity-and-access.md#native-client-sessions). Expired sessions clear account-dependent navigation and private content together; returning to the window revalidates the session. The selected Hub connection determines community identity, publication, review and administration. Local installations and projects aggregate origins and have no global Hub selector. Each operation retains its specific source.

App discovery searches the current Hub by default; users can explicitly search all connections. Results remain separated by source, identity, permissions and errors. Web searches only its hosting Hub and groups the site name with its home logo. It has no multi-Hub selector.

Web and App share English-default and Simplified Chinese presentation, with system/light/dark themes. Language and appearance are device preferences exposed in the global header. Hub content and taxonomy titles are authored data. Operator branding and configuration are described in [configuration](configuration.md).

## Local installation and authoring

App and CLI share sources, credential references, installation inventory, project shortcuts and the Go client core. Each source has a local alias, canonical URL, immutable registry ID and independent credentials. Updating a skill uses its saved source rather than the currently selected community. Missing or ambiguous source identity requires explicit recovery.

The local library also discovers existing skills without assigning Hub provenance. Users can inspect and edit local files, associate an exact Hub version after comparing contents, hide entries, create copies, and restore backed-up changes. Detected external managers retain ownership; shared symbolic links show one content record with multiple tool locations. See [local workflows](usage.md#skills-already-on-your-device).

Optional configuration declarations provide per-installation requirements. App and CLI share device-local non-secret settings, system credential storage and environment references. Diagnostics never return values; version changes require explicit authorization for the current digest. Matching saved values are reused after confirmation; source, account or installation changes require independent configuration. Values never upload to a Hub. Users can explicitly load selected installations into Bash, Zsh or PowerShell through a local pipe, check the receiving process and remove added variables. SkillHub does not inject into existing processes or execute agents. See [local configuration](local-configuration.md).

Codex and Claude Code support global and project installation. Project tool-directory aliases must remain inside the selected project; aliases of the project root itself are supported. Installation checks authorization and digest, previews changes, locks target directories, stages files and uses atomic replacement with recovery journals. Unmanaged or locally changed files are never silently overwritten. Withdrawal prevents future downloads but does not remove previously installed files.

A project manifest describes desired exact versions and tools. The change preview resolves the desired lock and shows installations and removals; confirmation applies the lock and reports each target outcome. Its lock records source identity, URL, coordinate, digest and relative target without credentials. Restoration rechecks authorization and reports each target independently; updating an installation does not silently change the project lock.

The shared bounded cache requires online authorization before reuse. Clearing it preserves credentials and installations. Network/login failures remain failed checks; they do not claim a version is withdrawn or current. Unavailable exact versions offer retry and an explicit available-version action without silently changing the installation target.

The App supports local draft editing and package preview. Saved drafts can be reopened from the publication page after restart, including incomplete metadata that still needs repair. Saved files and publication form choices persist locally, scoped to the Hub and verified account. Web stores its archive draft in same-origin IndexedDB. Restoring a draft rechecks namespace, category and audience permissions. Web offers archive publication and tool-specific installation instructions/downloads; browser UI does not write directly into local tool directories.

## Deployment and distribution

The Hub runs as one Go process with embedded Web, SQLite or PostgreSQL, and local immutable package storage. No daemon, Redis, queue or search cluster is required. Multi-node serving, S3 storage, SCIM/directory synchronization, multi-stage approval and external scanners are not supported.

GitHub hosts software distributions. App updates are checked explicitly and installed manually; application updates do not update skills or Hubs. Hosted skills retain their own licensing and permissions independently of the Apache-2.0 software license. See [distribution](distribution.md), [operations](operations.md) and the [deployment checklist](deployment-checklist.md).

Team lifecycle is Hub-admin controlled: archive removes a team from default discovery and stops writes while preserving authorized history, downloads, data and grants; restore reactivates writes. Withdrawal controls package distribution. Permanent cleanup requires archive, revision and typed namespace confirmation, retains audit and shared identities, reserves deleted names, and queues reference-safe package reclamation. See [team lifecycle](governance.md#team-archive-and-permanent-cleanup).

## Version delivery

Subscribers follow a skill and receive publication notifications subject to current access. Version comparison reads two authorized immutable packages and returns bounded file differences.

A skill suite is an ordinary reviewed package with `skillhub.suite.json`: exact same-Hub members, verified digests and one entry skill. Clients expand installation plans and track shared member ownership. The registry verifies all members without executing them.

Automation tokens are owner-scoped, hashed, expiring and revocable. Read tokens access the catalog; publish tokens also submit to one namespace. They cannot approve, administer or mint credentials. Current account grants remain authoritative.
