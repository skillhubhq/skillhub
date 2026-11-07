# Hub and CLI usage

[Documentation](README.md) / Hub and CLI

The current implementation supports governed publication, independent sources, safe installation/update/removal, and reproducible project restore. The desktop exposes the same shared-core workflows; see [desktop instructions](../desktop/README.md).

## Terminal basics

`skillhub --help` groups commands by discovery, local use, publishing and accounts. Add `--help` to any command for its flags and examples.

```sh
skillhub search --help
skillhub install --help
skillhub source list
skillhub list
```

Human output uses summaries, labeled fields and explicit empty states. Color is enabled only on a terminal; `--no-color`, a non-empty `NO_COLOR` environment variable, or `TERM=dumb` disables it. Redirected output contains no ANSI colors. For a stable machine-readable payload, always use `--json`; do not parse the human layout.

```sh
skillhub search review --source work --json > skills.json
skillhub list --json
NO_COLOR=1 skillhub source list
```

Errors go to stderr and retain a nonzero exit status. Partial searches include successful results and report the failed Hubs. Install, update, remove and project restore show a plan first: use `--dry-run` for a successful preview-only command, or `--yes` to apply. A preview without either flag returns nonzero to signal that no change was applied.

Shell completion is available through `skillhub completion bash|zsh|fish|powershell`. Use `skillhub completion SHELL --help` for installation instructions for your shell.

## Versioned preview releases

