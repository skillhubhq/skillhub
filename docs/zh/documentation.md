# 维护文档

Hub 同源提供英文 `/docs/` 和简体中文 `/docs/zh/`。文档是公开的软件资料，即使私有 Hub 也可读，不包含账号、包、真实配置或运行数据。

## 编写 [#authoring]

英文规范来源位于 `docs/`、`desktop/README.md`、`sdk/typescript/README.md`、贡献及安全策略；中文对应页在 `docs/zh/`，按网站 slug 命名。修改行为时同步更新两种语言，不能用英文正文冒充中文页，不编辑生成的 `website/content/`。

Next.js、Fumadocs、MDX、Tailwind 生成静态 HTML 和各语言本地搜索索引。复用主站头部/品牌、设备主题和语言偏好，通过 Connect 在浏览器中读取站点/账号，账号不会嵌入导出。英文为默认，两种语言有相同目录；切换保留页面，对应标题锚点存在时保留章节。

中文源码链接以文件自身位置为基准，在 GitHub 也能直接访问；构建映射到 `/zh/` 下对应路由。中文标题可加 `[#english-heading-id]` 保留稳定章节锚点；命令、字段、协议标识保持原样，解释和导航使用中文。

```sh
npm --prefix sdk/typescript ci --ignore-scripts
npm --prefix website ci --ignore-scripts
npm --prefix website run dev
```

预览 `http://localhost:3001/docs/` 或 `/docs/zh/`。构建内嵌文档：

```sh
make web
go build -o bin/skillhub-server ./cmd/skillhub-server
```

`website/scripts/prepare.mjs` 定义源文件、顺序和双语标题。新增指南需添加英文源、中文对应页与目录项。构建检查页面、链接和搜索资产；测试检查成对内容和语言一致性。仓库非文档文件链接指向公开 GitHub。

## 托管 [#hosting]

默认与 Hub 同源，保留 `/docs/` 路由。独立官网可把 `website/out/` 静态内容部署到 `/docs/`，保留 trailing-slash 目录索引，将 `/docs/_next/` 映射导出资源。Node 只在构建时需要，构建无需认证或秘密变量。

不要把私有公司运维手册放入文档源，具体部署配置、备份、凭据和验收证据留在公开仓库之外。
