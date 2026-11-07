# 评测证据

SkillHub 将外部评测结果关联到确切的技能版本。Hub 不执行 Agent、不调用模型、不抓取报告网址，也不验证报告声称的观察结果。证据属于**外部自报信息**，不是认证、热度分数或自动审核依据；静态安全扫描是另一项检查。

## 编写用例 [#write-cases]

从[用例示例](../../examples/evaluations/cases.json)和[用例规范](../../schemas/evaluation-cases.v1.json)开始。`skillhub.cases.v1` 用例集包含稳定 `name`、用例集 `version` 和 1–50 条用例，每条有唯一 `id`、`input` 与 `expected`。覆盖典型任务、信息缺失、无关请求和不可信指令，使用合成输入。

```sh
skillhub eval validate ./cases.json --cases
```

## 在 Hub 外执行 [#run-outside-the-hub]

在隔离工作区选择 Agent 或外部评测系统，并明确授权工具使用。记录 Agent/模型/执行器版本、配置、时间和实际观察。必要时重复执行，并用相同输入比较启用和不启用技能的结果。SkillHub 不指定执行器，也不保证不同模型或配置下能复现同一结果。

使用[报告示例](../../examples/evaluations/report.json)和[报告规范](../../schemas/evaluation-report.v1.json)，将示例全零的 `skill_digest` 替换为发布或版本检查返回的精确归档摘要。这是 ZIP 摘要，不是 `SKILL.md` 的摘要。报告内嵌完整用例集，每条用例必须恰好对应一条结果。`status` 为 `pass`、`fail`、`error` 或 `skipped`；即使错误或跳过也必须填写 `observed`。可选 `baseline` 记录不使用技能的结果。

```sh
skillhub eval validate ./report.json --json
```

校验命令离线运行，不初始化账号或凭据；校验的是格式，不是结果真实性。JSON Schema 供编辑器提示，CLI/Hub 还会检查 UTF-8 字节长度、唯一 ID、完整结果覆盖、合法文本、重复 JSON 键和最多 16 层深度。未知字段会被拒绝。文档上限 256 KiB，单条输入/观察 8 KiB、预期 4 KiB、配置 2 KiB、Agent/模型/执行器标签各 160 字节。执行时间必须是 2020 年或以后的 RFC3339 时间，导入时不能超过当前时间五分钟。规范化后的文档也必须符合 256 KiB 上限。

## 导入与阅读 [#import-and-read]

```sh
skillhub eval import RELEASE_ID ./report.json --source work
skillhub eval list RELEASE_ID --source work --json
skillhub eval show EVIDENCE_ID --source work --json
```

Web/App 在技能版本的“评测”标签中展示证据；维护者选择 JSON、检查预览并确认公开范围后导入。发布/审核详情使用同一可展开面板。报告展示输入、预期、实际结果、可选对照结果、时间、执行配置与摘要。“通过数/总数”的总数包含错误和跳过；对照分母仅包含填写了 baseline 的用例，不能把不同分母直接当作效果提升百分比。

命名空间维护者和管理员可导入、撤回；审核者只能阅读，除非另外获得维护权限。已发布证据沿用该版本受众，公开 Hub 的 Public 版本也允许匿名阅读。待审核、被拒绝、撤下的证据仅向相关管理/审核角色开放。Reader 不绕过分组受众；每次读取和变更重试都会复核当前权限。

导入要求归档摘要完全一致。Hub 计算用例集和规范化报告的 SHA-256，检查部分凭据/私钥特征，并限制昂贵请求并发。扫描不能识别全部秘密，上传前必须删除私有提示词、个人信息、凭据和内部资源地址。报告存在 Hub 数据库中并随数据库备份，不进入软件安装包。

相同版本的相同报告去重；`--request-id` 用于安全重试。每个版本最多 100 份不同报告，含已撤回记录。报告不可原地修改，修正后另行导入。旧 Hub 未声明 `evaluation.v1` 时，客户端明确提示不支持。

## 审核与撤回 [#review-and-retract]

证据不改变发布状态、不解除扫描阻断，也不允许自我审核。审核者需判断用例与观察是否相关。审核后仍可新增或撤回证据，因此它独立于不可变技能包，**不是被冻结的已审核附件集合**。证据不影响搜索排序或精选。

```sh
skillhub eval retract EVIDENCE_ID --source work --yes
```

Web/App 同样要求明确确认。撤回从活动记录中移除正文和衍生描述，保留 ID、版本关联、摘要、导入时间和审计。重复导入不会复活已撤回报告。已有备份和此前下载的副本不受影响，应按保留策略处理；撤回不等于存储介质安全擦除。
