import { defineConfig, defineDocs } from "fumadocs-mdx/config";

export const docs = defineDocs({ dir: "content/docs" });
export const docsZh = defineDocs({ dir: "content/zh" });
export default defineConfig({ mdxOptions: { format: "md" } });
