# 本地配置与密钥

CLI 与 App 共用设备上的本地配置。Hub 不接收配置值、凭据库引用或环境变量值。此功能管理配置要求，不执行 Agent、不向现有进程注入值、不生成 `.env` 文件，也不提供安全沙箱。

## 声明配置要求 [#declare-requirements]

技能作者可在 `SKILL.md` 旁添加 `skillhub.config.json`：

```json
{
  "version": 1,
  "variables": [
    { "name": "SERVICE_TOKEN", "description": "Token for the service used by this skill", "secret": true, "required": true },
    { "name": "SERVICE_REGION", "description": "Service region", "secret": false, "required": false }
  ]
}
```

这是 SkillHub 的可选声明，不是通用 Agent Skills 规范的一部分。技能仍可使用标准环境变量和工具自身的配置。声明只包含名称、说明和要求，不允许配置值、默认值、命令或可执行提供方。版本 1 最多支持 32 个唯一的大写变量名，文档不超过 32 KiB，未知字段会被拒绝。发布后，声明作为不可变技能包的一部分接受审核。

## 配置已安装技能 [#configure-an-installation]

在 App 中打开**本地技能 → 本机安装 → 配置**，或进入**本地技能 → 配置**。同一技能在不同项目、不同工具中的安装拥有独立配置。

| 保存方式 | 行为 |
| --- | --- |
| 系统凭据库 | 使用 macOS 钥匙串、Windows 凭据管理器或 Linux Secret Service；不可用时失败，不回退到明文保存 |
| 环境变量引用 | 检查当前进程中声明的同名变量，不将其值复制到 SkillHub |
| 本地非敏感值 | 将普通配置保存在客户端私有目录；声明为密钥的变量不可选择此方式 |

CLI 对应命令：

```sh
skillhub config list
skillhub config doctor /path/to/installed/skill
skillhub config set /path/to/installed/skill SERVICE_TOKEN --mode environment
skillhub config set /path/to/installed/skill SERVICE_TOKEN --mode keychain --value-stdin
skillhub config unset /path/to/installed/skill SERVICE_TOKEN --yes
skillhub config clear /path/to/installed/skill --yes
```

使用 `--value-stdin` 时，请通过管道读取可信凭据工具的输出，不要将密钥字面值写入命令或 shell 历史。诊断和 JSON 输出只显示可用状态，不返回配置值或钥匙串引用。必填项缺失、凭据库不可用或版本需要重新确认时，`doctor` 以失败状态退出；可选项缺失不会阻止就绪。

## 运行时与安全边界 [#runtime-and-security-boundaries]

保存后不会自动将值提供给 Codex、Claude Code 等进程。可使用下文的终端集成，或通过工具支持的启动方式配置运行环境。环境变量引用检查的是运行 SkillHub 的进程环境：从 Finder 启动的 App 与终端环境可能不同。修改启动环境后，应重启已有进程。

SkillHub 不执行密码库 CLI 命令。外部密码管理器可继续通过其标准环境变量工作流使用。向整个 Agent 提供变量，不代表该值被隔离到单个技能。同一操作系统用户的进程可能拥有相同的凭据库访问能力，因此这不是对已失陷用户账号的防护。

配置绑定 Hub 身份、来源地址、安装、项目和工具以及精确版本与摘要。升级后检查当前声明并授权新摘要，匹配的已有配置继续保留。本地已修改文件不能重新定义可信配置要求。卸载后保留配置，用户可在配置列表中主动清理；配置不会自动转移给新安装。

客户端备份包含 `local-configuration/` 中的非敏感配置和不透明绑定，不包含系统凭据库中的密钥值。密钥恢复依赖操作系统自己的备份机制。元数据与系统凭据更新若中断，诊断会报告缺失或不可用，可重试或清理保留条目。覆盖 App 时保留客户端数据目录的原位置；在不同设备或路径恢复元数据不会自动复制、重新绑定密钥。

## 在终端中使用已保存值 [#use-saved-values-in-your-terminal]

在同一设备安装 SkillHub CLI，并使用与 App 相同的客户端数据目录。在 **配置 → 在终端中使用** 选择 Bash、Zsh 或 PowerShell 7+，复制不含配置值的初始化与加载命令。此向导不会读取已保存值到界面，也不会启动终端或 Agent。

Zsh 示例（Bash 将 `zsh` 换成 `bash`）：

```sh
source <(skillhub env init zsh)
skillhub_env use /path/to/installed/skill
skillhub_env doctor /path/to/installed/skill
# 在当前终端自行启动所需工具。
# 使用结束后：
skillhub_env deactivate
```

PowerShell 7+ 示例：

```powershell
skillhub env init powershell | Out-String | Invoke-Expression
skillhub_env use 'C:\path\to\installed\skill'
skillhub_env doctor 'C:\path\to\installed\skill'
skillhub_env deactivate
```

`init` 仅输出集成代码、CLI 路径和客户端目录，不包含配置值、不修改 Shell 启动文件。PowerShell 执行的是可信初始化代码，不是密钥输出。`use` 通过本地管道读取配置，将值作为数据处理，仅导出到当前终端。内部传输拒绝直接输出到终端或普通文件。这是防止误操作的保护，不是阻止同一系统用户的其他进程访问凭据的安全边界。请使用可信、未录制的终端；Bash/Zsh 拒绝跟踪模式和 DEBUG trap，PowerShell 拒绝断点并在读取值前关闭表达式跟踪。主机级跟踪、调试和内存读取不在保护范围内。

用户必须明确选择一个或多个安装路径，不会自动汇总整个项目或所有 Hub 的变量。必填项缺失、安装内容变化、凭据库不可用、值格式不支持或多个安装声明同名变量时，会在改变环境前停止。`PATH`、`BASH_ENV`、`NODE_OPTIONS` 等运行时控制变量，以及 `OPENAI_API_KEY`、`ANTHROPIC_API_KEY` 等 Agent 登录变量不能由技能配置加载；Agent 自身登录应使用工具原有机制。

现有环境变量值不同时会报冲突；相同的已继承变量保持不变，可能被覆盖的 Shell 局部变量也会导致冲突。`deactivate` 仅删除集成新增的变量，不删除保存的配置、不撤销服务凭据，也无法清除运行中进程持有的副本。切换技能集合或重新初始化前请先清理。关闭终端也会结束它的环境。

`config doctor` 检查本地保存状态；`env doctor` 比较**当前进程**环境与保存值，仅输出变量名及 `present`（一致）、`missing`（缺失）、`different`（不同），不返回值。检查成功不代表服务登录成功，也不代表另一个进程已经收到变量。可在目标工具允许的命令执行环境中运行同一条不含值的 `skillhub env doctor TARGET` 来验证传递；不要把变量值打印到对话中。

Codex 会对启动的命令应用自己的[环境变量策略](https://learn.chatgpt.com/docs/config-file/config-advanced)，Claude Code 也有独立配置与权限。应检查这些规则，不应全局关闭过滤。已经打开的桌面 App 无法收到另一个终端的环境变化，本集成不会向现有进程注入配置。环境变量可被接收进程中的代码读取，并不隔离到单个技能。SkillHub 不自动向 Hub 或模型发送配置值，但下游工具仍可能在被要求打印或传输时暴露它们。

## 更新后的授权

更新技能后，在 App 检查当前配置要求并授权该摘要，或执行 `skillhub config renew TARGET DIGEST`。匹配的已有值和系统凭据库密钥可继续使用，移除的变量会清理，新必填变量需补充。不同来源、账号或安装不会继承授权。