Download a versioned build from [GitHub Releases](https://github.com/skillhubhq/skillhub/releases). While the repository remains private, repository access is required. Choose the archive matching your OS and architecture and verify it against the release's `SHA256SUMS`. Hub archives include the Web UI. Desktop packages are unsigned; the release notes describe platform requirements and upgrade limits.

## Download private CI builds

Repository members can download binaries from a successful [CI run](https://github.com/skillhubhq/skillhub/actions/workflows/ci.yml). Select a run whose commit matches the code you intend to use and whose complete matrix passed. The artifact names are:

| Platform | Hub and CLI | Desktop |
| --- | --- | --- |
| Linux | `hub-cli-ubuntu-latest` | Not packaged |
| macOS | `hub-cli-macos-latest` | `desktop-macos-latest` |
| Windows | `hub-cli-windows-latest` | `desktop-windows-latest` |

With an authenticated GitHub CLI, replace `RUN_ID` with the chosen run number:

```sh
gh run download RUN_ID --repo skillhubhq/skillhub \
  --name hub-cli-macos-latest --dir ./skillhub-build
chmod u+x ./skillhub-build/skillhub ./skillhub-build/skillhub-server
./skillhub-build/skillhub --help
```

Linux uses the corresponding Linux artifact and the same executable-permission step. The `chmod` command ensures executable permissions even when a download or extraction tool does not preserve them. Windows binaries have `.exe` names and need no `chmod`. These artifacts target the native architecture of their CI runner; build from source below for another supported architecture.

The macOS desktop artifact contains `SkillHub-macos.zip`; unpack that inner archive to retain the `.app` bundle structure and executable mode. Windows contains `SkillHub.exe` and uses the system WebView2 runtime. Desktop development builds are unsigned; distribution signing is not configured. `native-runtime-*` artifacts are JSON test evidence, not applications to install. Test-only desktop binaries are excluded from the production artifact groups.

CI artifacts are retained by GitHub for a limited period; they are development delivery artifacts, not a permanent versioned release channel. The source checkout and build commands below remain the reproducible delivery path.

## Build and initialize

```sh
make web
go build -o bin/skillhub-server ./cmd/skillhub-server
go build -o bin/skillhub ./cmd/skillhub
bin/skillhub-server init --data .local/hub --admin admin --password-stdin
bin/skillhub-server serve --data .local/hub --public-url http://127.0.0.1:8080
```

Supply the initial password through stdin (12–72 bytes). `serve` defaults to a private registry. `--private=false` permits anonymous access to published public releases. External deployments require an HTTPS public origin; the HTTP listener can sit behind an existing TLS reverse proxy.

## Connect independent sources

```sh
skillhub source add company https://skills.example.com
skillhub source add public https://public-skills.example.com
skillhub source list
skillhub login company
skillhub whoami --source company
```

`login` prints the Hub's browser authorization URL and a short code, then polls until you approve or cancel. `login company --username admin --password-stdin` is an alternative for local accounts. Credentials are stored in the OS credential store and are never written to project files. CLI and App share the directory chosen by `SKILLHUB_HOME`, or `skillhub` below the OS user configuration directory.

Automation may supply `SKILLHUB_TOKEN_COMPANY` for the `company` source. Source aliases are uppercased and hyphens become underscores when constructing this environment variable name. Injected credentials are not saved. A public source with no saved or injected login does not require a running OS credential service. If a saved credential has disappeared from that service, the client requires login again instead of silently becoming anonymous.

`logout company` revokes the saved current session and removes its local credential. If remote revocation fails, the command reports that separately. `source remove company` removes the connection and local credential; use logout first when you also need remote session revocation. Logout does not erase cached or installed files. Cache reuse still requires current Hub authorization.

## Configure a namespace and independent reviewer

```sh
skillhub admin namespace-put platform --source company
skillhub admin user-add reviewer --source company --password-stdin
skillhub admin grant platform USER_ID --source company --role reviewer
skillhub admin users --source company
skillhub admin grants platform --source company
```

New namespaces require review by default. Only Hub administrators create namespaces; namespace administrators can change their policies and role assignments. Pass the current `--revision` for policy/category updates. The server rejects stale revisions.

Reader, maintainer and reviewer are separate roles. Namespace admins inherit all three capabilities. Hub admins can manage all namespaces, but no one can approve or reject their own submission. Grant multiple roles when one person needs both submission and review authority.

Groups can be audiences and role recipients:

```sh
skillhub admin group-add engineering --source company
skillhub admin group-member GROUP_ID USER_ID --source company
skillhub admin grant platform GROUP_ID --source company --kind group --role maintainer
skillhub admin category-put operations --source company --title Operations
```

Group membership changes take effect on subsequent requests. Category/tag changes do not grant access. Group and category directories are available to authenticated users; user-directory and whole-Hub audit views require Hub administration.

## Publish and review

In the App, publication namespaces are selected from the current identity's submission-authorized list, including group-derived maintainer/admin grants. Reader-only and reviewer-only namespaces are excluded. Categories are selected from the current Hub's non-archived directory, with an explicit uncategorized option. Both lists support loading subsequent pages; unavailable options prevent submission. Refreshing or changing source reloads choices. The Hub always rechecks permissions and category availability when a version is submitted. Tags remain free text.

```sh
skillhub publish ./release-check --source company --namespace platform \
  --version 1.0.0 --visibility groups --group GROUP_ID --category operations --tag release
skillhub review list --source company
skillhub review inspect RELEASE_ID --source company
skillhub review approve RELEASE_ID --source company --reason "Reviewed fixed package"
skillhub review reject RELEASE_ID --source company --reason "Missing verification steps"
skillhub yank RELEASE_ID --source company --reason "Withdrawn version"
```

The skill directory must contain a root `SKILL.md` with valid `name` and `description` frontmatter. ZIP archives are limited to 20 MiB, expanded files to 80 MiB total / 8 MiB per file, and the main skill Markdown to 256 KiB. Paths, symlinks, duplicate files, case collisions and metadata are checked without executing content.

Versions use full semantic versions, and coordinates remain reserved after rejection or withdrawal. Submit a new version when contents or audience change. A namespace with review disabled publishes immediately. In the catalog, “latest” means the most recently published version visible to the caller; installations still require an explicit version.

Review inspection returns the exact submission's Markdown, file names, sizes and execution flags. The API also provides the immutable ZIP for client-side content preview. Normal catalog/download APIs never serve pending, rejected or withdrawn releases, even to an administrator.

Use `--request-id YOUR_UNIQUE_KEY` to retry a remote mutation safely. The same key is scoped to the authenticated user and operation; changing the payload under that key is rejected, and a replay still requires current authorization.

## Search, install, update and remove

```sh
skillhub search release --source company
skillhub search release
skillhub info company/platform/release-check
skillhub versions company/platform/release-check
skillhub install company/platform/release-check@1.0.0 --tool codex --project ./project --dry-run
skillhub install company/platform/release-check@1.0.0 --tool codex --project ./project --yes
skillhub install company/platform/release-check@1.1.0 --tool codex --project ./project --yes
skillhub install company/platform/release-check@1.0.0 --tool claude-code --scope global --yes
skillhub list
skillhub updates
skillhub update /absolute/path/to/managed/skill --dry-run
skillhub update /absolute/path/to/managed/skill --yes
skillhub update /absolute/path/to/managed/skill --version 1.0.0 --yes
skillhub remove /absolute/path/to/managed/skill --yes
```

Search sends the query to the selected sources, or all configured sources when none are selected. Each source retains its own results/error/cursor. A partial failure returns the successful results and a nonzero exit code. Use a cursor only with its originating source and unchanged filters.

Installation shows a plan unless `--yes` applies it; `--dry-run` returns a successful plan-only result. Installing another version over the same managed target performs an update. Unmanaged content, local modifications and attempts to replace a target with the same name from another Hub are rejected. Operations do not run package scripts.

`updates [TARGET]` reports each managed installation independently. Default `update TARGET` chooses the highest visible stable semantic version greater than the installed version; it does not select prereleases or downgrade because an older branch was published recently. `--version` explicitly selects a version, including a downgrade or prerelease. Both choices still require a plan/apply decision and protect local edits. A local update does not rewrite project manifests or locks; use the project workflow below to change shared project requirements. If multiple configured identities match one Hub, use `install` with an explicit alias to choose the identity.

Default tool targets:

| Tool | Global | Project |
| --- | --- | --- |
| Codex | `~/.agents/skills/NAME` | `PROJECT/.agents/skills/NAME` |
| Claude Code | `~/.claude/skills/NAME` | `PROJECT/.claude/skills/NAME` |

Sources: [OpenAI local skill locations](https://learn.chatgpt.com/docs/build-skills#where-codex-loads-local-skills), [Claude Code skill locations](https://code.claude.com/docs/en/skills#choose-where-skills-load). Claude's reserved `synced` and `anthropic-skills` names are rejected by that adapter. Custom tool configuration roots are not yet auto-detected.

`--json` produces machine-readable output. Errors go to stderr; secrets never appear in default command output. No background client daemon is required.

## Shared download cache

```sh
skillhub cache info
skillhub cache clear --yes
```

CLI and App share a bounded cache under the client data directory. The App shows usage and a cleanup button on **我的源**. The cache keeps up to 128 coordinate entries / 256 MiB of compressed packages and evicts the least recently used entries when admitting another package. An entry is isolated by canonical Hub URL, registry ID, namespace, name and exact version. Two aliases for the same Hub/coordinate can reuse bytes; different Hubs cannot share a cache key.

A cache hit still contacts that Hub with the current source identity. `DownloadRelease.known_digest` allows a matching response to omit archive bytes only after normal authorization, publication-status and server-blob integrity checks. The client verifies the coordinates, digest and package structure before using cached bytes. Withdrawn versions, revoked access and offline sources fail instead of falling back to cached installation. Older Hubs that return the complete package remain compatible.

Cache files are plain private local files, not encrypted storage. They follow the client's data-directory access controls, just as installed files do. Corrupt entries trigger a full authorized download and replacement; unavailable cache storage does not prevent a valid full download. Cleanup removes managed cache files only, preserving installed skills, source identities, credentials and project metadata. Interrupted temporary writes are cleaned under a cross-process cache lock. This lock is separate from target installation locks, and network requests do not hold it. Pending in-memory install plans may retain already downloaded bytes after disk-cache cleanup.

## Reproducible projects

```sh
skillhub project init --project ./project
skillhub project add company/platform/release-check@1.0.0 --tool codex --project ./project
skillhub project add public/community/docs-helper@2.1.0 --tool claude-code --project ./project
skillhub project lock --project ./project
skillhub project show --project ./project
skillhub project restore --project ./project --dry-run
skillhub project restore --project ./project --yes
skillhub project list
```

Commit `skillhub.json` and `skillhub.lock.json` to your project's version control. Ignore `.skillhub-project.lock` (the cross-process operation lock) and `.skillhub-write-*` (temporary metadata files). Whether generated tool directories belong in version control is a project decision; they can be reproduced from the two metadata files.

The manifest records exact versions, source aliases, coordinates and tools. The lock additionally pins canonical Hub URL, immutable instance identity, archive SHA-256 and a relative target for every entry. Neither file contains credentials. Schema 1 supports up to 100 entries and 1 MiB per metadata file. Versions must be full semantic versions; ranges and implicit `latest` are intentionally excluded.

On a second machine, configure each source explicitly and log in where needed, then run `project restore`. The restore command never adds or contacts an unconfigured URL from a checked-out lock file. A different local alias is accepted when exactly one configured source has the same URL and registry identity. Resolution with `project lock` uses the manifest's aliases, so use matching aliases when you intend to regenerate the lock.

Restore validates the complete plan before making installation changes. It rechecks Hub identity, current download authorization, locked archive digest and local file state, including when applying a previously created plan. It refuses unmanaged directories, local edits, changed metadata and target paths redirected through symlinks. Locked versions cannot bypass withdrawal or revoked permission, even if previously downloaded. Existing installations continue to exist after remote permission changes; the client cannot retract already copied content.

Batch application commits one target at a time and returns a result per target. If a source becomes unavailable after planning, other successful targets remain installed. Fix the reported failure and restore again. There is no cross-target atomic rollback and no implicit deletion of installations absent from the manifest.

To change shared requirements, repeat `project add ...@NEW_VERSION`, run `project lock`, review the two files and restore. `project remove NAME --tool TOOL` removes only the desired entry; use `skillhub remove TARGET` to uninstall its managed files. Editing a manifest makes its existing lock stale until resolved again. Metadata writes use a temporary file and atomic replacement; a lock is accepted only when its recorded manifest digest matches.

`project show` registers an existing project in the local SQLite inventory shared by CLI and App. `project forget --project PATH` removes only that shortcut, including for a deleted directory. It preserves metadata and installed files. Client database migration v1 → v2 preserves prior source/installation records and adds project shortcuts; future schema versions are rejected.

## Hub backup and recovery

```sh
skillhub-server backup --data .local/hub --output .local/hub-backup
skillhub-server verify-backup --input .local/hub-backup
skillhub-server restore --input .local/hub-backup --data .local/hub-restored
```

Output/restore directories must be new and outside the source directory; their parents must exist. Backups include a consistent database snapshot and its immutable packages. Restore preserves Hub identity and governance, invalidates old sessions/device authorizations, and refuses existing destinations. Use the same binary version for backup as the running Hub. See the [operations guide](operations.md) for upgrade, cutover, retention and recovery limitations.

## Executable smoke test

```sh
python3 scripts/smoke.py --server bin/skillhub-server --cli bin/skillhub
```

This starts two temporary Hub processes, provisions isolated credentials, exercises the real CLI, verifies both tool directories and a second project restored from copied manifest/lock files, takes an online backup, verifies and restores a Hub to a new directory, checks partial-source failure and old-session refusal, reauthenticates and downloads an exact restored version, and removes its temporary state. It does not use your saved credentials or modify your global tool directories.

## Web UI and shared desktop review

Run `make build` to build the Web assets and embed them in the Go Hub binary. Node/npm are build-time dependencies only; running the Hub still needs one Go executable, SQLite and local package storage. For separate steps, run `make web` before `go build ./cmd/skillhub-server`. A source-only Go build without Web assets returns an explicit 503 at `/`; RPCs remain available.

Open the configured Hub `--public-url` in a browser. Public Hubs allow anonymous browsing of public releases; private Hubs require login. `/login` and `/logout` use browser-only HttpOnly sessions. Configure the exact external origin as `--public-url`; reverse proxies must preserve it. Browser cookies are independent from CLI/App sessions.

Web publication accepts a ZIP with `SKILL.md` at its root. Preview it, choose an authorized namespace and active category, then confirm the immutable version and audience. The shared Team Review page is available inside both the Web UI and desktop App. Desktop publication can inspect a local directory directly. Web installation offers tool-specific CLI instructions and an archive download; the desktop App performs filesystem installation.


### Community and language

Use **Teams** to browse namespace homepages, published skills and (when authorized) member roles. **My profile** on desktop or the account button on Web opens your profile; **Edit profile** updates your display name and introduction. Namespace administrators can also edit team information. Each desktop source keeps its own Hub identity.

Open a published skill to like it, save it privately, post a comment or reply. **Saved** lists the exact releases saved by the current identity in the selected Hub. Comments and reactions belong to the selected version, and remain subject to its current audience. Removing a comment keeps the reply thread readable. These actions persist in the Hub database and are visible across Web and App; they persist with the community.

Both applications start in English. Choose **简体中文** in the header to switch immediately and retain the choice on reload. User-authored content and administrator-defined category names keep their original language.


### Multiple accounts on one Hub: updates

When several connection aliases point to the same Hub, choose the identity explicitly for remote update checks and plans:

```sh
skillhub updates /path/to/project/.agents/skills/example --source company-author
skillhub update /path/to/project/.agents/skills/example --source company-author --dry-run
```

App normally reuses the saved installation connection. If it is unavailable or its identity cannot be resolved, the library offers connection recovery. An explicitly selected replacement must match the installation's Hub identity and URL; failed authorization never switches to another account.
