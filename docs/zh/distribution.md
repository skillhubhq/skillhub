# 软件分发

GitHub 是源码、Issue 和版本下载的官方入口。各 Hub 只链接软件项目，不镜像安装器，也不向 GitHub 发送社区凭据、搜索或技能内容。下载页/安装指引使用无 referrer 链接，App 用系统浏览器打开。

## 渠道与平台 [#channels-and-platforms]

使用 [Releases](https://github.com/skillhubhq/skillhub/releases)，包括预发布，不把 `/releases/latest` 当作只有预发布时的默认入口。私有仓库下载需 GitHub 权限。

矩阵为 Linux amd64 Hub/CLI、macOS arm64 Hub/CLI/App、Windows amd64 Hub/CLI/App。暂不分发 Linux App 或 Intel macOS 包。macOS 需 13+，Windows App 需 WebView2。桌面安装包未提供正式签名、公证或自动安装。

版本包含 Hub/CLI 的 tar.gz/zip、App ZIP、macOS DMG、`SHA256SUMS`、`PROVENANCE.json`（revision、CI、目标及摘要），归档内含许可证和依赖清单。

Linux 用 `sha256sum --ignore-missing -c SHA256SUMS` 并确认目标文件 OK；macOS 用 `shasum -a 256 ARCHIVE` 对比完整摘要；Windows 用 `Get-FileHash ARCHIVE -Algorithm SHA256`。校验和不独立证明发行者，若有 attestation，再用 `gh attestation verify ARCHIVE --repo skillhubhq/skillhub`。

App 设置手动检查固定 GitHub Releases API，可筛稳定/预览并打开精确版本页。请求不带 Hub 凭据，不替换程序；网络/权限/平台缺包不显示已最新。最多查 100 条，满页视为无法确认完整性。见[更新](client-updates.md)。

## 手动安装 macOS [#manual-macos-installation]

DMG 包含 App、Applications 快捷方式、许可证与双语说明。退出旧 App，拖入替换，弹出映像再打开。连接、凭据、草稿、偏好和技能位于 bundle 外，应保留。降级遵循兼容/迁移说明。

ad-hoc 签名不是 Developer ID/公证。DMG 不绕过 Gatekeeper 或设备管理策略；核对来源和摘要，遵守正常系统安全流程，不关闭全局保护。本机启动成功不证明浏览器下载后的其他设备可运行。

```sh
node desktop/build.mjs
python3 scripts/collect_licenses.py --desktop --output desktop/bin/third-party
python3 scripts/build_dmg.py --output desktop/bin/SkillHub-manual-preview.dmg
```

仅本地生成，不发布；输出附 `.sha256`、`.json` 记录构建/签名限制，不覆盖已有文件。脏工作区标记为本地产物，安装包不包含 Hub/客户端数据。分发检查拒绝非 `skillhubhq/skillhub` 的旧项目引用，第三方依赖链接保持原状。

## 准备发布 [#preparing-a-release]

1. 提交完整候选含生成代码，前端 package 与 lock 的版本一致；CI 嵌入 App 版本。
2. 确切 main revision 的完整 CI 通过协议、并发/流程/原生、安全、源码卫生和许可证检查。
3. 手动运行 Prepare release，指定版本及成功 CI run ID；验证来源/分支/revision/版本，仅打包该 run 产物，拒绝已有 tag/release，只创建草稿。
4. 人工检查摘要、来源、原生报告与平台说明。真实租户/服务器/设备环境验收单列，不冒充代码验收或静默阻塞它。
5. 若可用，启用 GitHub 不可变发布，完善草稿后由维护者发布，不能替换既有版本二进制。

公开仓库准备流程生成 GitHub attestations；私有可用性取决于计划，不能虚称存在。账号可见性和签名凭据不由本地代码决定。

## 维护 [#maintenance]

下载页独立于社区登录，多 Hub App 软件更新与技能更新分开。服务器升级先备份并保留旧二进制/配置；schema 回退用兼容快照。将来需要离线分发，可通过自有部署系统镜像未修改的归档、摘要和来源证明，但不能让私有镜像成为开源产品依赖。

## 发布安全证据 [#security-evidence-for-release-preparation]

准备草稿要求同一 main revision 的 CI，以及 Go、JavaScript/TypeScript、Python、Actions 完整 CodeQL 分析且无打开告警。错误、警告、证据不全或 API 失败停止准备。关闭误报也需逐条数据流依据，不能把扫描任务成功等同安全认证。查询最多最近 100 份分析，旧候选可能需重扫；发布前复核新披露依赖漏洞。
