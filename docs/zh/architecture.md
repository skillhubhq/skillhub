# 架构

本地配置由共享客户端内核负责，保存在独立的 `local-configuration/` 私有目录中，并复用系统凭据接口。写操作先取得安装锁、再取得配置锁，通过暂存文件和原子替换提交；系统凭据写入前先持久化无密钥值的引用，便于中断后诊断、重试和清理。绑定包含来源身份、安装位置、精确版本及摘要。此数据没有远程 RPC、值导出或 Agent 执行入口。参见[本地配置与密钥](local-configuration.md)。

## 进程与存储 [#processes-and-storage]

Hub 单 Go 进程内嵌 Web，使用 SQLite/PostgreSQL 元数据及内容寻址 ZIP 包。CLI/App 共享 Go 内核，桌面 Wails + React/TypeScript 独立模块隔离原生依赖。

SQLite 用本地 WAL。PostgreSQL 经 `database/sql`、pgx、方言适配和事务初始化，在初始化前获取会话 advisory lease；查询绑定物理连接，失去所有权即拒绝请求、关闭服务。替换先停旧进程，不支持多在服副本或共享网络包目录。

## 协议 [#protocol]

- `proto/api/<domain>/v1` 定义 unary Connect 服务，common 定义分页/错误/枚举。
- 生成的 Go/TS 客户端是传输边界，领域对象和持久化不用 API 消息，不另建 REST 业务 API。
- Protobuf/ProtoJSON 同样授权，公共错误为安全 Connect code/details，不含秘密或包正文。
- 凭据验证产生身份，幂等重试仍校验当前权限；游标绑定身份/过滤/排序，未知枚举拒绝。
- 注册表声明协议主版本和能力名，旧 Hub 不支持时按连接明确报错，不静默忽略参数。
- `make proto` 生成双端，`make proto-check` 检查漂移；已发布字段身份不可复用，桌面构建刷新原生 bindings。

## 身份与授权 [#identity-and-authorization]

本地账号与可配置 OIDC/GitHub 适配器，外部身份绑定验证后的身份域/subject，不按邮箱；待审批不签发会话。成员资格与命名空间/平台权限分离。

浏览器用 HttpOnly/SameSite cookie、Origin 和自定义头检查；CLI/App 在 OS 凭据服务保存各 Hub 令牌。第三方登录的 state/PKCE（OIDC 另含 nonce）一次性、五分钟过期，重启失效，不保留 IdP 令牌。登录/回调仅是浏览器传输端点。

每次内容操作复核版本状态/受众，分组、命名空间角色、平台职责分离。社区资料按权限脱敏登录身份，见[权限](identity-and-access.md)。

## 目录与治理 [#catalog-and-governance]

版本不可变，包含摘要、受众和展示元数据。先选最新可读已发布版本再过滤；最近排序用发布序列，游标稳定。精选由 Hub 管理员按 namespace/name 设置，不绕过权限。

待办是可操作待审提交的实时查询，排除本人，数量/最早时间/列表一致读取。决定、审计、提交者通知同事务，通知所有权与权限实时复核。包检查有界离线，报告绑定摘要和扫描器版本，阻断不可覆盖，警告/不完整需独立审核与说明。

评测 RPC 存储严格校验的外部 JSON，绑定版本包摘要；公开读取沿用受众，未发布读取限定履职角色，维护者才能导入撤回。报告不参与执行或自动批准，见[评测证据](evaluations.md)。

## 社区 [#community]

个人/团队主页、评论、互动、头像和评测随 Hub 元数据备份。评论最多 4000 Unicode 字符，每用户每分钟 20 条，回复仅指向同版本未删除根评论，删除留墓碑。主页 revision 防并发覆盖，读写均复核权限。

头像仅 PNG/JPEG，512 KiB、2048×2048 上限，裁剪为 128×128 PNG 去元数据，不抓任意外链；GitHub 登录适配器仅可从固定 CDN 初始化未编辑过的空头像，失败回退首字母。

## 共享客户端内核 [#shared-client-core]

`internal/client` 管理源、凭据、远端客户端、包、目标适配、计划、安装清单和项目；不依赖终端/UI 框架。跨进程锁协调写入，SQLite 事务和安装日志跨越文件系统/数据库崩溃边界；执行复核单次计划、权限、摘要和未修改目标，物理路径不能重复归属。

安装记录保存 Hub ID/URL、精确版本/摘要和不透明连接引用。恢复连接需确认同一 Hub，在目标锁下改记录，不改技能文件。离线列表读取保存展示元数据。

清单记录精确版本/工具，锁记录 manifest 摘要、源身份、URL、归档摘要和相对路径。恢复只用配置源，预检目标、重新授权、逐目标报告。ZIP 缓存最多 128 项/256 MiB、LRU、独立锁/原子写；复用需在线授权，故障回退网络而非离线授权。

## 共享界面 [#shared-interface]

React 构建 Web/Wails，类型化 adapter 选择 Connect 或原生 Go bindings。源上下文显式，本地清单无全局 Hub 身份。语言/主题属于设备，作者内容不自动翻译；原生标题栏跟随主题。

技能以有界文本/Markdown 渲染，不执行脚本；浏览器提供指引/下载，原生端按计划写盘。软件更新仅联系固定分发服务，不信任 Hub 提供的可执行源。

`/docs/` 与 `/docs/zh/` 是 Next.js/Fumadocs 静态导出，复用头部/品牌、各语言本地搜索；逐页 CSP 只允许生成的 inline script 摘要。文档公开且不含 Hub 私有数据，Node 仅构建依赖。

## 维护与验证 [#maintenance-and-verification]

SQLite 快照配对精确引用包；PostgreSQL 独占所有权下原生 dump 配包。恢复要求新目标，保留实例身份、撤销会话/设备并记录审计，标记阻断不完整启动。回退通过快照，不原地降 schema。

结构校验、恢复和权限检查是持续产品行为。运行报告、部署参数和决策记录放 CI 或忽略目录，不放开源源码。见[数据库](database.md)、[运维](operations.md)和[测试](testing.md)。
