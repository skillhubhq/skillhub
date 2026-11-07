# 企业身份与 Microsoft Entra ID 接入

Hub 自己管理用户状态、社区资料、权限分组、命名空间角色和审计。OIDC 只负责证明登录者是谁；不会自动取得 Microsoft 目录或业务数据，也不会自动获得 Hub 管理员、发布者或审核者权限。

完整角色与公开/私有边界见 [企业身份与权限边界](identity-and-access.md)。当前优先企业内部使用，不提供公开自主注册。

## 账号管理

- Web / App：Studio（创作）→ Hub settings（Hub 设置）→ Users（用户）。管理员可以创建本地用户、启用/禁用账号；用户列表区分 Local account / Enterprise SSO，并展示社区显示名。
- Access groups 管理权限分组及成员；Role grants 为用户或分组分配命名空间 Reader、Maintainer、Reviewer、Admin。团队主页是命名空间的社区资料，不是另一套身份系统。
- 本地用户使用 Hub 密码。企业用户首次经 OIDC 登录时按部署开关创建普通账号，无需也不能设置一个默认 Hub 密码。
- 账号唯一绑定 `(issuer, sub)`。邮箱和显示名称都不是权限依据；不按同名或同邮箱合并本地账号。更换应用注册导致 sub 改变时需要另行规划身份迁移。
- 禁用 Hub 用户会撤销其 Web / App / CLI 会话及已批准但未领取的设备授权，保留作品、评论和审计。重新 SSO 登录不会绕过禁用状态。

## 默认建号策略与已有账号

默认没有公开注册入口或匿名注册 API。本地普通用户由 Hub 管理员或人员管理员在“创作 → Hub 设置 → 用户”创建；命名空间的 Maintainer（维护者）角色不能创建全站用户。初始管理员由部署者通过 `skillhub-server init` 建立。

本地密码登录与企业登录最终都对应同一个 Hub 用户表，作品、评论、收藏和权限归属于 Hub 用户 ID。企业登录另存已验证的 `(issuer, sub)` 到 Hub 用户 ID 的绑定。同一个企业身份再次登录沿用该用户；不同 Hub 仍分别管理成员和权限。

`oidc.allow_signup` 默认 `false`。已验证但尚未绑定的企业身份会建立**禁用的待审批账号**，浏览器显示“等待审批”，不发放 Web/App/CLI 会话。重复登录沿用同一申请；Hub 管理员或人员管理员在“创作 → Hub 设置 → 用户”核对已验证的 issuer/subject 后批准或拒绝。批准仅成为普通成员，分组和发布/审核角色需要另外分配；批准后用户重新登录。拒绝后重复登录不会重新开启申请，管理员可明确重新启用该账号。

显式设为 `true` 才允许符合 IdP 应用准入的首次身份直接成为普通成员；已有待审批/禁用账号不会因为开启开关而自动激活。申请队列最多 1,000 个待审批身份；先在 Entra 限制应用分配。没有公开注册或邮箱自动合并；同名本地用户不会成为微软用户的账号。issuer/subject 只向有人员管理权限的管理员展示，不进入社区资料或命名空间用户查询。

## Microsoft Entra ID 配置示例

