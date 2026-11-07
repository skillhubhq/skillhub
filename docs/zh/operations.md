# 单节点 Hub 运维

本页主要介绍 SQLite 快照。PostgreSQL 使用[数据库指南](database.md)中的数据库/包配套维护命令。YAML、参数与密钥注入参见[配置](configuration.md)。

## 部署与数据归属 [#deployment-and-data-ownership]

以专用 OS 账号运行一个 Hub，数据库 WAL 和包目录保存在同机可写磁盘。不支持网络共享文件系统或多副本。已有反向代理负责 HTTPS，不应把回环 HTTP 默认监听直接暴露公网。

```sh
skillhub-server serve --data /srv/skillhub/data \
  --listen 127.0.0.1:8080 --public-url https://skills.example.com \
  --name "Company Skills" --private=true --trusted-proxies 127.0.0.1/32
```

按[使用指南](usage.md)初始化一次。保持 origin 稳定，客户端绑定规范 URL 与不可变实例 ID。`--private=false` 仅让已发布 Public 版本匿名可读，发布和管理仍需认证。

目录包含 `hub.db`、运行时 WAL/SHM 和 `packages/*.zip`。账号、权限、分类、版本、审核、社区、评测、幂等、审计和实例密钥在数据库中；启动参数、TLS、代理和服务管理器配置需单独备份。备份含私有包和认证材料，保护强度不能低于在线目录。

## 在线备份 [#online-backup]

使用与运行中 Hub 同版本的程序。父目录需存在，输出目录必须新建且位于在线目录外。

```sh
skillhub-server backup --data /srv/skillhub/data \
  --output /srv/skillhub-backups/before-upgrade --timeout 10m
skillhub-server verify-backup --input /srv/skillhub-backups/before-upgrade
```

SQLite `VACUUM INTO` 生成一致快照，再复制快照引用的不可变包（含待审核、拒绝和撤下版本），重复摘要仅存一次；备份期间可继续发布。当前不会垃圾回收被引用包，未来 GC 必须与此协调。

`backup.json` 记录格式/schema、实例 ID、时间、数据库大小/摘要和包数。校验只读检查校验和、SQLite 完整性、schema/身份、外键及每个包格式/摘要，发现活动 sidecar、未完成标记、未知清单或损坏缺失包即失败，不通过重放 WAL 修改备份。

完整目录可复制到独立存储并再次校验；校验和不是签名，只恢复可信来源。预留数据库快照及全部压缩包空间，逐个有界解包验证。超时/取消非零退出。工具不安排定时、不删除旧备份；由运维制定保留策略并定期恢复演练。

## 恢复与切换 [#restore-and-cutover]

使用支持快照 schema 的程序恢复到**新目录**，不修改源备份、不合并或覆盖已有实例。

```sh
skillhub-server verify-backup --input /srv/skillhub-backups/before-upgrade
skillhub-server restore --input /srv/skillhub-backups/before-upgrade \
  --data /srv/skillhub/recovered-before-upgrade --timeout 10m
```

保留实例身份、账号密码、治理、包和历史审计/幂等数据；移除全部会话和待定设备授权，新增恢复审计。客户端需重登。快照后的密码、禁用、权限和内容变化不在恢复结果中，重新开放前必须核对。

1. 校验并恢复，保留当前目录用于回退。
2. 停旧进程，不能同时服务同一 origin。
3. 服务配置指向恢复目录，保持公开地址、访问模式和代理设置。
4. 启动后检查 `/healthz`、重新登录、实例 ID、权限、审核记录和精确版本下载。
5. 补齐快照后治理变化，再开放访问，按策略保留原目录和备份。

隔离演练用其他回环端口、临时客户端目录和别名，不重绑现有客户端。未完成目标带 `.skillhub-maintenance`，禁止启动；断电遗留时保留诊断后换新目录重试，不删标记强行启动。完成的备份目录也不可直接作为在线目录，必须先恢复。

## 升级与回退 [#upgrade-and-rollback]

