# Community hosting and login providers

SkillHub uses one Hub binary, user model and permission system. Deployment policy and login protocols are separate choices.

| Setting | Enterprise (default) | Community |
| --- | --- | --- |
| `mode` | `enterprise` | `community` |
| Default first verified external sign-in | Pending administrator approval | Ordinary account admitted automatically |
| Personal publication space | Administrators grant namespaces | One stable personal namespace on successful sign-in |
| Publication | Namespace review policy plus scanner findings | Independent review for every submission |
| Quotas | Existing request/package limits | Additional account, submission, storage and interaction limits |

`auth.admission` overrides the first-sign-in default with `automatic` or `approval`, independently of publication policy. Automatic admission in enterprise mode grants ordinary membership without a personal namespace. Existing pending, rejected and disabled accounts still require administrator action.

`private` independently controls anonymous access. Use `private: false` for a public catalog; Public releases are visible anonymously, while Members and Groups audiences remain protected. Authenticated actions retain their existing permission checks. Switching modes does not reactivate pending/disabled accounts, replace identities, remove roles or republish content. Existing members receive their personal namespace on their next successful login. Personal namespaces grant Maintainer only; they do not grant role administration or self-approval.

## Configure login adapters

`auth.providers` supports up to eight named entries. The initial adapters are `oidc` and `github`. Either adapter works in either mode, and multiple entries may coexist. Login and device-authorization pages render buttons from configuration; Web, CLI and App share the same Hub-issued identity and session flows.

```yaml
mode: community
private: false
auth:
  providers:
    - id: social
      type: github
      name: GitHub
      client_id: OAUTH_APPLICATION_CLIENT_ID
      client_secret_file: /run/secrets/skillhub-identity
```

Register a **GitHub OAuth App** with callback `https://skills.example.com/auth/providers/social/callback`. The callback uses the configured provider **id**, not its type or display name. Create separate registrations for different callback origins. Use a secret file or `client_secret_env: VARIABLE_NAME`, never both. Secrets are server-side only. No private repository or email scope is requested. See GitHub's [OAuth authorization flow](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps).

OIDC entries additionally set an HTTPS `issuer`; see [Entra setup](enterprise-login.md). Provider configuration changes require a restart. Unsupported adapters and duplicate IDs fail validation. `skillhub-server check-config --config PATH` resolves secret sources without contacting the identity service or opening the database.

Adapters verify identity only. The Hub owns account creation, admission, permissions, profile and session issuance. A verified authority and subject identify the account: OIDC uses `issuer`/`sub`; GitHub uses `https://github.com` and its numeric user ID. Login-name changes do not create a new account. Emails and matching nicknames never merge accounts. Account linking and directory group synchronization are not supported.

Initial display names are copied once. The GitHub adapter may fetch an avatar from its fixed avatar CDN without an OAuth token, with a timeout, byte cap and no redirects. The Hub normalizes PNG/JPEG content and fills only an untouched empty avatar. Avatar sync runs after sign-in with at most four concurrent users and a 15-second deadline, so a slow CDN does not delay login. Refresh the page to see a completed sync. Failure falls back to initials; the next login can retry. User-edited names and avatars are never overwritten. OIDC picture URLs are not fetched. The Hub does not retain identity-provider access/refresh tokens.

## Publication, moderation and capacity

See [community configuration](../deploy/community.example.yaml). Defaults are 10,000 total accounts for automatic admission, 10 submissions per user per rolling 24 hours, 200 submitted versions and 200 MiB of compressed packages per author, 5 GiB of package files across the Hub, 100 comments per user per rolling 24 hours and 10 reports per user per rolling 24 hours. Community RPC traffic is limited to 600 requests per client IP per minute (`limits.requests_per_minute`); configure trusted proxies correctly and size this limit for shared office networks. The existing 20-comment/minute and package-size limits also apply. Limits must be positive. Counts include rejected and withdrawn versions; withdrawal does not free storage quota. Duplicate package content counts per author/version for author quotas, while the global cap counts actual package files, including orphaned files.

Quotas and writes share database transactions. Retries with the same mutation key return the original result instead of consuming quota again. Existing identities can still log in after reaching the admission cap. Operators can provision recovery accounts through existing administrator tools. These limits bound normal application use; they do not replace reverse-proxy rate limits, disk monitoring, off-host backup or DDoS protection.

Signed-in readers can report a readable published version or comment. One report per user/target is retained. Hub administrators review reports in **Studio → Hub settings → Content reports**, record a decision, and dismiss it or remove the comment/withdraw the exact release. Reporting alone never removes content. Decisions are audited; reporters, report reasons and decision notes are visible only to Hub administrators. Authors see a neutral withdrawal reason, never the private moderation note. Removing a comment keeps reply tombstones; withdrawing a version does not delete installed copies. This uses existing Hub administration rather than a second moderator role system.

`site.privacy_url`, `site.terms_url`, `site.contact_url` and optional `site.filing_text`/`site.filing_url` appear in the Web footer. URLs must use HTTPS. They link to operator-maintained pages; configuring a URL does not create a policy page. Do not publish placeholder policies or registration details.

## Deployment verification

1. Back up metadata and packages together and restore into an isolated destination; see [database maintenance](database.md). Never point an older binary at an upgraded database.
2. Validate the chosen configuration and secret file permissions. Keep one independent local administrator. Confirm `mode`, `private` and assigned reviewers before opening traffic.
3. Register the identity application's actual callback origin, test sign-in/denial, verify profile initialization, then disable the Hub account and confirm Web/App/CLI tokens no longer work.
4. Submit as an ordinary member, verify pending review and self-approval rejection, approve with a different authorized account, install the exact version, report it and verify withdrawal blocks future access without altering installed files.
5. Test proxy client-IP handling and HTTPS, request rate limits, actual upstream identity/avatars connectivity, disk alerts, backup retention and off-host recovery. Measure expected catalog and download traffic on the target host. The repository's isolated tests do not certify server capacity or real identity credentials.
6. Publish actual operator contact, privacy/terms and any required domain registration information. Complete host/domain-specific requirements before allowing public traffic.

No extra runtime service, Agent execution or GitHub organization membership is required by the Hub. Review, database, package storage and local installations remain the same product capabilities in both modes.
