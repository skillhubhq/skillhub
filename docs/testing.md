# Testing

Use disposable databases, temporary client directories and synthetic packages. Never point integration tests at a shared Hub or production database. Store run reports in CI artifacts or ignored local storage; keep this guide focused on reproducible procedures.

## Software acceptance scope

Software release acceptance covers repository-controlled behavior: protocol compatibility, authorization and identity boundaries using local OIDC fixtures, publishing/review/install flows, native App state preservation, database initialization and recovery, security scans, and reproducible packaging. Run the supported-platform CI matrix against the exact candidate revision.

A real Entra tenant, an organization's server/proxy/storage and clean managed-device installation are follow-up integration checks owned by the deployment operator. They are not prerequisites or blockers for the software release. Keep their status separate: passing code tests does not claim that those environments have been validated. Use the [deployment checklist](deployment-checklist.md) when preparing a particular installation.

## Core and protocol

```sh
make check
make quality
make web
go build -o bin/skillhub-server ./cmd/skillhub-server
go build -o bin/skillhub ./cmd/skillhub
python3 scripts/smoke.py --server bin/skillhub-server --cli bin/skillhub
make sdk-smoke
```

`make check` runs Buf formatting/lint/build, Go vet and race tests, and TypeScript SDK serialization tests. `make quality` checks distributable source hygiene, documentation links and packaging tests. The executable smoke test exercises independent Hubs/identities, publication/review, both supported installation targets, project restore, cache, recovery and partial-source failure.

For protocol changes, run `make proto` and include generated output. `make proto-check` compares generation with the checked-out revision; run it on a checkout containing the intended generated changes. CI also checks protocol breaking changes. Do not hand-edit generated output.

Evaluation regression covers strict case/result parsing, archive binding, import/retraction, current-role checks, public/group audience isolation, both Connect codecs and paired backup preservation. See [evaluation evidence](evaluations.md) for portable fixtures and offline validation.

## Web and desktop

```sh
npm --prefix desktop/frontend test
npm --prefix desktop/frontend run build:web
node desktop/build.mjs --native-smoke
python3 scripts/desktop_fixture.py --automated \
  --app desktop/bin/SkillHub-smoke.app/Contents/MacOS/SkillHub \
  --report .local/native-runtime.json
```

On Windows use `desktop/bin/SkillHub-smoke.exe`. The fixture creates isolated Hubs, local state and packages, exercises the real WebView/Go bridge, verifies results through an independent CLI and removes its test data. Unlock the desktop for native interaction. A production build must not contain the smoke harness; rebuild with `node desktop/build.mjs` after smoke testing.

Run desktop Go tests from `desktop/`. On macOS:

```sh
CGO_CFLAGS=-mmacosx-version-min=13.0 \
CGO_LDFLAGS=-mmacosx-version-min=13.0 \
MACOSX_DEPLOYMENT_TARGET=13.0 go test -race ./...
```

The opt-in `SKILLHUB_TEST_OS_CREDENTIALS=1` test uses the real credential store, unique temporary references and disposable Hubs to verify App/CLI session sharing and isolation. Run it only under an isolated OS account with an accessible credential store and a built CLI at `../bin/skillhub` (or `.exe`).

For manual checks, build the production App and run `scripts/desktop_fixture.py --app PATH_TO_APP_EXECUTABLE`. Inspect light/dark themes, English/Chinese, narrow windows, keyboard focus, menus, dialogs, native pickers and error/empty states. Verify discovery → exact version → install/update/remove, publication → independent review → feedback, and account/permission management. Native credentials/pickers and clean-device installation require actual platform testing; a browser build is insufficient.

## Database and capacity

`SKILLHUB_TEST_POSTGRES_DSN` must select a disposable PostgreSQL service with permission to create/drop test databases. Use protected environment injection. Install matching-major `pg_dump`/`pg_restore` on PATH.

```sh
go test -race -count=1 ./internal/hubhttp
go test -count=1 -run TestPostgres ./internal/hub
go test -count=1 -run TestDiscoveryCapacity -v ./internal/hub
```

Without the explicit DSN, PostgreSQL tests skip. CI covers PostgreSQL 15, 16 and 18 using matching native tools. Maintenance checks include lease exclusivity, initialization cancellation, paired backup/restore, corrupted/missing packages, session invalidation, rollback into fresh destinations and incomplete-recovery guards.

Capacity tests use generated content to verify access-filtered pagination, latest-version selection, curation and review counts under concurrent reads. Record timings with the host/tool versions; do not use synthetic query latency as a deployment SLA. Measure realistic publication/download load and package sizes on the target environment.

## Compatibility

