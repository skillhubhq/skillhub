# Desktop App

Wails v3, React and TypeScript call the same Go client core as the CLI. Navigation separates Community, My library and Studio, with connections and preferences in Settings. It does not run an agent, execute package scripts or start a client daemon.

## Build and run

```sh
cd frontend
npm ci --ignore-scripts
cd ..
node build.mjs
```

The native build produces `bin/SkillHub.app` on macOS (13+) or `bin/SkillHub.exe` on Windows. Native platform prerequisites are required. The separate Go module keeps desktop dependencies out of Hub/CLI builds. The development build is unsigned; release signing requires the distribution owner's credentials.

Go-to-TypeScript bindings are generated and committed. `node build.mjs` refreshes them before compiling the frontend; CI verifies tracked and untracked binding drift. Package previews use plain text, including scripts and HTML, without running them. File reads recheck authorization and the original package digest.

## First use

1. Open **Settings → Connections**, add a Hub URL and alias, and sign in. Each connection keeps its own identity. Cache usage and cleanup are available in Settings.
2. In **Community → Explore**, search the current Hub or explicitly select all connections. Preview a version, choose a tool/destination and review the filesystem plan before installation.
3. In **My library → Installed**, check updates or plan removal. Updates reuse the recorded source; local edits prevent silent replacement/removal.
4. In **My library → Projects**, open/create a manifest, select desired versions, lock and restore. Updating an installation does not silently change a project lock.
5. In **Studio**, submit packages, read review feedback, handle review tasks or administer the selected Hub according to your permissions.

The App and CLI share the operating system's `skillhub` configuration directory, SQLite inventory and credential references by default. `SKILLHUB_HOME` selects an isolated client directory. Saved sessions use the OS credential store. Environment-injected source tokens are available for isolated acceptance and CI; they are not saved in project files. A locked/unavailable credential store is an error, not permission to send private requests anonymously.

Change previews are single use and expire after five minutes. A failed or expired plan must be recreated. Batch restore reports each target independently; successful targets are not rolled back when another target fails.

## Acceptance

Run `go test -race ./...` in this module for the real-Hub desktop service/CLI shared-state integration tests. On macOS, use deployment target 13 for the native linker:

```sh
CGO_CFLAGS=-mmacosx-version-min=13.0 CGO_LDFLAGS=-mmacosx-version-min=13.0 \
MACOSX_DEPLOYMENT_TARGET=13.0 go test -race ./...
```

See [testing guide](../docs/testing.md). The separate `node build.mjs --native-smoke` App exercises the actual WebView/Go bridge against the disposable fixture. CI runs it on macOS and Windows and preserves a JSON report. Human visual checks, native picker/keychain/browser interactions remain distinct acceptance evidence; successful builds alone do not prove runtime behavior.

CI additionally sets `SKILLHUB_TEST_OS_CREDENTIALS=1` for `TestDesktopUsesSystemCredentialStore` after building the real CLI at `../bin/skillhub[.exe]`. This opt-in test uses the runner's actual OS credential store, unique temporary credential references and two disposable Hubs. It checks App-to-CLI and CLI-to-App login sharing, logout deletion, remote session revocation and source isolation. Ordinary tests skip it so development runs do not trigger keychain prompts. Run it manually only under an isolated OS account with an accessible credential store; no credential values are printed.
