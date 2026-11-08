# Official community

[skills.codectn.com](https://skills.codectn.com) is the official public SkillHub community. A self-hosted Hub has independent accounts, content and permissions; connecting it never sends its credentials or private skills to the official service.

## Connect and browse

A new App profile with no saved connections attempts to connect to the official Hub once, anonymously. Discovery times out after three seconds and can be retried later. Existing profiles keep their connections. Removing the official connection is remembered; the App does not add it back. **Settings → Connections → Connect official Hub** lets you reconnect explicitly.

The **Official** badge identifies the configured official HTTPS origin, not a Hub-provided name or a skill author's endorsement. Third-party Hubs cannot obtain the badge by changing their display name. Treat every skill as code and instructions you need to review, including skills hosted on the official community.

## Open between Web and App

On a skill page, **Install skill → Desktop app → Open in SkillHub** opens the exact Hub, skill and version. The App displays the source and requests confirmation before connecting a new Hub. It does not receive browser cookies or GitHub tokens. You choose the tool, project/global destination and approve a file-change preview separately.

The `skillhub://open` link accepts only `hub`, `skill` and `version`. It cannot name a local destination, run an agent, grant access or install automatically. macOS registers the scheme through the installed application bundle. Windows registers it for the current user when the App is first launched; no administrator permission is required. If the browser cannot open the App, use the download link or copy the Hub address and coordinate. ZIP download and CLI instructions remain available.

App details also include **View on source Hub**, which opens the hosting website at the same skill version. Private Hubs may require a separate browser login. The link never carries credentials.

## Curated third-party skills

Official hosting is separate from original authorship. Before including a third-party skill, record its upstream repository, exact commit, path and license; preserve required license/notice files and original authorship. Import only the needed skill directory and referenced files, validate the archive and inspect static scan findings. Do not run bundled scripts during import.

Use a distinct curated namespace. Describe the content as an upstream copy, not an official original or an endorsement by the upstream author. Pin imported revisions, review updates before publishing a new immutable version, and keep a removal/contact path. Popularity alone does not establish safety or permission to redistribute. Source-available or unclear licenses require separate review.
