# 测试

使用可丢弃数据库、临时客户端和合成包，集成测试不可指向共享 Hub/生产库。运行报告放 CI 或忽略目录，本指南只保留可复现流程。

## 软件验收范围 [#software-acceptance-scope]

代码验收覆盖协议、身份/授权（本地 OIDC）、发布审核安装、App 状态保留、数据库初始化与恢复、安全和打包。确切候选 revision 需跑完整支持平台 CI。

真实 Entra、组织服务器/代理/存储、全新受管设备安装属于运维后续集成验收，不阻塞代码验收，也不能被本地成功替代。使用[部署检查](deployment-checklist.md)单列状态。

## 核心与协议 [#core-and-protocol]

```sh
make check
make quality
make web
go build -o bin/skillhub-server ./cmd/skillhub-server
go build -o bin/skillhub ./cmd/skillhub
python3 scripts/smoke.py --server bin/skillhub-server --cli bin/skillhub
make sdk-smoke
```

check 覆盖 Buf 格式/lint/build、Go vet/race 和 SDK；quality 检查分发源码卫生、文档链接与打包。真实 CLI 冒烟覆盖独立 Hub/身份、发布审核、两种工具、项目恢复、缓存、恢复和部分源失败。

协议改动先 `make proto` 并提交生成输出，`make proto-check` 在含预期代码的 checkout 对比，CI 另检查 breaking change，不能手改生成文件。

评测可定向运行 `go test -race ./internal/evaluation ./internal/hub ./internal/hubhttp ./internal/client ./internal/cli`，覆盖格式、版本摘要、权限、导入/撤回和恢复。示例 JSON 应通过离线验证；Web/App 使用同一显示组件。

## Web 与桌面 [#web-and-desktop]

```sh
npm --prefix desktop/frontend test
npm --prefix desktop/frontend run build:web
npm --prefix website test
node desktop/build.mjs --native-smoke
python3 scripts/desktop_fixture.py --automated \
  --app desktop/bin/SkillHub-smoke.app/Contents/MacOS/SkillHub \
  --report .local/native-runtime.json
```

Windows 用 `desktop/bin/SkillHub-smoke.exe`。fixture 建临时 Hub/客户端/包，经真实 WebView/Go bridge 操作、独立 CLI 验证后清理。原生交互需解锁桌面，正常分发前重建无 harness 的 App。

在 `desktop/` 运行：

```sh
CGO_CFLAGS=-mmacosx-version-min=13.0 \
CGO_LDFLAGS=-mmacosx-version-min=13.0 \
MACOSX_DEPLOYMENT_TARGET=13.0 go test -race ./...
```

真实 OS 凭据测试仅显式 `SKILLHUB_TEST_OS_CREDENTIALS=1` 开启，要求隔离 OS 账号、可用凭据服务和已构建 CLI。手动可运行 `scripts/desktop_fixture.py --app PATH_TO_APP_EXECUTABLE`，检查浅深主题、中英文、窄窗口、键盘、菜单、弹窗、原生选择器、错误/空态及发现→安装、发布→独立审核→反馈、账号权限全流程。浏览器不能替代原生系统集成。

文档检查英文/中文完整目录、正文/搜索、切换保留对应页、可用章节锚点和主站头部；build 校验静态链接与搜索产物。新增指南需成对维护。

## 数据库与容量 [#database-and-capacity]

通过受控环境注入 `SKILLHUB_TEST_POSTGRES_DSN`，只指向能创建/删除临时库的可丢弃 PostgreSQL。PATH 装匹配主版本 dump/restore。

```sh
go test -race -count=1 ./internal/hubhttp
go test -count=1 -run TestPostgres ./internal/hub
go test -count=1 -run TestDiscoveryCapacity -v ./internal/hub
```

未设 DSN 时跳过，不代表通过。CI 覆盖 15/16/18，检查租约、初始化取消、配套恢复、损坏/缺包、失效会话、新目标回退和未完成标记。容量以合成内容验证权限分页、最新可读版本、精选、并发待办；记录宿主/工具版本，不把合成延迟当 SLA，实际负载在目标环境测量。

