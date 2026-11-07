# Third-party components

SkillHub's Apache-2.0 license covers original project code and project-authored brand assets. It does not replace the licenses of dependencies or hosted skill packages.

The committed dependency owners are `go.mod` / `go.sum`, `desktop/go.mod` / `desktop/go.sum`, `desktop/frontend/package-lock.json` and `sdk/typescript/package-lock.json`. Generated Protovalidate TypeScript descriptors retain their [upstream notice](sdk/typescript/LICENSE.protovalidate).

Release packaging includes `LICENSE`, `NOTICE`, this file and dependency license texts under `third-party/`. Build tooling inventories the actual Go application imports and frontend package tree; unresolved license collection is a release error rather than a claim that every dependency is Apache-2.0. Dependency metadata is not a legal opinion or a substitute for reviewing new license obligations.

Local development skills, test databases and company deployment assets are excluded from release packages and are not part of this license grant.
