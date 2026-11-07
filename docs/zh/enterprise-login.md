# 企业登录

Hub 支持本地账号和一个 OIDC 身份提供方，优先满足企业内部使用。默认没有公开注册；首次企业验证进入准入审批，批准后再登录。企业身份绑定经过验证的 issuer/subject，不按邮箱自动合并本地账号。

## 账号与准入 [#accounts-and-admission]

本地账号由有权限的管理员主动创建，密码通过 stdin 或受控界面输入。企业登录证明身份；Hub 成员资格与发布、审核、管理权限分别授予。

默认 `oidc.allow_signup: false`：新身份验证后创建待审批申请，不发放可用会话。Hub 管理员或人员管理员在“创作 → Hub 设置 → 人员”审批；批准仅成为普通成员，不获得命名空间或平台角色。拒绝和禁用状态不会因再次登录自动解除。显式开启自动准入时，新身份可直接成为普通成员，仍不获得管理能力。

## 配置 Microsoft Entra ID [#configure-microsoft-entra-id]

1. 创建单租户应用，使用 Web 平台；注册精确 HTTPS 回调 `https://skills.example.com/auth/oidc/callback`。
2. 记录租户 ID、应用 Client ID，确认管理员同意所需权限。
3. 在企业应用中要求用户分配，只分配预期用户/分组，明确来宾和 MFA 策略。使用租户专属 issuer，不能用 `/common`、`/organizations` 或邮箱后缀代替租户边界。全局管理员不受相同分配约束，测试未分配拒绝时必须用普通账号。
4. 创建应用凭据，通过受控密钥存储注入 **secret value**，不要把密钥放入 Git、前端、App、命令参数或截图。
5. 保留独立本地管理员，用于首批审批和应急恢复。

```sh
skillhub-server serve \
  --data /srv/skillhub --listen 127.0.0.1:8080 \
  --public-url https://skills.example.com --name 'Team Skills' --private=true \
  --oidc-issuer https://login.microsoftonline.com/TENANT_ID/v2.0 \
  --oidc-client-id APPLICATION_CLIENT_ID --oidc-name Microsoft \
  --oidc-allow-signup=false
```

通过密钥服务注入 `SKILLHUB_OIDC_CLIENT_SECRET`，或使用[配置](configuration.md)中的密钥文件。示例 ID 是占位符。新 SSO 用户不会自动成为初始化管理员。

Hub 仅请求 `openid profile email`，使用稳定身份与初始展示名，不读取 Microsoft Graph、目录分组或头像，不保留身份提供方的访问/刷新令牌。登录使用授权码、PKCE、一次性 state、浏览器绑定 cookie 和 nonce，验证签名、issuer、audience 与有效期。修改配置需重启。

## Web、App 与 CLI [#web-app-and-cli-sign-in]

Web 选择“使用 Microsoft 继续”；首次等待审批，批准后重新登录。本地登录位于可展开区。CLI 使用 `skillhub login SOURCE`，App 在连接中选择浏览器登录；Hub 的 `/authorize` 支持先完成 Microsoft 验证，再明确确认客户端显示的设备码。

企业凭据只输入 Microsoft 页面。CLI/App 接收各 Hub 自行签发的独立令牌，切换 Hub 不转移身份或权限。

## 生命周期与限制 [#lifecycle-and-limitations]

企业 Hub 会话和设备令牌有效期八小时，本地账号会话 30 天。退出仅撤销当前 Hub 会话，不退出 Microsoft。Entra 撤销分配或禁用账号**不会即时通知 Hub**；需要立即撤销时，管理员同时禁用 Hub 账号，或等待会话过期重新认证。

尚未实现 SCIM、目录分组映射、邀请/重置邮件、自动账号合并、企业头像同步和 Entra 全局退出。Hub 管理员可给已有企业账号授予/移除 Hub 管理员身份，变更后该用户需重新登录。企业密码和 MFA 仍由 Entra 管理；参见[账号恢复](operations.md#account-lifecycle-and-emergency-recovery)。

## 验证接入 [#verify-the-integration]

本地测试使用隔离 TLS OIDC 提供方、真实 RSA 签名和 JWKS，检查稳定身份、issuer 隔离、准入审批、禁用账号、令牌撤销、备份恢复，以及错误签名/issuer/audience/nonce、过期、错误 cookie、state 过期和回调重放，不跳过 TLS/JWT 校验。

真实租户仍需验收用户分配、来宾、MFA、回调域名、代理和设备授权。这些是部署环境检查，代码测试不代表已连接真实租户。参考 [Microsoft 应用分配](https://learn.microsoft.com/en-us/entra/identity-platform/howto-restrict-your-app-to-a-set-of-users)、[OIDC 协议](https://learn.microsoft.com/en-us/entra/identity-platform/v2-protocols-oidc)和 [ID Token 字段](https://learn.microsoft.com/en-us/entra/identity-platform/id-token-claims-reference)。