Protocol and client-state compatibility tests verify fail-closed handling of unsupported schemas, capabilities and unknown enums. To compare two explicitly selected local binaries:

```sh
python3 scripts/compatibility.py \
  --server /path/to/candidate-server --cli /path/to/candidate-cli \
  --baseline-server /path/to/comparison-server \
  --baseline-cli /path/to/comparison-cli
```

The runner uses fresh fixtures and covers both client/server directions, alternating CLI use of shared state, and a baseline Hub snapshot followed by replacement with the candidate binary and rollback into a new directory using the baseline binary. It verifies source identity, review permissions, package downloads, installed files and token invalidation after recovery. CI runs this matrix on Linux against the pull-request base or previous main revision and retains both revisions in the compatibility artifact. PostgreSQL maintenance is exercised separately across the database matrix.

Before a release, also run against the last supported **released** binaries; the immediately preceding commit is not a substitute for that acceptance. No historical release baseline exists before the first release. Record exact revisions with the resulting artifact. Passing one pair does not establish a universal version-support guarantee.

### Native App replacement

Build the candidate native-smoke App and Hub/CLI binaries as above, then select an explicit comparison commit using its full 40-character lowercase SHA:

```sh
python3 scripts/desktop_upgrade.py \
  --baseline-ref COMPARISON_COMMIT \
  --candidate-app desktop/bin/SkillHub-smoke.app \
  --server bin/skillhub-server --cli bin/skillhub \
  --report .local/native-upgrade.json
```

On Windows use `.exe` paths. Run with an unlocked desktop and accessible OS credential store. The helper builds a detached comparison checkout, overlays only the native upgrade test instrumentation and records its hashes/revision. Its production core/UI remain from that commit. Use `--baseline-app PATH` instead when supplying an already instrumented baseline; record its source revision separately. Unsupported comparison instrumentation fails rather than falling back to the candidate.

The runner seeds two independent Hub accounts through the real OS credential store, two saved drafts (valid and incomplete), four installations (Codex/Claude Code × global/project), a project manifest/lock and a local file modification. It removes login passwords before subsequent phases; no environment-token injection substitutes for saved credentials. It restarts the baseline, replaces the same App path with a distinct candidate, restarts it, then restores the compatible baseline binary. macOS replacement uses a verified, read-only mounted DMG. Each phase checks records, credential/file digests and real authenticated App/CLI requests; the candidate also reopens drafts through the publication UI.

Fixture Hubs, client state, installation directories and credential references are isolated and cleaned up. The smoke bundle uses a separate application identity; production App state is untouched. These tests do not simulate browser quarantine, signing identity changes, different device/user migration or a future incompatible schema. CI runs the same replacement regression on macOS and Windows; clean-device distribution checks remain separate.

## Security and deployment

CI scans source/history for secrets and checks dependencies with the pinned tooling. Static scans are bounded evidence; review archives and dependencies before distribution. Environment addresses, tenant identifiers, credentials and completed operational reports belong outside the source repository.

GitHub CodeQL default setup analyzes Go, JavaScript/TypeScript, Python and Actions with extended queries and both remote and local input sources. Review findings against the security boundaries in [SECURITY.md](../SECURITY.md); a passing analysis job only confirms that the scanner ran. Fix actionable findings, and record the data-flow and validation evidence when dismissing a false positive. Do not disable queries to hide findings.

The [deployment checklist](deployment-checklist.md) records optional-to-release, environment-specific follow-up against the actual identity provider, reverse proxy, database/storage and installation devices. Local fixtures cannot certify those integrations, and pending integration work does not fail code-level acceptance.

Native overwrite tests use one isolated application and credential-store identity. When comparing a checkout with a different distributor identity, the harness normalizes only those identifiers and records the affected files in `fixture_identity_normalization`. Such a run verifies data-format compatibility; it does not certify automatic migration between application identities. Distributors must keep their application and credential-store identifiers stable across releases.

## Login adapters and community policy

Run `go test -race ./internal/identityprovider ./internal/hubhttp ./internal/hub ./internal/serverconfig` to verify provider validation, OIDC signatures/claims, OAuth PKCE, state/cookie/replay isolation, stable identities, mode-specific admission, disabled/pending account retention, safe avatar initialization, quotas, moderation and schema initialization and provenance upgrade preservation. Adapter tests use controlled HTTP transports; OIDC tests use a local TLS provider with real signed tokens. They do not require or claim real provider credentials.

`TestPostgresCommunityPolicyAndModeration` runs through the existing isolated PostgreSQL fixture on versions 15, 16 and 18. The native fixture additionally exercises the report dialog, shared Go methods, report queue authorization, resolution and comment tombstones. All fixture data is disposable; these tests never open the operator's current Hub data.
