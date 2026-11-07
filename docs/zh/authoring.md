# 创建与测试技能

技能是包含 `SKILL.md` 及可选辅助文件的目录。用编辑器或编程 Agent 编写内容；SkillHub 负责校验、发布、审核和安装，不调用模型，也不运行技能。

## 创建起点 [#create-a-starting-point]

```sh
skillhub init ./api-review --description "Use when reviewing API compatibility and error handling."
```

命令创建新目录和有效的 `SKILL.md`，不会覆盖已有目录、文件或符号链接。目录名作为稳定标识，也可通过 `--name` 指定；使用小写字母、数字和连字符。

在编辑器或编程 Agent 中打开目录，可以这样提出需求：

> 完善 SKILL.md，用于审核 API 变更。先询问缺失的约束，明确触发条件、操作步骤、预期输出和边界。只修改此目录；未经我指示，不要发布、执行脚本或使用私有数据。

可使用 Agent 自带的技能编写能力；SkillHub 不安装 Agent，也不修改模型设置。App 中的“创作 → 发布 → 创建技能”使用相同模板打开本地草稿。

## 发布前校验 [#validate-before-publishing]

```sh
skillhub validate ./api-review
skillhub validate ./api-review --json
```

校验离线运行，不初始化客户端账号数据库或读取凭据。它应用与发布相同的元数据、路径、归档、文件和大小规则，并运行内置静态扫描；不会执行脚本或向模型发送内容。

无效包和阻断级发现会返回非零退出码。警告与未检查的二进制文件交由人工审核。校验通过不能证明技能安全或有效；提交时 Hub 仍独立检查，不能绕过审核。

## 在 Agent 中测试行为 [#test-behavior-in-an-agent]

在隔离工作区中使用有代表性的非敏感输入，明确授权测试所需的工具操作。

| 用例 | 检查重点 |
| --- | --- |
| 典型任务 | 根据提供的事实生成要求的输出 |
| 缺少信息 | 询问必要信息而非编造 |
| 无关请求 | 不错误触发或接管任务 |
| 不可信输入 | 嵌入指令不能扩大用户授权 |
| 本地修改 | 保留无关文件，破坏性操作先征求许可 |

对相同任务比较使用与不使用技能的结果，记录技能版本或包摘要、输入、Agent/模型版本、配置、预期与实际输出。对不稳定任务重复执行，再下结论。点赞、下载量和静态扫描均不是效果分数。

外部执行完成后，通过 `skillhub eval` 或 Web/App 的“评测”面板导入脱敏结果，参见[评测证据](evaluations.md)。不要在包或报告中包含真实私有提示词、凭据或个人数据。

## 发布与改进 [#publish-and-improve]

```sh
skillhub source add work https://skills.example.com
skillhub login work
skillhub publish ./api-review --source work --namespace team --version 1.0.0
skillhub governance notifications --source work
```

选择有发布权限的命名空间。提交成功可能仍处于待审核；根据反馈修改后，重新校验并提交新版本，不能修改已提交版本的内容。

```sh
skillhub install work/team/api-review@1.0.0 --tool codex --dry-run
skillhub install work/team/api-review@1.0.0 --tool codex --yes
```

审核通过后先装入测试项目，再让目标 Agent 用技能完成具体任务。安装仅提供文件，发现、调用和执行由 Agent 负责。参见[命令行工作流](usage.md)和[发布审核](governance.md)。
