# Hub 与 CLI 使用

[文档首页](index.md) / Hub 与命令行

CLI 支持发布审核、独立数据源、安全安装/更新/移除和项目恢复；App 通过同一 Go 内核提供这些能力，见[桌面说明](desktop.md)。

## 命令行基础 [#terminal-basics]

```sh
skillhub --help
skillhub search --help
skillhub install --help
skillhub source list
skillhub list --json
```

帮助按发现、本地使用、发布和账号分组，子命令 `--help` 提供参数和示例。人类输出有摘要、标识与空态；只有终端启用颜色，`--no-color`、非空 `NO_COLOR` 或 `TERM=dumb` 可关闭。自动化必须用 `--json`，不要解析展示布局。错误写 stderr，非零退出。部分源搜索失败时保留成功结果并报告失败源。

安装、更新、移除和项目恢复先显示计划，`--dry-run` 只预览并成功退出，`--yes` 才执行；两者均未指定时，预览后非零退出，表示没有应用变更。`skillhub completion bash|zsh|fish|powershell` 提供补全，安装方式见对应帮助。

## 获取版本包 [#versioned-preview-releases]

从 [GitHub Releases](https://github.com/skillhubhq/skillhub/releases) 选择 OS/架构，核对 `SHA256SUMS`。Hub 包内嵌 Web；App 使用手动安装，签名和平台限制见[分发](distribution.md)。

## CI 构建 [#download-private-ci-builds]

可从完整通过且 revision 正确的 [CI](https://github.com/skillhubhq/skillhub/actions/workflows/ci.yml) 下载开发产物：Linux 为 `hub-cli-ubuntu-latest`，macOS 为 `hub-cli-macos-latest`/`desktop-macos-latest`，Windows 为 `hub-cli-windows-latest`/`desktop-windows-latest`。

```sh
gh run download RUN_ID --repo skillhubhq/skillhub \
  --name hub-cli-macos-latest --dir ./skillhub-build
chmod u+x ./skillhub-build/skillhub ./skillhub-build/skillhub-server
```

macOS App 需解开内部 `SkillHub-macos.zip` 保留 bundle 和执行权限；Windows 使用 `.exe` 与 WebView2。`native-runtime-*` 是测试证据，不是安装包。CI 产物有保留期且不是正式版本渠道，其他架构请源码构建。

## 构建与初始化 [#build-and-initialize]

```sh
make web
go build -o bin/skillhub-server ./cmd/skillhub-server
go build -o bin/skillhub ./cmd/skillhub
bin/skillhub-server init --data .local/hub --admin admin --password-stdin
bin/skillhub-server serve --data .local/hub --public-url http://127.0.0.1:8080
```

stdin 提供 12–72 字节初始密码，不放命令参数。默认私有；`--private=false` 允许匿名浏览已发布 Public 版本。非回环地址需 HTTPS，可由代理终止 TLS。

## 连接独立源 [#connect-independent-sources]

```sh
skillhub source add work https://skills.example.com
skillhub source add public https://public-skills.example.com
skillhub source list
skillhub login work
skillhub whoami --source work
```

`login` 输出浏览器授权 URL/短码并轮询，需明确批准；本地账号也可 `login work --username admin --password-stdin`。凭据保存在 OS 凭据存储，不写项目文件。CLI/App 共用 `SKILLHUB_HOME` 或 OS 配置目录下的 `skillhub`。

自动化可注入 `SKILLHUB_TOKEN_WORK`（别名大写、连字符转下划线），不会保存。无凭据的公开源不需凭据服务；已有凭据丢失则要求重登，不悄悄退为匿名。`logout work` 撤销当前会话并删除本地凭据，远端失败单独报告；`source remove work` 仅移除连接和本地凭据，需要远端撤销时先退出。退出不清安装/缓存，缓存复用仍检查当前权限。

## 命名空间与独立审核者 [#configure-a-namespace-and-independent-reviewer]

```sh
skillhub admin namespace-put platform --source work
skillhub admin user-add reviewer --source work --password-stdin
skillhub admin grant platform USER_ID --source work --role reviewer
skillhub admin users --source work
skillhub admin grants platform --source work
skillhub admin group-add engineering --source work
skillhub admin group-member GROUP_ID USER_ID --source work
skillhub admin grant platform GROUP_ID --source work --kind group --role maintainer
skillhub admin category-put operations --source work --title Operations
```

新命名空间默认需审核，仅 Hub 管理员创建；命名空间管理员可改策略/角色，策略与分类变更需当前 `--revision` 防止覆盖并发修改。Reader、Maintainer、Reviewer 分离，命名空间管理员继承三者；没人能自审。分组可作为受众或角色主体，成员变化在后续请求生效；分类/标签不授予权限。Web/App 使用搜索选择用户、分组和命名空间，CLI 使用明确 ID。

## 发布与审核 [#publish-and-review]

App 从当前源有提交权限的命名空间中选择，分类取当前 Hub 未归档目录；源变化会重新加载，服务端仍复核。标签为自由文本。

```sh
skillhub publish ./release-check --source work --namespace platform \
  --version 1.0.0 --visibility groups --group GROUP_ID --category operations --tag release
skillhub review list --source work
skillhub review inspect RELEASE_ID --source work
skillhub review approve RELEASE_ID --source work --reason "Reviewed fixed package"
skillhub review reject RELEASE_ID --source work --reason "Missing verification steps"
skillhub yank RELEASE_ID --source work --reason "Withdrawn version"
```

根目录 `SKILL.md` 必须有有效 name/description。限制为 ZIP 20 MiB、总展开 80 MiB、单文件 8 MiB、主 Markdown 256 KiB。校验路径、符号链接、重复、大小写碰撞和元数据，不执行内容。

版本使用完整语义版本号。已发布坐标撤下后仍不可复用；待审核或被拒绝的拟定版本可提交新修订，使用新的审核标识。不要求审核的命名空间可直接发布，扫描警告仍可强制独立审核。目录默认选择最高稳定版本；没有稳定版时选择最高预发布版。发布通过后，技能展示信息和统一受众规则同步更新。普通目录/下载不返回待审核、拒绝或撤下包，管理员也不例外；审核接口另有受控预览。

远端变更可用 `--request-id UNIQUE_KEY` 幂等重试，主体/操作/参数必须一致，重试也重新授权。外部效果报告见[评测证据](evaluations.md)。

## 搜索、安装、更新与移除 [#search-install-update-and-remove]

```sh
skillhub search release --source work
skillhub search release
skillhub info work/platform/release-check
skillhub versions work/platform/release-check
skillhub install work/platform/release-check@1.0.0 --tool codex --project ./project --dry-run
skillhub install work/platform/release-check@1.0.0 --tool codex --project ./project --yes
skillhub install work/platform/release-check@1.0.0 --tool claude-code --scope global --yes
skillhub updates
skillhub update /absolute/path/to/managed/skill --dry-run
skillhub update /absolute/path/to/managed/skill --yes
skillhub update /absolute/path/to/managed/skill --version 1.0.0 --yes
skillhub remove /absolute/path/to/managed/skill --yes
```

未指定源时搜索全部配置源，每源保留独立结果/错误/游标；游标只能用于相同源和筛选。安装其他版本到同一受管目标即更新，拒绝覆盖未受管、本地修改或不同 Hub 同名内容，不执行包脚本。

默认更新选择高于当前的最高可读稳定语义版本，不自动选预发布或降级。显式 `--version` 可选预发布/降级，仍需计划并保护本地修改。来源使用安装时记录，与正在浏览的社区无关。多个身份匹配同一 Hub 时需明确选择连接；安装更新不改项目清单/锁。

| 工具 | 全局 | 项目 |
| --- | --- | --- |
| Codex | `~/.agents/skills/NAME` | `PROJECT/.agents/skills/NAME` |
| Claude Code | `~/.claude/skills/NAME` | `PROJECT/.claude/skills/NAME` |
| Cursor | `~/.cursor/skills/NAME` | `PROJECT/.cursor/skills/NAME` |
| Gemini CLI | `~/.gemini/skills/NAME` | `PROJECT/.gemini/skills/NAME` |
| GitHub Copilot | `~/.copilot/skills/NAME` | `PROJECT/.github/skills/NAME` |

Claude 保留的 `synced`、`anthropic-skills` 名称会拒绝。暂不自动发现自定义工具配置根目录。无需后台守护进程。

## 共享下载缓存 [#shared-download-cache]

```sh
skillhub cache info
skillhub cache clear --yes
```

CLI/App 共用缓存，App 设置连接页可查看/清理。最多 128 个坐标、256 MiB 压缩包，LRU 淘汰；键绑定规范 URL、实例 ID、命名空间、名称和精确版本，不跨 Hub 复用。

命中仍以当前身份在线检查下载权限、状态和服务端 blob 完整性，再验证本地摘要/结构。离线、撤下或撤权不会允许缓存安装。损坏缓存触发重新授权下载，缓存不可用不阻止有效全量下载。缓存是受目录权限保护的明文，不是加密存储。清理仅删受管缓存，保留凭据、安装和项目；临时文件/并发用独立锁协调，不长期持有网络请求锁。

## 可复现项目 [#reproducible-projects]

```sh
skillhub project init --project ./project
skillhub project add work/platform/release-check@1.0.0 --tool codex --project ./project
skillhub project lock --project ./project
skillhub project show --project ./project
skillhub project restore --project ./project --dry-run
skillhub project restore --project ./project --yes
skillhub project list
```

提交 `skillhub.json` 和 `skillhub.lock.json`，忽略 `.skillhub-project.lock` 与 `.skillhub-write-*`。生成的工具目录是否提交由项目决定。清单记录精确版本、别名、坐标和工具；锁额外固定 URL、实例 ID、SHA-256 和相对目标，不含凭据。schema 1 最多 100 项、每文件 1 MiB，不支持版本范围或隐式 latest。

其他机器先配置/登录源，再 restore；不会自动添加或请求锁中未配置 URL。同 URL/实例唯一匹配时允许不同本地别名，但重新 lock 仍使用清单别名。

恢复在变更前校验全部计划，执行时复查身份、授权、摘要、文件和元数据，不覆盖本地修改、未受管目录或逃逸符号链接。撤下/撤权不能靠锁或缓存绕过，已安装文件不会远程撤回。逐目标提交，部分失败不回滚已成功目标；修复后重试，无隐式删除清单外安装。

更新共享要求需重新 add 精确版本、lock、检查并 restore。`project remove NAME --tool TOOL` 仅删期望条目，实际卸载用 `skillhub remove TARGET`。修改清单使锁失效，元数据写入采用临时文件/原子替换。`project show` 注册快捷入口，`project forget --project PATH` 仅忘记入口，保留项目文件和安装。

App 与 CLI 共用本地数据时应一起升级。升级前备份已关闭的客户端数据目录；回退到旧结构版本时，应恢复对应的数据快照和程序。

## 备份与恢复 [#hub-backup-and-recovery]

```sh
skillhub-server backup --data .local/hub --output .local/hub-backup
skillhub-server verify-backup --input .local/hub-backup
skillhub-server restore --input .local/hub-backup --data .local/hub-restored
```

目标必须新建且在源外，父目录存在。备份含一致数据库及引用包。恢复保留实例/治理并使旧会话和设备授权失效。使用与运行服务同版本程序备份，完整流程见[运维](operations.md)。

## 可执行冒烟 [#executable-smoke-test]

```sh
python3 scripts/smoke.py --server bin/skillhub-server --cli bin/skillhub
```

测试启动两个临时 Hub、隔离凭据，通过真实 CLI 验证发布审核、两种工具、项目恢复、在线备份/恢复、部分源失败与旧会话拒绝，不使用保存的凭据或修改真实全局技能目录。

## Web 与共享桌面体验 [#web-ui-and-shared-desktop-review]

`make build` 构建并内嵌 Web；Node/npm 仅构建时需要。仅编 Go 且无 Web 资产时 `/` 明确返回 503，RPC 仍可用。浏览器打开配置的 public URL，私有 Hub 需登录；浏览器使用 HttpOnly 会话，代理需保持精确外部 origin。

Web 仅展示自身 Hub，站点名与 Logo 合并；App 可管理多个独立连接。两者共享社区、创作、评测、审核和管理界面，默认英文，支持简体中文及浅色/深色/系统主题。Web 安装提供 CLI 操作指引和归档下载，不能直接写本机工具目录；App 则展示路径和计划后安装。源码文档是公开软件资料，即使私有 Hub 的 `/docs/` 也公开，不包含社区运行数据。

点赞、收藏和讨论以技能为单位。收藏每个技能只显示一项，默认打开最高稳定版本；评论标注讨论的版本。升级或清理旧包不会清空这些互动，技能访问规则统一作用于全部版本和讨论。失效收藏仍可由本人移除。

### 版本差异与更新订阅

在技能详情或投稿预览中选择「比较版本」，指定基线版本，查看新增、删除和修改的文件，包括可执行权限。文本按行标记差异，大文件和二进制文件显示变更状态。比较双方都需对当前账号可访问。

```sh
skillhub compare company/team/review 1.0.0 1.1.0
skillhub subscriptions follow company/team/review
skillhub subscriptions list --source company
skillhub subscriptions unfollow company/team/review
```

「订阅更新」跟随技能而不是单个版本。新版本正式发布时，有访问权限的订阅者会收到站内通知；待审核提交不会触发通知。发布与通知在同一事务内完成。访问权撤销后，通知会隐藏内容。在「通知 → 我的订阅」可管理订阅，包括取消已不可访问的技能。订阅不会自动修改本地文件。

### 技能套装

套装通过命名空间、名称、精确版本和 SHA-256 摘要，固定同一 Hub 中的 1–50 个技能。在「发布 → 创建技能套装」中搜索技能、选择版本并设置入口技能。套装复用团队的发布权限、扫描与审核流程。投稿、审核和下载都会检查每个成员当前的访问权限，套装不会额外授予访问权。套装不能嵌套，也不能含有目标文件夹同名的技能。

App 和 CLI 的安装、更新、移除入口会展开套装，先展示所有文件变更。入口文档指明从哪个技能开始，Hub 不执行技能。Web 详情展示套装成员，提供「在 SkillHub 中打开」与 CLI 安装指引。

```sh
skillhub install company/team/starter@1.0.0 --tool cursor --scope project --dry-run
skillhub install company/team/starter@1.0.0 --tool cursor --scope project --yes
skillhub suites list
skillhub update /path/to/project/.cursor/skills/starter --yes
skillhub remove /path/to/project/.cursor/skills/starter --yes
```

移除套装会保留单独安装、被其他套装共用或用户修改过的成员。不会覆盖独立安装的冲突版本；被套装固定的成员应通过套装更新。每个目录原子写入，操作中断后保留已完成内容与归属记录，可以重试。套装总下载量上限为 64 MiB。项目清单目前记录单个技能；将套装安装到项目请使用 `install --scope project`。

CLI 发布：保存以下 JSON 定义，再运行 `skillhub suites publish suite.json --source company`：

```json
{
  "namespace": "team",
  "name": "starter",
  "version": "1.0.0",
  "description": "Tools for reviewing a change",
  "visibility": 2,
  "entry": "team/review",
  "members": [{"namespace": "team", "name": "review", "version": "1.0.0"}]
}
```

客户端提交前解析并固定摘要。`visibility: 2` 表示 Hub 成员，`1` 表示公开，仍受 Hub 访问策略约束。手工制作的包需要在 `SKILL.md` 同目录提供 `skillhub.suite.json`，包含 `schema: 1`、`entry` 和带 `digest` 的成员列表。

### 自动化访问令牌

在「账号 → 安全 → 自动化令牌」或 CLI 中创建：

```sh
skillhub tokens create build-read --source company --scope read --days 30 --output /secure/build-read.token
skillhub tokens create release-submit --source company --scope publish --namespace team --days 30 --output /secure/release-submit.token
skillhub tokens list --source company
skillhub tokens revoke TOKEN_ID --source company
```

App/Web 只展示一次密钥，CLI 写入仅当前用户可读的新文件。将密钥保存在 CI 的密钥管理功能中，为 `company` 连接注入 `SKILLHUB_TOKEN_COMPANY` 环境变量；别名中的连字符转换为下划线。不要放入命令参数或项目文件。

只读令牌可发现、比较和下载有权访问的技能；投稿令牌额外允许向指定团队提交，并遵循正常审核规则。令牌不能审核、管理 Hub、创建凭据或修改社区数据，始终受当前账号与团队权限约束。有效期 1–90 天，可立即撤销，每人最多 20 个有效令牌。账号安全变更和备份恢复会撤销令牌。技能运行时密钥与访问令牌分开管理，仍保存在本机。
