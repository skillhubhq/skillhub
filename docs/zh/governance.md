# 发布与审核

Hub 负责发布检查、独立审核、审计和平台职责。Web/App/CLI 使用同一服务端权限边界，无需队列或邮件服务。

## 包检查与批准 [#package-checks-and-approval]

每次提交先校验归档，再用有界离线 `builtin-static/2` 扫描。不会执行文件或发送内容给第三方。报告绑定包摘要，包含引擎版本、时间、规则、文件/行和固定修复提示，不复制命中的秘密原文。

- 私钥头和部分凭据特征阻止批准；即使管理员也不能覆盖，需修复后提交新版本。
- 远程 shell 管道、本地进程执行或终止、HTML 外部脚本、破坏性命令、指令覆盖语言，以及未检查的二进制内容要求独立审核，即使命名空间允许直接发布。
- 警告或不完整扫描需审核者写说明，仍禁止自审；缺失、过时或摘要不符时需重扫。
- 无发现只表示内置规则未命中，不能证明安全；这不是杀毒、沙箱或执行授权系统。
- 最多 100 条发现，溢出阻止批准。历史已发布包不会自动撤下；扫描失败/取消不能生成成功报告。
- 扫描报告仅向有管理/审核权限的人开放，无匿名端点；扫描使用昂贵操作并发门禁和地址限流。

Web/App 在提交正文上方显示可展开摘要；扫描与必要说明齐备后才可批准。规则标识和固定技术发现为英文，界面支持中英文。行为[评测证据](evaluations.md)独立于静态扫描，不自动批准版本。

## 审核通知 [#review-notifications]

审核决定与提交者通知在同一事务提交，幂等重试不重复通知。收件箱属于选定连接和当前用户，分页列出批准/要求修改结果，打开后标记已读并跳到确切版本。

读取和已读操作都会复核权限，撤权后旧通知不可读，不在收件箱复制技能正文。通知随重启和备份保留。另有实时查询的审核待办、数量和最早日期，排除自己的提交，在读取、窗口聚焦和可见时定期刷新。它是拉取列表，不是保证送达的推送；没有邮件、Webhook 或桌面推送。

## 平台职责 [#platform-duties]

| 职责 | 允许 | 不允许 |
| --- | --- | --- |
| Hub 管理员 | 全局管理、职责授予/撤销、密码重置 | 自审、绕过扫描阻断 |
| 人员管理员 | 列账号、创建普通账号、审批企业准入、启停普通账号 | 授予平台/Hub 角色、分组授权、命名空间管理、密码重置、修改自身或特权平台账号 |
| 审计员 | 只读审计 | 修改账号/授权、额外技能内容访问 |

平台职责不授予命名空间访问。仅 Hub 管理员可分配，变更会撤销受影响用户的会话/设备授权，未知角色拒绝。分组由 Hub 管理员管理，命名空间管理限定在各自范围；无通用自定义角色构建器或 Entra 分组自动映射。

企业身份仍按 issuer/subject 和默认准入审批处理。人员管理员不能通过批准入站授予发布审核权限，也不能通过重置密码接管账号。微软端离职不即时撤销 Hub 会话，需同时禁用 Hub 用户。

## CLI [#cli]

```sh
skillhub governance review-tasks --source work
skillhub governance notifications --source work
skillhub review list --source work
skillhub review inspect RELEASE_ID --source work
skillhub review approve RELEASE_ID --source work --reason "Reviewed fixed package"
skillhub review reject RELEASE_ID --source work --reason "Missing verification steps"
```

用 `skillhub governance --help` 查看扫描、已读和平台职责命令；修改需要相应权限，自动化使用 `--json` 和稳定请求键。复现验收见[测试](testing.md)。

## 上游署名 [#upstream-attribution]

收录版本区分三种身份：

| 身份 | 含义 |
| --- | --- |
| 原作者 | 有来源依据的名称和 HTTPS 主页；不会创建 Hub 账号，也不表示作者认可此次收录 |
| 收录维护者 | 发布命名空间及其团队主页；命名空间授权决定谁能维护这份内容 |
| 操作账号 | 真实认证的提交或审核账号，保留在审计记录中；外部作者名称不会获得权限 |

Web 和 App 统一展示来源目录、完整上游 Git commit、SPDX 许可证标识、包内许可证路径、修改声明和收录版本。卡片署名原作者，团队和本地技能页保留相同归属。版本来源面板可查看收录审核说明。这些是维护者提交并经过审核的声明，不是上游身份认证标识。

发布时保留上游目录中的文件、许可证和版权声明。无需为了 Hub 署名改写 `SKILL.md`：通过发布表单或 `--provenance FILE` 单独提交元数据。CLI 只接受明确指定的来源文件，不自动信任包内声明。

```json
{
  "author_name": "Example Authors",
  "author_url": "https://example.org/authors",
  "source_url": "https://example.org/source/0123456789abcdef0123456789abcdef01234567/skill",
  "revision": "0123456789abcdef0123456789abcdef01234567",
  "license": "Apache-2.0",
  "license_file": "LICENSE.txt",
  "modified": false,
  "changes": ""
}
```

```sh
skillhub publish ./example-skill --source community --namespace curators \
  --version 0.0.0-upstream.0123456789ab --visibility public \
  --provenance ./attribution.json
```

Hub 要求包内存在对应的文本许可证。审核者仍需核实许可证适用性、来源归属、精确提交和声明保留情况；Hub 不联网抓取或独立核验上游仓库。修改过文件时必须设定 `modified: true` 并填写改动说明。所有带来源声明的提交均须独立审核，即使命名空间允许直接发布。声明随版本固定并参与幂等校验，纠正声明需提交新版本。

上游没有发布版本号时，使用明确的收录快照版本，例如 `0.0.0-upstream.<commit 前缀>`，不要虚构上游 `1.0.0`。安装记录保存署名及包摘要，离线也能查看。向不支持 `catalog.provenance.v1` 的旧 Hub 发布来源声明会明确失败，避免静默丢失。
