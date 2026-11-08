# Product scope

SkillHub is a self-hosted registry for sharing, reviewing and installing agent skills. It serves enterprise and public communities with independent identities and permissions. It manages skill files; it does not execute skills, invoke models or provide an agent runtime.

## Hub, Web, App and CLI

| Component | Responsibility |
| --- | --- |
| Hub | Identity, namespaces, access groups, taxonomy, immutable packages, reviews, community content and audit |
| Web | Browse and contribute to the hosting Hub; publish, review and administer within the user's permissions |
| App | Connect independent Hubs, browse their communities, author skills and manage installations on this device |
| CLI | Script the same remote and local workflows through the shared Go client core |

Each Hub owns its users and content. The default is private access with no public registration. Operators may enable anonymous browsing of published Public releases. Publishing and administration always require authentication. Local accounts are provisioned by authorized administrators; verified enterprise sign-in enters an approval queue by default. Community mode enables automatic third-party admission, personal publication spaces, mandatory review and quotas using the same user and permission model; see [community hosting](community-hosting.md). See [identity and access](identity-and-access.md) and [enterprise login](enterprise-login.md).

## Publication and governance

A skill archive contains `SKILL.md` and optional supporting files. A release binds namespace, stable skill name, version, content digest and audience. Accepted versions are immutable; changes require a new version. A namespace can require independent review or permit direct publication. Security findings can require review regardless of that setting. Authors cannot approve their own submissions.

Namespaces organize ownership and publication. Access groups grant audience membership or namespace roles. Categories and tags support discovery and never confer access. Teams are namespace profiles, not a second permission system. See [governance](governance.md).

External evaluation reports bind an exact version and package digest. Maintainers can import sanitized results; readers follow that version's audience. Reports are self-reported evidence, not executed or independently verified by the Hub, and do not grant approval or affect ranking. See [evaluation evidence](evaluations.md).

## Discovery and community

The catalog selects the latest readable published release for each skill. Users can search stable names, authored display names and descriptions, filter by category/tag, and browse by name, recent publication or administrator curation. Recent means publication event order, not highest semantic version. Curated selection follows a stable namespace/name and never changes visibility. Likes, comments and bookmarks do not influence sorting; popularity ranking and personalized recommendations are not supported.

Profiles contain a display name, biography and optional uploaded avatar. Public profile discovery depends on readable contributions; private contributions remain filtered. Login usernames are not public display-name fallbacks. Avatars accept bounded PNG/JPEG files, normalize them to a 128-pixel PNG and strip original metadata. Arbitrary remote image fetching is not supported.

Comments, replies, likes and bookmarks belong to an exact published version. Replies have one level; removing a root comment preserves its replies. Likes are public counts and bookmarks are private to the user and Hub. Withdrawal or revoked access removes the release from community reads.

The compact Inbox icon shows a pending-review count. Its dialog separates live work awaiting the user's review from outcomes on their own submissions. It shows pending count and oldest submission date, excludes self-submissions and links to exact versions. It refreshes on focus and periodically while visible. The two views keep work awaiting a decision separate from submission outcomes. Email, webhook and desktop push delivery are not supported.

## Skill presentation

The portable `SKILL.md` name is the stable identifier. A release may additionally have English and Simplified Chinese display names (up to 80 Unicode characters each) and a built-in icon. Display falls back from the selected language to the other authored language, then the stable name. Authored content is never machine-translated.

Curated releases separately identify the original author, the maintaining namespace and the authenticated operator. Their source commit, license and modification declaration are immutable, require review, and persist with installations. See [upstream attribution](governance.md#upstream-attribution).

Presentation metadata is immutable, participates in review and is stored with local installations for offline display. It does not modify the original archive. Publishing, cards, details, reviews and local inventory use the same display rules. Custom uploaded skill icons are not supported.

## Navigation and preferences

App navigation separates Community, My library and Studio. The selected Hub connection determines community identity, publication, review and administration. Local installations and projects aggregate origins and have no global Hub selector. Each operation retains its specific source.

App discovery searches the current Hub by default; users can explicitly search all connections. Results remain separated by source, identity, permissions and errors. Web searches only its hosting Hub and groups the site name with its home logo. It has no multi-Hub selector.

Web and App share English-default and Simplified Chinese presentation, with system/light/dark themes. Language and appearance are device preferences exposed in the global header. Hub content and taxonomy titles are authored data. Operator branding and configuration are described in [configuration](configuration.md).

## Local installation and authoring

App and CLI share sources, credential references, installation inventory, project shortcuts and the Go client core. Each source has a local alias, canonical URL, immutable registry ID and independent credentials. Updating a skill uses its saved source rather than the currently selected community. Missing or ambiguous source identity requires explicit recovery.

Optional configuration declarations provide per-installation requirements. App and CLI share device-local non-secret settings, system credential storage and environment references. Diagnostics never return values; version changes require renewed configuration. Values never upload to a Hub. Users can explicitly load selected installations into Bash, Zsh or PowerShell through a local pipe, check the receiving process and remove added variables. SkillHub does not inject into existing processes or execute agents. See [local configuration](local-configuration.md).

Codex and Claude Code support global and project installation. Project tool-directory aliases must remain inside the selected project; aliases of the project root itself are supported. Installation checks authorization and digest, previews changes, locks target directories, stages files and uses atomic replacement with recovery journals. Unmanaged or locally changed files are never silently overwritten. Withdrawal prevents future downloads but does not remove previously installed files.

A project manifest describes desired exact versions and tools. Its lock records source identity, URL, coordinate, digest and relative target without credentials. Restoration rechecks authorization and reports each target independently; updating an installation does not silently change the project lock.

The shared bounded cache requires online authorization before reuse. Clearing it preserves credentials and installations. Network/login failures remain failed checks; they do not claim a version is withdrawn or current. Unavailable exact versions offer retry and an explicit available-version action without silently changing the installation target.

The App supports local draft editing and package preview. Saved drafts can be reopened from the publication page after restart, including incomplete metadata that still needs repair. Only saved skill files persist: unsaved editor text and unsubmitted publication form choices are not restored. Web offers archive publication and tool-specific installation instructions/downloads; browser UI does not write directly into local tool directories.

## Deployment and distribution

The Hub runs as one Go process with embedded Web, SQLite or PostgreSQL, and local immutable package storage. No daemon, Redis, queue or search cluster is required. Multi-node serving, S3 storage, SCIM/directory synchronization, multi-stage approval and external scanners are not supported.

GitHub hosts software distributions. App updates are checked explicitly and installed manually; application updates do not update skills or Hubs. Hosted skills retain their own licensing and permissions independently of the Apache-2.0 software license. See [distribution](distribution.md), [operations](operations.md) and the [deployment checklist](deployment-checklist.md).
