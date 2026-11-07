# Identity and permissions

SkillHub prioritizes internal skill sharing. Hubs are private by default, with no public registration and administrator approval for first-time enterprise sign-in. Anonymous browsing is an explicit deployment choice. Clients do not provide a central account service, merge users across Hubs or synchronize their permissions.

## Separate identity, profile and authorization

| Layer | Boundary |
| --- | --- |
| Sign-in identity | Local password or one OIDC provider. Enterprise identities bind to verified issuer/subject values, never an email match. |
| Hub membership | Each Hub owns user IDs, account status and sessions. Matching names across Hubs do not identify the same authorization principal. |
| Community profile | Display name, bio and avatar are visible through readable contributions or shared teams. Login usernames are returned only to their owner and Hub administrators. |
| Access groups | Grant content audience membership and namespace roles. Groups are not public teams and are not synchronized from an enterprise directory. |
| Teams | Presentation profiles for namespaces. Member lists are visible only to namespace members and Hub administrators. |
| Content audience | Each immutable release has a Public, Members or Groups audience, independent of the author's sign-in method. |

A profile is readable by its owner, a Hub administrator, or someone with a readable published contribution, an undeleted comment or shared namespace membership connecting them to that person. Being signed in does not permit browsing every profile by user ID. Unknown and inaccessible users both return not found. Shared profiles never fall back to another person's login username; public author links use opaque Hub-local user IDs.

Public and private access do not use separate account systems. There are no separately editable enterprise and public profiles. Publishing public content can make the author's community profile visible with that content. Internal deployments should stay private. Before enabling public access, review profile exposure, anonymous browsing and external publication policy.

## Effective permissions

The server enforces this matrix. Hidden menus are not an authorization boundary.

| Principal | Read | Publish / withdraw | Review | People and permissions |
| --- | --- | --- | --- | --- |
| Anonymous visitor | Published Public releases on public Hubs only | No | No | No |
| Ordinary member | Public, Members and otherwise authorized content | No by default | No by default | Edit own profile and local password; cannot create users |
| Namespace Reader | Public and Members; Groups still requires audience membership | No | No | Read namespace membership |
| Namespace Maintainer | Namespace content and submissions | Submit and withdraw in the namespace | Only with an additional Reviewer or Admin role | Cannot create Hub-wide users |
| Namespace Reviewer | Namespace content and submissions | Only with an additional Maintainer or Admin role | Review in the namespace; no self-approval | Cannot create Hub-wide users |
| Namespace Admin | Namespace content and submissions | Yes | Yes; no self-approval | Manage namespace roles, team profile and review policy; not Hub-wide users |
| People administrator | Same as an ordinary member; no additional content access | Requires a namespace role | Requires a namespace role | Create ordinary accounts, approve enterprise admission, enable/disable ordinary users; no password resets or role grants |
| Auditor | Same as an ordinary member; no additional content access | Requires a namespace role | Requires a namespace role | Read-only audit access |
| Hub administrator | All Hub content and submissions | Yes | Yes; no self-approval | Users, groups, Hub-wide grants, taxonomy and audit |

Grants are additive; there is no explicit deny rule. Reader grants do not bypass Groups audiences: a Reader must belong to one of the selected groups. Removing group membership removes that read access immediately. Maintainers, Reviewers and namespace Admins can read Groups content in their namespace to perform their duties. Hub administrators can read all content. Grant only the roles needed for the person's responsibilities; this is not storage designed to hide content from these trusted roles.

**Members** means all active signed-in members of the Hub, not members of one team. **Public** content still requires sign-in on a private Hub. Switching a Hub to public mode can expose existing Public releases to anonymous visitors, so inventory content and profiles first. The default publication audience is Members.

Catalog, detail, comments, downloads and cache reuse recheck authorization after permission revocation or withdrawal. Files already installed on a user's disk cannot be recalled remotely. Bookmarks belong only to their owner. Review binds content and audience; modifying content or widening its audience requires another publication workflow.

## Enterprise admission

Use the identity provider's application assignment to control eligibility, and retain a local emergency administrator. By default, a newly verified enterprise identity enters the approval queue. A Hub or people administrator verifies its stable identity and approves access. Operators may explicitly enable automatic admission. New enterprise members have no publishing, review or administration roles until granted separately.

Local accounts do not automatically become enterprise accounts. Invitation claiming, identity prebinding, SCIM, directory group mapping, immediate offboarding synchronization and account migration are not implemented. Enterprise Hub sessions last eight hours. Disabling a Hub user immediately revokes Hub sessions, but disabling that user only in the identity provider does not notify the Hub. Offboarding procedures must include Hub account disablement. See [enterprise login](enterprise-login.md).

## Verify authorization

Use non-administrator accounts to verify role combinations, group revocation, independent review and inaccessible-resource isolation. An administrator demonstration is not a permission test. See [testing](testing.md) and the [deployment checklist](deployment-checklist.md).
