# 数据库驱动

Hub 通过 Go `database/sql` 支持默认 SQLite 和可选 PostgreSQL。CLI/App 本地状态仍为 SQLite。切换 Hub 数据库不改变本地包存储或客户端行为；MySQL 等未支持，新增驱动需方言、初始化和行为测试。

## 配置 [#configure]

SQLite 无需连接密钥。PostgreSQL 使用专用空数据库/模式及能创建表、索引、序列的服务角色，包目录仍需独立持久化。服务保持**单 Hub 进程**，不是多副本或高可用方案。

初始化前获取 PostgreSQL 会话级 advisory lease，所有查询绑定该物理连接。丢失所有权后拒绝请求并关闭 HTTP 服务，不通过连接池重连绕过锁。数据库操作串行化，优先保证单节点正确性；替换版本必须先停旧进程。

```yaml
data: /var/lib/skillhub/data
public_url: https://skills.example.com
private: true
database:
  driver: postgres
  dsn_file: /run/secrets/skillhub-database
```

密钥文件包含 pgx 连接串，如 `postgres://USER:PASSWORD@DB_HOST/skillhub?sslmode=verify-full`，按数据库要求配置 CA/客户端证书。也可省略文件并注入 `SKILLHUB_DATABASE_DSN`；二者互斥，上限 8 KiB。密码不能进入 YAML、Git 或命令行参数，错误不会回显 DSN。

`check-config`、`init --password-stdin`、`serve` 使用同一配置。空数据库在一个事务中创建完整结构；已有数据库必须使用受支持的 schema 版本。配置校验不联网。切驱动不会自动迁数据，尚无 SQLite→PostgreSQL 转换；新建实例或另行设计并验证导入方案。数据库和包必须配套备份。

## PostgreSQL 备份、升级和恢复 [#postgresql-backup-upgrade-and-recovery]

PATH 中需同时有与服务器**相同主版本**的 `pg_dump`/`pg_restore`，CI 覆盖 15、16、18。维护 DSN 必须是 `postgres://` 或 `postgresql://` URL，不接受关键字或 libpq service 别名。恢复目标必须同 PostgreSQL 主版本、同 schema 名；跨主版本升级和跨驱动转换不在支持范围。

使用专用 schema，备份拒绝无关表/视图。整个维护期间停 Hub、禁用自动重启并阻止外部写入，工具需持有目录锁和数据库所有权锁。这不能协调任意 DBA 的并发写入。先建备份父目录，目标目录必须不存在。

```sh
skillhub-server backup --config /etc/skillhub/hub.yaml \
  --output /srv/backups/skillhub-before-upgrade --timeout 10m
skillhub-server verify-backup --database-driver postgres \
  --input /srv/backups/skillhub-before-upgrade --timeout 10m
```

备份包含 `hub.dump`、所引用的不可变 `packages/` 和 `backup.json`，记录实例 ID、Hub schema、PostgreSQL 主版本、大小/摘要和有序包摘要集合。校验检查清单、归档结构、每个包的摘要和格式，但不联网，不能代替恢复演练。密钥、配置、二进制不包含在内，应单独安全保存；备份含私有身份和内容，需加密、限权、独立存储并校验复制品。校验和只能发现损坏，不能证明来源可信。

### 替换服务程序 [#replace-the-server-binary]

停 Hub，用当前程序创建并校验备份，保留程序和配置。确认新程序支持数据库版本后再替换。当前 Hub 只有版本 1 的完整初始结构，启动会拒绝不支持的版本，不修改已有表；不提供结构转换命令。恢复流量前验证就绪、登录、权限和包下载。

### 恢复与回退 [#restore--rollback]

准备**新的空数据库（同 schema 名）和新数据目录**，通过单独受控配置指向目标，不复用在服 DSN。拒绝非空 schema 或已有目录，不覆盖原实例。

```sh
skillhub-server restore --config /etc/skillhub/recovery.yaml \
  --input /srv/backups/skillhub-before-upgrade --timeout 10m
```

回到快照状态时，用 `restore` 恢复至新目标，再选择匹配程序和恢复配置。恢复前核对快照后的数据、密码与权限变化。

恢复在单事务内导入并比对实际实例/schema/包引用，清除全部会话和待定设备授权，记录 `instance.restore`。账号、待审批申请、权限和包保留，禁用/拒绝状态不解除。目录与数据库双重恢复标记阻止不完整实例启动，包括换目录指向同数据库的情况。失败目标保留以便诊断，重试必须新建目标，**不可删除标记强行启动**。

重新开放前检查新登录、旧令牌拒绝、非管理员受众隔离、审核、下载/安装和实例 ID，记录恢复耗时。不可让源和恢复实例同时服务同一 origin。备份保留、异机复制、磁盘/失败告警及责任人单独配置。

## 行为测试 [#behavior-tests]

`SKILLHUB_TEST_POSTGRES_DSN` 只能指向可丢弃 PostgreSQL，账号需能创建/删除测试数据库。HTTP 测试建隔离 schema，维护测试建隔离数据库。

```sh
go test -race -count=1 ./internal/hubhttp
go test -count=1 -run TestPostgres ./internal/hub
```

本地安装匹配主版本工具；CI 在 PostgreSQL 15/16/18 容器内运行维护工具。覆盖租约先于 DDL、断连、配套恢复、失效会话、损坏/缺包、错误身份、不完整恢复阻断、初始化事务失败与未来 schema 拒绝。普通 `make check` 未指定 DSN 时跳过 PostgreSQL，不可据此宣称其已验证。参见[测试](testing.md)与[部署检查](deployment-checklist.md)。
