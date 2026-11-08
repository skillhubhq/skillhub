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

保存后不会自动将值提供给 Codex、Claude Code 等进程。请通过工具支持的启动方式或外部凭据工具单独配置运行环境。环境变量引用检查的是运行 SkillHub 的进程环境：从 Finder 启动的 App 与终端环境可能不同。修改启动环境后，应重启已有进程。

SkillHub 不执行密码库 CLI 命令。外部密码管理器可继续通过其标准环境变量工作流使用。向整个 Agent 提供变量，不代表该值被隔离到单个技能。同一操作系统用户的进程可能拥有相同的凭据库访问能力，因此这不是对已失陷用户账号的防护。

配置绑定 Hub 身份、来源地址、安装、项目和工具以及精确版本与摘要。升级后需要清理旧配置、检查当前声明再重新保存。本地已修改文件不能重新定义可信配置要求。卸载后保留配置，用户可在配置列表中主动清理；配置不会自动转移给新安装。

客户端备份包含 `local-configuration/` 中的非敏感配置和不透明绑定，不包含系统凭据库中的密钥值。密钥恢复依赖操作系统自己的备份机制。元数据与系统凭据更新若中断，诊断会报告缺失或不可用，可重试或清理保留条目。覆盖 App 时保留客户端数据目录的原位置；在不同设备或路径恢复元数据不会自动复制、重新绑定密钥。
