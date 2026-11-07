# 桌面 App

Wails v3、React 与 TypeScript 调用 CLI 共用的 Go 内核。导航分为社区、本地技能、创作，连接和偏好在设置中。不运行 Agent、包脚本或后台客户端守护进程。

## 构建与运行 [#build-and-run]

在 `desktop/` 下执行：

```sh
cd frontend
npm ci --ignore-scripts
cd ..
node build.mjs
```

macOS 13+ 生成 `bin/SkillHub.app`，Windows 生成 `bin/SkillHub.exe`，需要各平台原生构建依赖。独立 Go 模块避免 Hub/CLI 引入桌面依赖。开发构建不提供正式签名/公证。

Go→TypeScript bindings 生成并检入；构建前自动刷新，CI 检查已跟踪及新增漂移。包内脚本和 HTML 仅作文本预览，不执行；读取复核权限和原始包摘要。

## 初次使用 [#first-use]

1. 设置 → 连接，添加 Hub URL/别名并登录。每连接独立身份，也可查看清理缓存。
2. 社区 → 发现，默认搜索当前 Hub，也可明确选全部连接。预览版本、选择工具/目录、检查计划后安装。
3. 本地技能 → 已安装，检查更新或移除。更新沿用来源，本地修改阻止静默替换。
4. 本地技能 → 项目，打开/创建清单、选择版本、锁定和恢复。更新安装不自动改项目锁。
5. 创作中编辑保存草稿、提交、读反馈、处理审核和有权管理的 Hub 设置。版本评测可导入外部 JSON，Hub 不执行评测。

CLI/App 默认共用 OS 配置目录的 `skillhub`；`SKILLHUB_HOME` 可选择隔离目录。会话在系统凭据存储，环境令牌只用于明确注入，不写项目。凭据服务锁定/不可用是错误，不会悄悄匿名访问私有源。

计划单次使用，五分钟过期，失败或过期需重建。批量恢复逐目标报告，一个失败不回滚其他成功项。

## 验收 [#acceptance]

在桌面模块运行：

```sh
CGO_CFLAGS=-mmacosx-version-min=13.0 CGO_LDFLAGS=-mmacosx-version-min=13.0 \
MACOSX_DEPLOYMENT_TARGET=13.0 go test -race ./...
```

Windows 不需要 macOS 的链接环境变量。`node build.mjs --native-smoke` 构建独立测试 App，通过真实 WebView/Go bridge 连接临时 Hub；CI 在 macOS/Windows 保存 JSON 结果。测试完成后构建正常 App，不能把测试 harness 分发给用户。

真实凭据测试需先构建 CLI，并显式设置 `SKILLHUB_TEST_OS_CREDENTIALS=1`，以独立 OS 账号和可访问凭据服务运行 `TestDesktopUsesSystemCredentialStore`；临时引用验证共享登录、退出、撤销和源隔离，不输出凭据。普通测试跳过以避免弹窗。视觉、系统文件选择器、浏览器授权和全新设备安装需单独验证，不能只看构建成功。见[测试](testing.md)。