## 兼容性 [#compatibility]

```sh
python3 scripts/compatibility.py \
  --server /path/to/candidate-server --cli /path/to/candidate-cli \
  --baseline-server /path/to/comparison-server \
  --baseline-cli /path/to/comparison-cli
```

隔离测试双向新旧客户端/服务端、共享状态、基线快照升级和新目录回退，验证身份、权限、包、安装和恢复会话撤销。CI 以 PR base/前一 main 为对照，保留双方 revision；发布前还应与上一支持发布版比较，首次发布不存在历史发布基线。单对成功不是任意兼容承诺。

### 原生 App 覆盖替换 [#native-app-replacement]

```sh
python3 scripts/desktop_upgrade.py \
  --baseline-ref COMPARISON_COMMIT \
  --candidate-app desktop/bin/SkillHub-smoke.app \
  --server bin/skillhub-server --cli bin/skillhub \
  --report .local/native-upgrade.json
```

基线为明确完整 40 位小写 SHA，Windows 使用 `.exe`。解锁桌面/凭据服务。工具构建 detached 对照，仅覆盖测试 instrumentation 并记录哈希，不把候选代码冒充基线；也可明确传已带 instrumentation 的 `--baseline-app`。

种入两 Hub 真实 OS 凭据、有效/不完整两草稿、两工具×全局/项目四安装、manifest/lock 和本地修改，后续移除登录密码，不用环境令牌替代已保存凭据。依次基线重启、同路径覆盖候选、重启、兼容基线回退；macOS 用校验后的只读 DMG。比较状态/凭据/文件摘要和真实认证，候选重开草稿。

fixture 使用独立应用/凭据身份，不碰生产状态。不模拟浏览器 quarantine、签名身份变化、跨用户设备或未来不兼容 schema。不同发行身份的基线仅在测试中规范化标识并记录 `fixture_identity_normalization`，验证格式兼容，不证明跨应用身份自动迁移；真实发行应保持标识稳定。

## 安全与部署 [#security-and-deployment]

CI 扫秘密和依赖，静态扫描只是证据，发布前审阅归档/依赖。真实环境地址、租户、凭据和完成报告不入源码。CodeQL 分析 Go、JS/TS、Python、Actions 的远端/本地输入；任务成功只表示扫描运行。真实问题修复，误报需数据流/验证依据，不能禁规则隐藏问题。参见[安全策略](security.md)和[部署检查](deployment-checklist.md)。

## 登录适配器与社区策略 [#login-adapters-and-community-policy]

运行 `go test -race ./internal/identityprovider ./internal/hubhttp ./internal/hub ./internal/serverconfig`，验证配置、OIDC 签名与字段、OAuth PKCE、state/cookie/重放隔离、稳定身份、按模式准入、禁用/待审状态保留、安全头像初始化、配额、举报处理和 结构初始化与来源字段升级保留。适配器测试使用受控 HTTP transport，OIDC 使用本地 TLS 服务和真实签名，不要求或宣称使用真实提供方凭据。

`TestPostgresCommunityPolicyAndModeration` 沿用 PostgreSQL 隔离测试库，覆盖 15、16、18。原生夹具验证举报弹窗、共享 Go 方法、队列权限、处理结果与评论占位。全部数据可丢弃，不打开部署方当前 Hub 数据。

## 团队生命周期

`TestNamespaceLifecycle` 与 `TestPostgresNamespaceLifecycle` 验证管理员权限、修订号和确认名称、归档可见性、恢复、共享包保留、永久清理、共享身份保留、命名空间占位与幂等重试。`TestNamespaceCleanupFailureCanRetry` 模拟文件删除失败。结构升级测试覆盖版本 11 到 12 的身份保留。传输层及 CLI 回归拒绝未授权和未确认操作；原生 smoke 会打开共用弹窗、归档临时团队、检查输入名称门禁并完成清理。所有写入型验收仅使用可丢弃的隔离实例。
