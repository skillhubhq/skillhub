# SkillHub 文档

[项目概览](../../README.md) · [参与贡献](contributing.md)

SkillHub 用于分享、审核和安装技能，不执行 Agent。文档提供英文和简体中文，右上角可切换语言；示例中的域名、账号和输入均为虚构内容。

## 使用技能 [#use-skills]

| 指南 | 内容 |
| --- | --- |
| [Hub 与命令行](usage.md) | 连接、登录、发现、安装和恢复项目 |
| [创建与测试技能](authoring.md) | 编写指令、本地校验和外部效果测试 |
| [评测证据](evaluations.md) | 用例规范、外部结果导入和版本证据 |
| [桌面 App](desktop.md) | 原生应用与本地工作流 |
| [App 更新](client-updates.md) | 检查版本并安全替换应用 |

## 运行社区 [#run-a-community]

| 指南 | 内容 |
| --- | --- |
| [配置](configuration.md) | 站点名称、访问策略、YAML 与密钥来源 |
| [企业登录](enterprise-login.md) | Microsoft Entra ID / OIDC 与准入审批 |
| [身份与权限](identity-and-access.md) | 账号、命名空间角色、分组与平台职责 |
| [发布与审核](governance.md) | 安全扫描、独立审核、待办与通知 |
| [数据库](database.md) | SQLite / PostgreSQL 与维护命令 |
| [备份与运维](operations.md) | 备份、恢复、升级、回退与账号恢复 |
| [部署检查](deployment-checklist.md) | 真实环境和身份提供方的验收 |

## 开发与集成 [#build-and-integrate]

| 指南 | 内容 |
| --- | --- |
| [产品边界](product.md) | 支持的行为与限制 |
| [架构](architecture.md) | Hub、协议、共享内核和存储 |
| [TypeScript SDK](sdk.md) | 生成的客户端与 RPC 示例 |
| [测试](testing.md) | 使用隔离数据复现检查 |
| [维护文档](documentation.md) | 中英文内容、预览和构建 |
| [分发](distribution.md) | 安装包、校验和与发布流程 |

安全问题通过[私密渠道](security.md)报告，不要提交到公开 Issue。凭据、真实部署地址、私有社区截图和测试运行报告不属于源码贡献。

- [社区部署与登录适配器](community-hosting.md)
