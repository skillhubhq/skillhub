# Contributing to SkillHub

Thanks for helping make skills easier to share and use. Start with a focused issue describing the user problem; small bug fixes can go directly to a pull request. Please keep discussions respectful and specific.

## Local setup

| Tool | Version |
| --- | --- |
| Go | See [`go.mod`](go.mod) and [`desktop/go.mod`](desktop/go.mod) |
| Node.js | 24 |
| Buf | Pinned in [CI](.github/workflows/ci.yml) |

```sh
make web         # Build the shared Web interface
make check       # Protocol, Go race tests and SDK tests
make quality     # Documentation, source hygiene and packaging tests
make desktop     # Build the App on a supported host
```

Run a disposable Hub using the [README instructions](README.md#self-host). Never develop against a live company registry. The [documentation index](docs/README.md) links to the product, architecture and operational references.

## Before opening a pull request

- Run `make check`, `make web` and `make quality`. Run desktop Go tests and a native build when changing native/shared client behavior.
- Run `make proto-check` after regenerating protocol changes. It compares the working tree with the checked-out revision; intended generated changes must be included in the PR and pass on its committed checkout.
- Keep Go formatted and format changed frontend files with the pinned Prettier. Keep human CLI output readable and preserve the `--json` contract. Preserve English-default and Simplified Chinese UI behavior, keyboard interaction and both themes.
- Include meaningful regression coverage for authorization, publication, filesystem changes or protocol changes. Describe the behavior, test evidence and remaining platform limitations in the PR.
- Avoid credentials, customer data, screenshots of private communities, machine paths, internal deployment configuration and local skills in contributions.

## Engineering boundaries

Read `AGENTS.md`, `docs/product.md` and `docs/architecture.md`. Proto owns RPC contracts. Clients share the Go core; Hub authorization is authoritative. Do not execute packages or silently overwrite user files. Keep the single-node deployment lightweight. No sibling repository is needed to build or run this project.

Handwritten identifiers and errors use English. Non-obvious Go/Proto comments follow the repository's Chinese-comment convention. Generated code must be regenerated, not edited manually.

## Commit messages

Use `type(scope): description` for commit subjects and squash-merge titles. Write a short English imperative description of the resulting behavior. Choose a scope such as `hub`, `cli`, `desktop`, `web`, `auth`, `storage` or `docs`.

```text
feat(cli): add grouped command help
fix(auth): reject expired login attempts
docs(readme): clarify self-hosting setup
```

Use `feat` for capabilities, `fix` for defects, and `refactor`, `docs`, `test`, `build`, `ci` or `chore` for their respective changes. Explain motivation and relevant compatibility details in the body when needed; keep progress notes and development-session summaries out of the subject.

## Documentation

Keep documentation aligned with current behavior and supported constraints. Put repeatable commands in [testing](docs/testing.md); attach execution evidence to the pull request or CI artifacts. Keep development diaries, temporary plans, private deployment reports and superseded proposals outside distributable source.

## License and security

Contributions are accepted under Apache-2.0. Include the provenance and license of any third-party material you add. Do not import proprietary code without authorization. Report vulnerabilities privately using `SECURITY.md`; public issues are for non-sensitive defects.
