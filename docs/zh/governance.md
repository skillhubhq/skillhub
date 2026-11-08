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

## 团队归档与永久清理

团队是带展示资料的命名空间。**仅 Hub 管理员**可以预览、归档、恢复或永久清理团队；命名空间管理员和人员管理员没有此权限。Web 与 App 的入口一致：**创作 → Hub 配置 → 命名空间 → 团队归档与清理**，CLI 使用同一套服务端权限与状态检查。

| 操作 | 内容与权限如何处理 |
| --- | --- |
| 归档 | 团队及全部版本退出社区发现、直接下载、贡献列表、评测展示和审核待办。停止发布、审核、资料修改、授权修改及互动，旧客户端也无法绕过。内容、原版本状态和成员授权保留但不生效。 |
| 恢复 | 恢复保留的权限及版本状态。已撤回或被拒绝的版本不会重新发布；待审核版本重新进入待办，操作时再次检查权限。 |
| 永久清理 | 必须先归档，并提供当前修订号、再次输入完整命名空间。删除该团队全部版本、评论回复、点赞收藏、审核通知、扫描、评测证据、举报、团队资料、精选及授权。命名空间永久保留占位，不可再次使用。 |

共享的 Hub 用户、第三方登录身份、权限组、群组成员关系和审计记录**不会随团队删除**。已删除的个人空间不会在下次登录时自动重建。其他团队不受影响。用户本机已安装的技能文件不会被删除；归档或删除后无法再次下载、更新或复用下载缓存，本地检查会提示版本不可用，不会自动换成别的版本。

操作弹窗先展示影响数量。修订号可以阻止过期请求覆盖后续的归档、恢复或删除操作。CLI 示例：

```sh
skillhub admin namespace-inspect research --source company
skillhub admin namespace-archive research --source company --revision 1 --yes
skillhub admin namespace-restore research --source company --revision 2 --yes
# 永久清理前必须再次归档，并通过 inspect 获取最新修订号。
skillhub admin namespace-purge research --source company --revision 4 --confirm research --yes
```

永久清理无法在产品内撤销。删除业务内容前应备份；恢复必须由运维使用数据库与包的配套备份，并核对备份之后的写入。已有备份和安全审计遵循各自保留策略，不承诺存储介质上的彻底擦除。包含被删除版本正文、团队资料或评论的幂等响应缓存也会清理。最小的命名空间占位记录用于防止旧链接指向新团队。

数据库删除提交后，系统通过持久队列回收无引用包；其他团队仍使用的包保留。回收与发布共用写锁。文件清理失败时队列保留，可在修复存储权限后使用同一个 `--request-id` 重试，或重启 Hub 继续回收。回收失败不会撤销已经完成的团队删除，也不会阻止 Hub 启动。

写入型验收应优先在隔离 Hub 运行。经明确授权在正式实例创建的测试团队，也应通过这些产品接口归档、检查和清理；不要隐式删除账号或共享权限组，账号停用是独立的管理决定。旧 Hub 不声明 `namespace.lifecycle.v1` 时，新客户端隐藏入口或明确提示升级 Hub。