1. 在企业自己的 Entra 租户注册单租户 Web 应用，记录 Directory (tenant) ID 与 Application (client) ID。
2. 将 Web redirect URI 精确注册为 `https://skills.example.com/auth/oidc/callback`。本地联调可使用 Hub 的 localhost 地址，生产必须 HTTPS。不要把 SPA 或桌面客户端直接注册成持有服务端密钥的应用。
3. 在 Enterprise applications 中开启“Assignment required”，仅分配需要使用内部 Hub 的用户/组。Hub 使用具体租户的 issuer；不要使用 `/common`、`/organizations` 或邮箱后缀来替代租户边界。租户中的来宾能否访问也由应用分配控制。按微软要求为需要分配的应用完成管理员同意；全局管理员不受此分配限制，因此验收未分配访问时使用普通账号。见[微软应用分配说明](https://learn.microsoft.com/en-us/entra/identity-platform/howto-restrict-your-app-to-a-set-of-users)。
4. 创建应用凭据，将 secret **值**通过部署系统的 secret 注入 `SKILLHUB_OIDC_CLIENT_SECRET`。不要放到 Git、前端、App、CLI 参数或共享截图里。
5. 启动 Hub，例如：

```sh
skillhub-server serve \
  --data /srv/skillhub \
  --listen 127.0.0.1:8080 \
  --public-url https://skills.example.com \
  --name 'Company Skills' \
  --private=true \
  --oidc-issuer https://login.microsoftonline.com/TENANT_ID/v2.0 \
  --oidc-client-id APPLICATION_CLIENT_ID \
  --oidc-name Microsoft \
  --oidc-allow-signup=false
```

`TENANT_ID` 和 `APPLICATION_CLIENT_ID` 是占位符；密钥由部署系统提供。默认保持审批模式，并在 Entra 配置应用分配。保留独立本地管理员处理首次审批与应急恢复；SSO 用户不会成为首个管理员。

仅请求 `openid profile email`。当前只使用稳定身份与首次登录的显示名，不读取 Microsoft Graph、目录群组或头像，不保存 IdP 的 access/refresh token。登录采用授权码 + PKCE、一次性 state、浏览器绑定 Cookie、nonce，并验证签名、issuer、audience、有效期。当前每个 Hub 配置一个 OIDC 提供者，配置变更需要重启。

## 三端的登录体验

- Web：登录页选择“Continue with Microsoft”，完成企业登录；首次等待管理员批准，批准后再次登录返回社区。原有本地账号入口仍在可展开区域。
- CLI：使用现有 `skillhub login SOURCE` 浏览器授权；App：Connections → 浏览器登录。访问 Hub `/authorize` 时可以先用 Microsoft 登录，再明确输入客户端显示的设备码进行确认。
- 企业凭据只进入微软登录页面；CLI/App 领取的是该 Hub 自己发放的独立令牌。换 Hub 不共享权限。

## 生命周期与当前边界

企业账号的 Hub 会话有效期为 8 小时，包括设备授权发放的令牌；本地账号仍沿用 30 天。退出只撤销当前 Hub 会话，不等于退出微软账号。用户在 Entra 被停用或移出应用分配后，已有 Hub 会话不会实时收到通知：需要管理员在 Hub 禁用账号以立即撤销，或等待会话到期并重新登录。

不支持 SCIM 自动预配/离职同步、Entra 群组自动映射、邀请邮件、密码重置邮件、自动账号合并、企业头像同步或 Entra 全局单点退出。Hub 管理员可在人员管理中为已有企业账号授予或移除 Hub 管理员权限，变更后该账号须重新登录。企业自助密码/多因素认证由 Entra 提供；本地账号的改密、重置与离线恢复见 [运维文档](operations.md#account-lifecycle-and-emergency-recovery)。

## 验证登录接入

代码测试使用独立 TLS OIDC 提供者、真实 RSA 签名及 JWKS：检查首次建号、再次登录稳定性、跨 issuer 隔离、默认待审批、批准/拒绝、禁止待审批提升管理员、禁用后拒绝登录、浏览器/设备会话撤销、备份恢复；拒绝错误签名、issuer、audience、nonce、过期令牌、错误 Cookie、过期 state 和回调重放。没有跳过 TLS 或 JWT 验证。

真正上线前仍需在企业 Entra 应用中验证允许/未分配用户、来宾策略、MFA、回调域名和反向代理。此仓库测试不代表已经连接到真实企业租户。微软注册与授权配置参考 [OIDC 官方说明](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc) 与 [ID token claims](https://learn.microsoft.com/en-us/entra/identity-platform/id-token-claims-reference)。
