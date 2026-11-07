# SkillHub engineering contract

- Commit subjects use Conventional Commits: `type(scope): description`, with an English imperative description and a meaningful product or component scope. Use `feat`, `fix`, `refactor`, `docs`, `test`, `build`, `ci` or `chore`; describe the resulting change rather than the work session.
- Read `docs/product.md` and `docs/architecture.md` before changing a boundary.
- The product is an independent skills registry, multi-source CLI, and desktop App sharing a Go client core. It does not execute skills or agents.
- `proto/` owns RPC contracts and shared stable enums. Use Buf and generated Connect clients. Do not add a second REST/JSON business API.
- Use `common.*` generated enums directly when semantics match. Keep generated API messages out of domain aggregates, commands, repositories, and persistent state.
- Handwritten Proto declarations and non-obvious implementation comments use Chinese. Identifiers, logs, errors, and test names use English. Never log credentials or package contents.
- Proto files use STANDARD, COMMENTS, and UNARY_RPC lint. Fields start at 1 and are contiguous for new messages. Once published, preserve wire identities; breaking changes require an explicit protocol-version decision.
- Update the canonical definition, regenerate, and commit generated output together. `make proto-check` must regenerate and check tracked and untracked generated output.
- Hub authorization is authoritative. Unknown enum values and missing credentials fail closed. Approval binds immutable content and audience.
- CLI and App call the same client core. The core must not depend on terminal/UI frameworks. Source credentials are isolated; local writes use cross-process locking and recovery journals.
- Never overwrite unmanaged or modified installed files silently. Never execute package scripts during inspection/install.
- Keep deployment single-node and lightweight. No daemon, Redis, queue, or search cluster in the deployment model.
- Use `make check` and focused/race/integration tests appropriate to the change. Record run-specific evidence in CI artifacts or ignored local storage; document reproducible verification in `docs/testing.md`.
- Public documentation describes current behavior, supported constraints and reproducible procedures. Do not commit development diaries, temporary plans, session transcripts, historical acceptance ledgers or superseded design alternatives.
- Keep all work within this repository. Do not copy any organization's internal assets, proprietary source, credentials, deployment configuration or live Hub data into it. Documentation uses reserved example domains and synthetic fixtures; operational evidence remains in ignored local storage.
