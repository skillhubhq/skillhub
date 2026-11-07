# Desktop updates

The App checks GitHub Releases on user request and opens the exact release page for a manual download. It does not download executables in the background, replace itself or install automatically. macOS packages use ad-hoc signing; Developer ID signing and notarization are not included.

## Check and install

Open Settings → Updates to see the installed version/build, select stable or preview, and check for updates. Version ordering is semantic rather than based on upload time. Missing platform assets, unavailable/private repositories, rate limits and network failures produce explicit feedback, not an up-to-date claim. A full 100-release response is reported as unverifiable rather than assumed complete.

Release notes are rendered as inert text. Download opens the official exact-version GitHub page. On macOS, quit the App, open the DMG, replace the application bundle using its Applications shortcut, eject the image and reopen. Verify the release checksum and follow the operating system's normal security policy. A DMG is a packaging format, not proof of publisher identity.

Sources, credential references, local inventory, drafts, preferences and installed skill files live outside the App bundle. Preserve these when replacing the App. Back up client state before a version with a state-format change. Downgrades require a compatible binary/state snapshot; never roll back skill directories as a side effect of replacing the application.

## State preservation

| State | Storage and replacement behavior |
| --- | --- |
| Connections, credential references, inventory and project shortcuts | Client data directory: `SKILLHUB_HOME` when explicitly configured, otherwise the OS user configuration directory's `skillhub` subdirectory |
| Saved skill drafts | `drafts/` beneath the client data directory; reopen with Publish → Open saved draft |
| Source credentials | OS credential store, referenced by the client database; not bundled into App files or a client-directory backup |
| Language, theme and selected community | The App's WebView profile; preserve the same OS account and application identity |
| Installed skills and project manifests/locks | Their existing global or project directories; replacing the App does not rewrite them |

Before replacement, save editor files and quit both App and CLI. Back up the entire closed client directory and any project/global files you need to recover; retain the prior binary and a device backup that includes the WebView profile. Do not copy only an active SQLite database file. A directory backup is not a portable credential backup: a new account or device may need to authenticate each connection again. Keep deployment credentials out of these artifacts.

After reopening, verify both Hub identities, language/theme, saved draft contents and local inventory. Reopen a project and compare its lock and installed files, including local edits. Check that no skill update was applied as a side effect. Unsaved editor text and unsubmitted publication form choices are not persistent drafts. Browser publication drafts remain page-local.

For rollback, first quit all clients. Use the prior binary directly only if it supports the existing state format; an unsupported newer schema must be rejected without mutation. Otherwise recover a matching pre-upgrade client snapshot and reconcile any installations or edits made since that snapshot before resuming work. A successful same-format rollback test does not promise arbitrary downgrades. Hub database recovery is a separate operator procedure.

## Update boundaries

| Component | Owner | Effect |
| --- | --- | --- |
| Hub and embedded Web | Deployment operator | Server binary, metadata migrations and package storage |
| Desktop App | Device user or IT | Application bundle and shared client state |
| CLI | Device user or package manager | CLI executable and shared client state |
| Installed skills | User through an install plan | Selected managed files from their originating Hub |

An App with several Hub connections has one software update channel. Hubs cannot choose an executable URL, signing key or updater. Unsupported remote capabilities affect only that connection. App updates do not update Hubs, skills or project locks.

## Network and trust

Update requests are bounded and timeout-limited. They contain no Hub credentials, URLs, searches, manifests or inventory, and no persistent device identifier. The App does not embed a private GitHub token. Private distributions require the user's normal GitHub access in the browser.

Checksums detect corruption but do not independently authenticate a compromised download source. Verify provenance and any available platform/attestation evidence. Automatic installation, signing-service integration and alternate update feeds are not supported.

Build packaging and release checks are described in [distribution](distribution.md); reproducible regression commands are in [testing](testing.md).