升级前用旧程序备份并校验；停机，保留旧程序/配置，再用新程序打开在线目录。迁移事务执行并保留实例身份，拒绝未知未来 schema。检查登录、权限和下载。

回退 schema 变化时停新服务，用兼容程序从升级前备份恢复到新目录，不让旧程序直接打开新 schema。核对备份后已接受的写入再确定恢复点。Hub schema 为 10，客户端为 2；SQLite 清单格式 1 支持校验 Hub schema 2–10。恢复保持原 schema，下一次打开才执行受支持迁移，校验过程不偷偷迁移快照。

## 诊断与限制 [#diagnostics-and-limits]

- `/healthz` 仅证明进程存活，不代表权限、容量和包完整性；需认证目录/下载探测及备份校验。
- Hub 向 stderr 输出结构化诊断和请求 ID，不启用代理正文/Authorization 日志。日志不替代治理审计。
- HTTP：16 KiB 请求头、10 秒读头、60 秒读写/空闲超时、32 MiB 总请求体；RPC 另限制身份/注册表 8 KiB、目录/管理 64 KiB、发布 32 MiB、评测导入 400 KiB（含编码开销）。
- 包：20 MiB ZIP、80 MiB 总展开、8 MiB/文件、2048 项、256 KiB 主技能 Markdown；有并发门禁和认证限流。
- 默认忽略转发地址，只配置直接可信代理 CIDR；代理必须覆盖 `X-Forwarded-For`。Hub 自右向左找首个非可信地址，错误链回退到直连端。不要信任 `0.0.0.0/0` 或整个员工网段。
- 源故障检查 HTTPS、实例 ID、登录、凭据服务、命名空间/受众；诊断不导出令牌或包正文。
- 安装失败保留本地修改，不手动删事务日志绕过恢复。

## 账号生命周期与应急恢复 [#account-lifecycle-and-emergency-recovery]

Web/App“设置 → 账号安全”修改本地密码，需要当前密码。Hub 管理员在人员管理重置其他本地用户密码、授予/撤销管理员身份（含已有 SSO 用户）；修改密码或管理身份撤销该用户会话/设备授权。最后一个有效管理员不能降级，企业密码仍由 IdP 管理。不发送重置邮件或自动过期密码。

```sh
skillhub password-change --source work --passwords-stdin
skillhub admin user-password-reset USER_ID --source work --password-stdin
skillhub admin user-administrator USER_ID --source work --enabled=true
```

改密从 stdin 读取当前/新密码两行，重置读取一行；使用密码管理器，不进入 shell 参数或历史。变更后重登。

若丢失管理员凭据，先停服务并备份，再以服务 OS 账号运行：

```sh
skillhub-server recover-admin --data /var/lib/skillhub/data \
  --username EXISTING_LOCAL_USERNAME --password-stdin
```

仅恢复已有本地账号：启用、授予管理员、设置密码、撤销会话并记录审计；拒绝不存在或 SSO 账号。排他目录锁阻止与服务同时运行。数据目录访问本身是管理员权限，应严格限制。

## 候选构建与目标验收 [#candidate-preparation-and-internal-server-acceptance]

`deploy/skillhub.service` 和 `deploy/nginx.conf.example` 是模板，不是已安装服务。创建专用账号、受控程序目录和可写数据目录，初始化一次，填写真实域名/TLS/租户后才启用。秘密由受控环境/文件提供，不放入检入的 unit。

```sh
python3 scripts/build_candidate.py --output .local/candidate-build --arch amd64
```

干净授权 revision 的构建记录 SHA、源码哈希、工具链、目标和程序摘要；`--allow-dirty` 仅生成本地诊断产物，不能当候选版。App 在支持宿主构建并使用同 revision。校验复制品，保留旧二进制和备份。

真实部署另行记录：服务重启、域名/代理/TLS/存储；真实 Entra 分配、MFA、来宾、设备登录及撤权；非管理员发布审核安装全流程；定时备份、独立副本、恢复耗时和回退。代码测试、交叉编译或健康检查不能替代目标验收；这些环境检查也不应冒充代码发布阻塞。完整清单见[部署检查](deployment-checklist.md)。
