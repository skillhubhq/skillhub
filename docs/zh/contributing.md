# 参与贡献

欢迎围绕明确用户问题贡献，小修复可直接提 PR。讨论保持尊重、具体，避免一次混入无关改动。

## 本地环境 [#local-setup]

Go 版本见 `go.mod`/`desktop/go.mod`，Node.js 24，Buf 版本由 CI 固定。无须依赖其他内部仓库。

```sh
make web
make check
make quality
make desktop
```

使用临时 Hub 和合成包开发，不能指向真实公司仓库。参考[文档](index.md)与项目 README。

## 提交 PR 前 [#before-opening-a-pull-request]

- 运行 `make check`、`make web`、`make quality`；变更原生/共享客户端时运行桌面 Go 测试和原生构建。
- 修改协议后重新生成，提交生成代码，再在该 checkout 运行 `make proto-check` 检查漂移。
- Go 保持 gofmt，前端用固定版本 Prettier，CLI 保持清晰人类输出和稳定 `--json` 契约。
- 保留英文默认/中文界面、键盘操作和双主题；指南同步双语。
- 权限、发布、文件系统、协议变更需有意义回归，PR 说明行为、测试和平台限制。
- 不提交凭据、客户数据、私有社区截图、本机路径、内部部署配置或本地 skill。

## 工程边界 [#engineering-boundaries]

阅读 AGENTS.md、产品和架构。Proto 拥有 RPC 契约，客户端共享 Go 内核，Hub 授权权威；不执行包、不静默覆盖用户文件，保持单节点轻量。标识符/错误用英文，非显然 Go/Proto 注释遵循中文约定，生成文件不能手改。

## PR 检查 [#pull-request-checks]

main 要求 PR 与协议、安全、质量、Go、PostgreSQL、桌面检查通过，解决评审讨论并更新分支。使用 squash merge 和 Conventional Commit 标题，保护主干免直接/强制推送和删除，管理员同样遵守。依赖更新使用相同检查，不自动合并。

## 提交说明 [#commit-messages]

使用 `type(scope): description`，简短英文祈使句描述行为，scope 可为 hub、cli、desktop、web、auth、storage、docs。

```text
feat(cli): add grouped command help
fix(auth): reject expired login attempts
docs(readme): clarify self-hosting setup
```

类型包括 feat、fix、refactor、docs、test、build、ci、chore。必要时正文写动机/兼容性，不用工作过程日记作为提交标题。

## 文档 [#documentation]

只描述当前支持行为和限制，可复现命令放测试指南，运行证据放 PR/CI。开发日记、临时计划、私有部署报告、废弃方案不进入分发源码。

## 许可与安全 [#license-and-security]

贡献按 Apache-2.0 接受，第三方材料附来源/许可证，不导入未授权专有内容。漏洞按安全策略私密报告，公开 Issue 只放非敏感缺陷。
