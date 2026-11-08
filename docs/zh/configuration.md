# Hub 配置

通过 `serve --config PATH` 或 `check-config --config PATH` 显式加载 YAML。不自动发现 `.env`、不递归合并覆盖配置，也不直接连接云密钥服务。

优先级：内置默认值 → 指定 YAML → 显式命令行参数。`false` 是有效覆盖值。相对路径基于进程工作目录，部署建议用绝对路径。配置上限 64 KiB，拒绝未知字段、重复键、多文档和错误类型；错误不会回显 YAML 值。

```yaml
data: /var/lib/skillhub/data
listen: 127.0.0.1:8080
public_url: https://skills.example.com
name: Company Skills
private: true
trusted_proxies:
  - 127.0.0.1/32
```

此代理示例只适用于直接连接的同机代理。容器可在隔离网络内监听 `0.0.0.0:8080`，外部访问由部署网络限制。不能未经检查就把回环代理配置套用于负载均衡器或 CDN。

| YAML 字段 | 命令行参数 | 默认值 |
| --- | --- | --- |
| `database.driver` | `--database-driver` | `sqlite`，另支持 `postgres` |
| `database.dsn_file` | `--database-dsn-file` | 空；也可用 `SKILLHUB_DATABASE_DSN` |
| `data` | `--data` | `.local/hub` |
| `listen` | `--listen` | `127.0.0.1:8080` |
| `public_url` | `--public-url` | `http://127.0.0.1:8080` |
| `name` | `--name` | `SkillHub` |
| `private` | `--private` | `true` |
| `trusted_proxies` 列表 | `--trusted-proxies` 逗号分隔 | 空 |

公开地址必须是 origin，不能带部署子路径或查询参数；非回环地址要求 HTTPS。Web 内嵌，不需配置前端 API 地址。数据库可选 SQLite/PostgreSQL，包仍存本地，无 Redis/S3 依赖。参见[数据库](database.md)。

## 企业登录与密钥 [#enterprise-login-and-secrets]

```yaml
mode: enterprise
auth:
  providers:
    - id: company
      type: oidc
      name: Microsoft
      issuer: https://login.microsoftonline.com/TENANT_ID/v2.0
      client_id: APPLICATION_CLIENT_ID
      client_secret_file: /run/secrets/skillhub-identity
```

替换占位符；密钥文件仅包含值，可带一个结尾换行，只允许服务账号和可信运维读取。也可使用 `client_secret_env: SKILLHUB_IDENTITY_SECRET` 并注入对应环境变量；二者同时配置会报错。不支持 YAML 内联 `client_secret`。密钥上限 8 KiB，永不打印。

服务管理器可注入环境变量或挂载密钥文件，Hub 本身不依赖云 SDK。修改后需重启。回调地址为 `PUBLIC_URL/auth/providers/company/callback`。首次准入仍需结合身份提供方的应用分配，参见[企业登录](enterprise-login.md)。

## 校验、初始化、运行 [#validate-initialize-run]

```sh
skillhub-server check-config --config /etc/skillhub/hub.yaml
skillhub-server init --config /etc/skillhub/hub.yaml --admin admin --password-stdin
skillhub-server serve --config /etc/skillhub/hub.yaml
```

初始化仅执行一次。`check-config` 检查格式、设置和密钥可用性，不建库、不联网，也不能证明存储权限、密钥有效性或代理路由正确。`serve` 执行 OIDC 发现，不可达会启动失败。维护命令使用同一配置，SQLite/PostgreSQL 均支持备份、校验和恢复，见[维护流程](database.md)。

## 部署方可自定义什么 [#what-an-operator-can-customize]

配置控制社区名称、公开地址、监听/存储、公开/私有读取、代理信任和企业登录。改名称不改变不可变 registry ID 或内容。Web 读取当前名称；已有 App/CLI 连接缓存添加时名称，不自动同步重命名，本地别名由使用者选择。

应用内管理用户、分组、命名空间权限、分类和标签；命名空间管理员编辑团队主页及审核策略，用户编辑个人资料。这些保存在数据库中，不是 YAML 设置。

暂不提供自定义社区 Logo、首页文案/布局、法律/联系页、全站默认语言/主题或自定义下载目标。语言主题为设备偏好。软件 GitHub 项目和下载渠道独立于社区名称；分叉可按许可证改源码和构建链接，但不是运行时管理功能。

## 模式与登录适配器 [#modes-and-login-adapters]

`mode` 默认为 `enterprise`，也可设为 `community`；模式决定准入、投稿和配额策略，`private` 独立决定匿名可见性。登录方式通过 `auth.providers` 配置，最多八个 OIDC/GitHub 入口，任一适配器都可以用于任一模式。参见[社区部署与登录适配器](community-hosting.md)。

```yaml
mode: enterprise
auth:
  providers:
    - id: company
      type: oidc
      name: Microsoft
      issuer: https://login.microsoftonline.com/TENANT_ID/v2.0
      client_id: APPLICATION_CLIENT_ID
      client_secret_file: /run/secrets/skillhub-identity
```

回调地址为 `PUBLIC_URL/auth/providers/company/callback`。也可使用 `client_secret_env: SKILLHUB_IDENTITY_SECRET`，与密钥文件互斥。原有 `oidc` 配置/参数继续使用原回调及密钥规则，但不得与 `auth.providers` 混用。`limits` 设置社区配额，`site` 设置站点隐私、条款、联系及登记链接，均为 YAML 配置。
