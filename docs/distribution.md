# Releases and client distribution

GitHub is the canonical home for source, issues and versioned downloads. Each Hub owns its community data and only links to the project; it does not mirror installers or send Hub credentials, searches or skill content to GitHub. The shared Downloads page and Web installation guide link to Releases without a referrer. App links open the system browser.

## Channels and platforms

Use [Releases](https://github.com/skillhubhq/skillhub/releases), including prereleases. `/releases/latest` is not the default entry while only previews exist. Private repositories require GitHub access; anonymous distribution starts only after repository visibility is changed by the owner.

The current release matrix is Linux amd64 Hub/CLI, macOS arm64 Hub/CLI/App and Windows amd64 Hub/CLI/App. Linux App and Intel macOS packages are not currently published. macOS requires 13+; Windows App requires WebView2. Desktop packages do not include paid platform signing or Apple notarization. Signing, notarization and automatic App updates require separate work and are not implied by GitHub hosting.

A release contains:

- `skillhub-VERSION-OS-ARCH.tar.gz` / `.zip`: CLI and Hub with embedded Web.
- `skillhub-app-VERSION-OS-ARCH.zip`: desktop App.
- `skillhub-app-VERSION-macos-arm64.dmg`: manual macOS installer image.
- `SHA256SUMS`: hashes for the application archives.
- `PROVENANCE.json`: exact source revision, CI run, artifact targets and hashes.
- License texts and dependency inventories inside the application archives.

Download the archive and SHA256SUMS from the same release. On Linux use `sha256sum --ignore-missing -c SHA256SUMS` in a directory containing only the intended release downloads; check that the intended file reports OK. On macOS use `shasum -a 256 ARCHIVE` and compare the full digest with its SHA256SUMS entry. On Windows use `Get-FileHash ARCHIVE -Algorithm SHA256`. Checksums provide integrity, not independent publisher authentication. If the release has a GitHub attestation, also use `gh attestation verify ARCHIVE --repo skillhubhq/skillhub`.

App settings provides manual update checking against the official GitHub Releases API, stable/preview filtering and an exact-version download-page link. It sends no Hub credentials and does not download or replace executables. Private-repository access, network failures and unavailable platform packages are explicit states, not an up-to-date result. The check is bounded to 100 recent releases; a full page is reported as unverifiable rather than claiming completeness. The [desktop updates](client-updates.md) describes the update and installation behavior.

## Manual macOS installation

A macOS DMG contains SkillHub.app, an Applications shortcut, license notices and bilingual installation instructions. Quit an existing App, drag the new copy to Applications, choose Replace, eject the image and reopen the installed App. Connections, credentials, drafts, preferences and installed skills live outside the bundle; keep those directories when replacing the application. Check database compatibility before downgrading.

macOS packages use ad-hoc signing only, not Developer ID or Apple notarization. A DMG does not suppress Gatekeeper or managed-device policies. Verify the source and checksum and follow macOS guidance for trusted downloads; do not disable system-wide security controls. A local successful launch does not prove a browser-downloaded package will be accepted on another Mac.

To build a local manual preview on macOS (no publication):

```sh
node desktop/build.mjs
python3 scripts/collect_licenses.py --desktop --output desktop/bin/third-party
python3 scripts/build_dmg.py --output desktop/bin/SkillHub-manual-preview.dmg
```

The output includes `.dmg.sha256` and `.dmg.json` with revision, build kind and signature limitations. Existing output is refused. Dirty local builds are labeled local, not released as a new version. No Hub or client data is copied into the disk image. Source hygiene and packaging reject SkillHub project references outside `skillhubhq/skillhub`, including executable strings and dependency inventories. Third-party dependency links remain unchanged. CI builds the image on macOS; release preparation verifies its version, source revision and checksum before including it alongside ZIP archives.

## Preparing a release

1. Commit the complete candidate, including generated bindings, and set `desktop/frontend/package.json` and its lockfile to the intended version. CI embeds that version in the macOS App bundle.
2. Pass the entire `CI` workflow for that exact main-branch push: protocol compatibility/drift, race tests, executable flows, native acceptance, vulnerabilities, source hygiene and license collection.
3. Run **Prepare release** manually with the version tag and successful CI run ID. It verifies the run owner/workflow/branch/revision and version, downloads only that run's artifacts and creates a new draft. Existing tags/releases are refused; binaries are not rebuilt during packaging.
4. Review the draft, hashes, provenance, native reports and platform notices. Software acceptance is based on repository-controlled tests and security/packaging evidence. Target-server, real identity-provider and managed-device integration checks are separate operator follow-up, not release blockers.
5. Enable GitHub immutable releases before public publishing if available, complete all assets on the draft, then publish it. The workflow does not publish the draft automatically. Never replace an existing release's binaries.

GitHub attestations are produced by the preparation workflow for public repositories; private repository availability depends on the GitHub plan, so private preparation does not claim an attestation. GitHub account/repository settings, signing credentials and visibility are not controlled by local source changes.

## Maintenance

The Web's download page is independent of community login. An installed App can manage multiple Hubs; updating the App is different from updating a skill. Existing installations continue to retain their origin. Back up the Hub before server upgrades and keep the exact previous binary and configuration; schema rollback uses a compatible backup.

If an organization needs offline distribution later, mirror the unmodified GitHub release archives, hashes and provenance through its deployment system. Do not make a private mirror a mandatory product dependency.

## Security evidence for release preparation

Draft preparation requires successful CI for the exact main-branch revision, complete CodeQL analyses for Go, JavaScript/TypeScript, Python and Actions at that same revision, and no open code-scanning alerts. Analysis errors, warnings, incomplete evidence or API failures stop preparation. A dismissed alert still requires an individual data-flow review; a successful scanner job alone is not a security clearance. The evidence lookup is bounded to the latest 100 analyses, so older candidates may require a fresh scan. Review newly disclosed dependency vulnerabilities before publication. Environment-integration status remains separate and is not a release gate.
