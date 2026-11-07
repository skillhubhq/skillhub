# 社区部署与登录适配器

SkillHub 使用同一个 Hub 程序、用户模型和权限体系。部署模式与登录协议相互独立。

| 配置 | 企业模式（默认） | 社区模式 |
| --- | --- | --- |
| `mode` | `enterprise` | `community` |
| 第一次第三方身份验证 | 等待管理员审批 | 自动成为普通成员 |
| 个人投稿空间 | 管理员分配命名空间 | 成功登录时分配稳定的个人命名空间 |
| 发布 | 命名空间审核策略与安全扫描 | 每次提交都需独立审核 |
| 配额 | 既有请求与包大小限制 | 额外限制账号、投稿、存储和互动 |

`private` 独立控制匿名访问。公开目录使用 `private: false`；匿名用户仅能看到 Public 内容，Members 和 Groups 受众仍受保护。切换模式不会启用已有待审批/禁用账号、替换身份、清除角色或重新发布内容。已有成员下次成功登录时获得个人空间。个人空间仅授予 Maintainer，不授予角色管理或自我审核权限。

## 配置登录适配器 [#configure-login-adapters]

`auth.providers` 最多配置八个登录入口，当前内置 `oidc` 与 `github` 适配器，两者均可用于任一模式，也可同时配置。登录页和设备授权页根据配置生成按钮，Web、CLI 和 App 使用相同的 Hub 身份与会话流程。

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

创建 **GitHub OAuth App**，登记回调 `https://skills.example.com/auth/providers/social/callback`。路径取自提供方 **id**，不取自类型或显示名。不同来源域名使用各自的应用注册。密钥通过文件或 `client_secret_env: VARIABLE_NAME` 注入，两者互斥，仅由服务端使用；不申请私有仓库或邮箱权限。参考 [GitHub OAuth 授权流程](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps)。

OIDC 还需配置 HTTPS `issuer`，参见 [Entra 接入](enterprise-login.md)。配置变更需重启。未知适配器、重复 ID 会拒绝启动。`skillhub-server check-config --config PATH` 解析密钥源，但不连接身份服务或数据库。

适配器仅验证身份，Hub 负责建号、准入、权限、资料和会话。账号依据经过验证的身份域与 subject 识别：OIDC 为 issuer/sub，GitHub 为 `https://github.com` 与数值用户 ID。改名不会新建账号，相同邮箱或昵称不会合并账号。暂不支持账号关联、目录分组同步。

初始展示名仅复制一次。GitHub 适配器可从固定头像 CDN 读取图片，不携带 OAuth token，并限制超时、体积和跳转；Hub 将 PNG/JPEG 标准化，仅填充未编辑过的空头像。头像在登录后后台同步，最多同时处理四位用户、每次限时 15 秒，不会因为 CDN 缓慢拖延登录；完成后刷新页面即可看到。失败显示首字母，下次登录可重试。用户编辑过的名称和头像不会被覆盖。OIDC 的任意 picture URL 不会被抓取。Hub 不保留第三方访问/刷新令牌。

## 发布、治理与容量 [#publication-moderation-and-capacity]

参见[社区配置](../../deploy/community.example.yaml)。默认自动准入以总账号数 10,000 为上限；每位用户滚动 24 小时最多提交 10 次，累计最多 200 个版本、200 MiB 压缩包；Hub 包文件总计最多 5 GiB；每位用户滚动 24 小时最多 100 条评论、10 条举报。社区 RPC 按客户端 IP 默认每分钟限 600 次（`limits.requests_per_minute`）；需正确配置可信代理，并按共享办公网络人数调整。原有每分钟 20 条评论和包体积限制继续有效。配额必须为正数。被拒绝、撤下版本仍占用作者配额，撤下不会释放存储。相同包按作者/版本累计作者配额，全局配额按实际文件计算，也计入孤立文件。

配额与写入在同一事务内检查，相同幂等键重试不会重复扣额。准入上限不影响已有成员登录，管理员仍可通过已有工具创建恢复账号。这些约束限制正常应用使用，不能代替代理限流、磁盘告警、异机备份或 DDoS 防护。

登录用户可举报自己能读取的已发布版本或评论，每个用户/目标保留一条举报。Hub 管理员在“创作 → Hub 设置 → 内容举报”中填写处理说明，驳回举报、移除评论或撤下精确版本。举报不会自动删内容。处理留有审计，举报人、举报原因及内部处理说明仅 Hub 管理员可见。发布者只能看到通用撤下原因，不会看到内部处理说明。评论移除保留回复占位；版本撤下不会删除已安装副本。复用已有 Hub 管理职责，不新增独立版主体系。

Web 页脚展示 `site.privacy_url`、`site.terms_url`、`site.contact_url` 和可选的 `site.filing_text` / `site.filing_url`。链接要求 HTTPS，由部署方维护实际页面，配置链接不会自动生成政策页。不要公开占位政策或登记信息。

## 部署验收 [#deployment-verification]

1. 将数据库与包文件配对备份，并恢复至隔离目标，参见[数据库维护](database.md)。schema 1 包含个人空间、包计量和举报；不要让旧程序打开升级后的数据库。
2. 检查配置和密钥权限，保留独立本地管理员，开放流量前确认模式、匿名访问策略和审核者。
3. 注册真实回调来源，测试成功/拒绝登录与初始资料；禁用 Hub 账号，确认 Web/App/CLI 令牌失效。
4. 普通成员投稿应待审且不能自审；另一个有权账号批准后安装精确版本；举报并撤下后，新读取/下载应被拒绝，已安装文件应保留。
5. 检查代理真实 IP、HTTPS、限流、身份/头像服务连通性、磁盘告警、备份保留与异机恢复；按实际访问与下载量测量目标服务器。仓库隔离测试不代表真实凭据或服务器容量已验收。
6. 发布真实的联系、隐私、条款与必要域名登记信息，开放公网前完成部署地要求。

两种模式复用审核、数据库、包存储和本地安装能力，不增加运行服务，不执行 Agent，也不要求用户属于特定 GitHub 组织。
