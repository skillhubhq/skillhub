# Testing

Use disposable databases, temporary client directories and synthetic packages. Never point integration tests at a shared Hub or production database. Store run reports in CI artifacts or ignored local storage; keep this guide focused on reproducible procedures.

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

Without the explicit DSN, PostgreSQL tests skip. CI covers PostgreSQL 15, 16 and 18 using matching native tools. Maintenance checks include lease exclusivity, migration cancellation, paired backup/restore, corrupted/missing packages, session invalidation, rollback into fresh destinations and incomplete-recovery guards.

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

## Security and deployment

CI scans source/history for secrets and checks dependencies with the pinned tooling. Static scans are bounded evidence; review archives and dependencies before distribution. Environment addresses, tenant identifiers, credentials and completed operational reports belong outside the source repository.

GitHub CodeQL default setup analyzes Go, JavaScript/TypeScript, Python and Actions with extended queries and both remote and local input sources. Review findings against the security boundaries in [SECURITY.md](../SECURITY.md); a passing analysis job only confirms that the scanner ran. Fix actionable findings, and record the data-flow and validation evidence when dismissing a false positive. Do not disable queries to hide findings.

Complete the [deployment checklist](deployment-checklist.md) against the actual identity provider, reverse proxy, database/storage and installation devices. Local fixtures cannot certify those integrations.
