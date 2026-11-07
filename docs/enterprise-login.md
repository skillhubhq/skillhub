# Enterprise login with Microsoft Entra ID

The Hub owns account status, community profiles, access groups, namespace roles and audit. OIDC proves who is signing in. It does not grant access to Microsoft directory or business data, and it does not automatically grant Hub administration, publishing or review permissions.

See [identity and permissions](identity-and-access.md) for the complete role model and public/private boundaries. Public self-registration is not supported.

## Account management

- In Web or App, open **Studio → Hub settings → Users**. Authorized administrators can create local users and enable or disable accounts. The directory distinguishes Local account from Enterprise SSO and shows the community display name.
- **Access groups** manages groups and membership. **Role grants** assigns Reader, Maintainer, Reviewer or Admin roles to users or groups within a namespace. A team page is a namespace's community profile, not another identity system.
- Local users sign in with a Hub password. Enterprise users are admitted through OIDC according to deployment policy; they do not receive a default Hub password.
- An enterprise identity binds to `(issuer, sub)`. Email and display name are not authorization identifiers and never automatically merge local accounts. Changing an application registration can change `sub` and requires a separate identity migration plan.
- Disabling a Hub user revokes Web, App and CLI sessions and approved but unclaimed device authorizations. Contributions, comments and audit records remain. Signing in through SSO again cannot bypass disablement.

## Default admission and existing accounts

There is no public registration form or anonymous registration API. A Hub administrator or people administrator creates ordinary local accounts in **Studio → Hub settings → Users**. A namespace Maintainer cannot create Hub-wide users. The operator creates the initial administrator with `skillhub-server init`.

Local and enterprise sign-in both resolve to the same Hub user table. Contributions, comments, bookmarks and permissions belong to the Hub user ID. Enterprise sign-in also stores a verified `(issuer, sub)` binding to that ID. Repeated sign-in with the same enterprise identity reuses the same user; each Hub still owns its membership and permissions independently.

`oidc.allow_signup` defaults to `false`. A verified but unbound enterprise identity creates a **disabled pending account**. The browser shows a waiting-for-approval page, and no Web, App or CLI session is issued. Repeated sign-in reuses the request. A Hub or people administrator inspects the verified issuer/subject and approves or rejects admission. Approval grants ordinary membership only; groups and publishing/review roles require separate grants. The user signs in again after approval. A rejected request does not reopen on repeated sign-in; an administrator can explicitly enable the account later.

Setting `oidc.allow_signup` to `true` explicitly allows eligible first-time identities to become ordinary members immediately. Existing pending or disabled accounts are not activated by this setting. The queue supports at most 1,000 pending identities; restrict Entra application assignment first. No email-based account merging occurs. Issuer and subject are shown only to administrators authorized to manage people, never in community profiles or namespace user lookups.

## Configure Microsoft Entra ID

1. Register a single-tenant Web application in your own Entra tenant. Record the Directory (tenant) ID and Application (client) ID.
2. Register the exact redirect URI: `https://skills.example.com/auth/oidc/callback`. Local integration may use the Hub's localhost URL; production requires HTTPS. Do not configure a SPA or desktop client to hold the server-side secret.
3. Under Enterprise applications, enable **Assignment required** and assign only intended users or groups. Use the specific tenant issuer, not `/common`, `/organizations` or an email suffix as the tenant boundary. Application assignment also governs guest access. Complete administrator consent where required. Global administrators are not subject to this assignment restriction, so use an ordinary account when testing rejection of an unassigned user. See [Microsoft's application assignment guidance](https://learn.microsoft.com/en-us/entra/identity-platform/howto-restrict-your-app-to-a-set-of-users).
4. Create an application credential. Inject its secret **value** through your deployment's secret store as `SKILLHUB_OIDC_CLIENT_SECRET`. Do not put it in Git, frontend code, the App, CLI arguments or shared screenshots.
5. Start the Hub with tenant-specific configuration:

```sh
# Inject SKILLHUB_OIDC_CLIENT_SECRET through the deployment's secret store.
skillhub-server serve \
  --data /srv/skillhub \
  --listen 127.0.0.1:8080 \
  --public-url https://skills.example.com \
  --name 'Team Skills' \
  --private=true \
  --oidc-issuer https://login.microsoftonline.com/TENANT_ID/v2.0 \
  --oidc-client-id APPLICATION_CLIENT_ID \
  --oidc-name Microsoft \
  --oidc-allow-signup=false
```

`TENANT_ID` and `APPLICATION_CLIENT_ID` are placeholders. Keep approval mode enabled and configure Entra assignment. Retain an independent local administrator for first approvals and emergency recovery; an SSO user never becomes the bootstrap administrator automatically.

The Hub requests only `openid profile email`. It uses the stable identity and initial display name, does not read Microsoft Graph, directory groups or avatars, and does not retain the identity provider's access or refresh tokens. Sign-in uses authorization code with PKCE, one-time state, a browser-bound cookie and nonce, and verifies signature, issuer, audience and expiry. Each Hub currently supports one OIDC provider. Configuration changes require a restart.

## Web, App and CLI sign-in

- **Web:** choose **Continue with Microsoft**. First-time users wait for approval, then sign in again. Existing local accounts remain available through the expandable local sign-in section.
- **CLI:** use `skillhub login SOURCE`. **App:** choose browser sign-in in Connections. The Hub's `/authorize` page can authenticate through Microsoft before the user explicitly confirms the device code shown by the client.
- Enterprise credentials go only to Microsoft's sign-in page. App and CLI receive separate tokens issued by that Hub. Switching Hubs does not transfer permissions.

## Lifecycle and limitations

Enterprise Hub sessions last eight hours, including device-authorized tokens. Local account sessions last 30 days. Signing out revokes the current Hub session; it does not sign out of Microsoft. Removing an Entra assignment or disabling an Entra user does not immediately invalidate existing Hub sessions. An administrator must disable the Hub account for immediate revocation, or wait for session expiry and reauthentication.

SCIM provisioning/offboarding, Entra group mapping, invitation email, password reset email, automatic account merging, enterprise avatar synchronization and Entra global single sign-out are not implemented. Hub administrators can grant or remove Hub administrator status for an existing enterprise account; that user must sign in again after the change. Entra manages enterprise passwords and MFA. See [account recovery](operations.md#account-lifecycle-and-emergency-recovery) for local password changes, resets and offline recovery.

## Verify the integration

Automated tests use an isolated TLS OIDC provider with real RSA signatures and JWKS. They cover stable identity, issuer isolation, pending admission, approval/rejection, rejection of administrator elevation for pending accounts, disabled-account sign-in, browser/device session revocation and backup/restore. They reject invalid signatures, issuer, audience, nonce, expired tokens, incorrect cookies, expired state and callback replay. Tests do not skip TLS or JWT verification.

When connecting a real tenant, validate assigned and unassigned users, guest policy, MFA, callback domain and reverse proxy behavior. These are deployment-specific checks; repository tests do not claim a connection to a real enterprise tenant. Refer to Microsoft's [OIDC protocol](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc) and [ID token claims](https://learn.microsoft.com/en-us/entra/identity-platform/id-token-claims-reference).
