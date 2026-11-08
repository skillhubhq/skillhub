import { readFile, writeFile, mkdir, copyFile, rm } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";

const root = fileURLToPath(new URL("../", import.meta.url));
const repo = path.dirname(root.replace(/\/$/, ""));
export const pages = [
  ["docs/README.md", "index", "Overview"],
  ["docs/usage.md", "usage", "Use skills"],
  ["docs/local-configuration.md", "local-configuration", "Local configuration and secrets"],
  ["docs/official-community.md", "official-community", "Official community"],
  ["docs/authoring.md", "authoring", "Create and test a skill"],
  ["docs/evaluations.md", "evaluations", "Evaluation evidence"],
  ["desktop/README.md", "desktop", "Desktop App"],
  ["docs/client-updates.md", "client-updates", "App updates"],
  ["docs/configuration.md", "configuration", "Configuration"],
  ["docs/enterprise-login.md", "enterprise-login", "Enterprise login"],
  ["docs/community-hosting.md", "community-hosting", "Community and login providers"],
  [
    "docs/identity-and-access.md",
    "identity-and-access",
    "Identity and permissions",
  ],
  ["docs/governance.md", "governance", "Publishing and review"],
  ["docs/database.md", "database", "Databases"],
  ["docs/operations.md", "operations", "Backup and operations"],
  [
    "docs/deployment-checklist.md",
    "deployment-checklist",
    "Deployment checklist",
  ],
  ["docs/product.md", "product", "Product scope"],
  ["docs/architecture.md", "architecture", "Architecture"],
  ["sdk/typescript/README.md", "sdk", "TypeScript SDK"],
  ["docs/testing.md", "testing", "Testing"],
  ["docs/distribution.md", "distribution", "Distribution"],
  ["docs/documentation.md", "documentation", "Maintain these docs"],
  ["CONTRIBUTING.md", "contributing", "Contributing"],
  ["SECURITY.md", "security", "Security"],
];
const slugs = new Map(
  pages.map(([src, slug]) => [src, slug === "index" ? "/" : `/${slug}/`]),
);
for (const [, slug] of pages) slugs.set(`docs/zh/${slug}.md`, slug === "index" ? "/" : `/${slug}/`);
export function resolveLink(src, target, locale = "en") {
  if (/^(?:[a-z][a-z\d+.-]*:|\/\/|#)/i.test(target)) return target;
  const match = target.match(/^([^#?]*)(.*)$/);
  const file = path.posix.normalize(
    path.posix.join(path.posix.dirname(src), match[1]),
  );
  if (slugs.has(file))
    return (locale === "zh" ? "/zh" : "") + slugs.get(file) + match[2];
  return `https://github.com/skillhubhq/skillhub/blob/main/${file}${match[2]}`;
}
export function rewriteLinks(src, markdown, locale = "en") {
  let fence;
  return markdown
    .split(/(?<=\n)/)
    .map((line) => {
      const marker = line.trimEnd().match(/^ {0,3}(`{3,}|~{3,})(.*)$/);
      if (fence) {
        if (
          marker &&
          marker[1][0] === fence[0] &&
          marker[1].length >= fence.length &&
          !marker[2].trim()
        )
          fence = undefined;
        return line;
      }
      if (marker) {
        fence = marker[1];
        return line;
      }
      if (/^(?: {4}|\t)/.test(line)) return line;
      // Preserve inline examples; rewrite only rendered Markdown links.
      return line.replace(
        /(`+)[\s\S]*?\1|\]\(([^\s)]+)\)/g,
        (match, code, target) =>
          code ? match : `](${resolveLink(src, target, locale)})`,
      );
    })
    .join("");
}
export async function prepare() {
  for (const locale of ["en", "zh"]) {
    const output = path.join(
      root,
      locale === "en" ? "content/docs" : "content/zh",
    );
    await rm(output, { recursive: true, force: true });
    await mkdir(output, { recursive: true });
    for (const [src, slug, title] of pages) {
      const input = locale === "en" ? src : `docs/zh/${slug}.md`;
      let body = await readFile(path.join(repo, input), "utf8");
      const translatedTitle = body.match(/^# ([^\n]+)/)?.[1];
      if (!translatedTitle) throw new Error(`Missing document title: ${input}`);
      body = body.replace(/^# [^\n]+\n+/, "");
      // 两种语言使用相同的规范文档相对链接，生成时转换为对应语言路由。
      body = rewriteLinks(input, body, locale);
      await writeFile(
        path.join(output, `${slug}.md`),
        `---\ntitle: ${JSON.stringify(locale === "en" ? title : translatedTitle)}\n---\n\n${body}`,
      );
    }
    const labels =
      locale === "en"
        ? ["Getting started", "Run a Hub", "Build and contribute"]
        : ["开始使用", "运行 Hub", "开发与贡献"];
    await writeFile(
      path.join(output, "meta.json"),
      JSON.stringify(
        {
          title: "SkillHub",
          pages: [
            "index",
            `---${labels[0]}---`,
            "usage",
            "official-community",
            "authoring",
            "evaluations",
            "desktop",
            "client-updates",
            `---${labels[1]}---`,
            "configuration",
            "enterprise-login",
            "community-hosting",
            "identity-and-access",
            "governance",
            "database",
            "operations",
            "deployment-checklist",
            `---${labels[2]}---`,
            "product",
            "architecture",
            "sdk",
            "testing",
            "distribution",
            "documentation",
            "contributing",
            "security",
          ],
        },
        null,
        2,
      ),
    );
  }
  await mkdir(path.join(root, "public"), { recursive: true });
  for (const [src, dest] of [
    ["mark-color.svg", "mark.svg"],
    ["favicon.svg", "favicon.svg"],
  ])
    await copyFile(
      path.join(repo, "desktop/frontend/public/assets/brand", src),
      path.join(root, "public", dest),
    );
}
if (process.argv[1] === fileURLToPath(import.meta.url)) await prepare();
