import { readFile, writeFile, readdir, stat } from "node:fs/promises";
import { createHash } from "node:crypto";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { pages as guides } from "./prepare.mjs";
const root = fileURLToPath(new URL("../out/", import.meta.url));
async function walk(dir) {
  const entries = await readdir(dir, { withFileTypes: true });
  return (
    await Promise.all(
      entries.map((e) =>
        e.isDirectory() ? walk(path.join(dir, e.name)) : path.join(dir, e.name),
      ),
    )
  ).flat();
}
const files = await walk(root);
const policies = {};
let pages = 0;
for (const file of files.filter((f) => f.endsWith(".html"))) {
  const html = await readFile(file, "utf8");
  const hashes = [...html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script>/g)]
    .filter((m) => m[1])
    .map(
      (m) => `'sha256-${createHash("sha256").update(m[1]).digest("base64")}'`,
    );
  const rel = path.relative(root, file).replaceAll(path.sep, "/");
  policies[rel] =
    `default-src 'self'; script-src 'self' ${[...new Set(hashes)].join(" ")}; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'`;
  for (const match of html.matchAll(/(?:href|src)="(\/docs\/[^"#?]*)/g)) {
    const target = decodeURIComponent(match[1].slice("/docs/".length));
    if (target.split("/").includes(".."))
      throw new Error(`Unsafe documentation link: ${target}`);
    const dest = path.join(root, target);
    const info = await stat(dest).catch(() => null);
    if (!info) throw new Error(`Broken link in ${rel}: ${target}`);
    if (info.isDirectory()) await stat(path.join(dest, "index.html"));
  }
  pages++;
}
await stat(path.join(root, "api/search"));
await stat(path.join(root, "api/search-zh"));
for (const locale of ["", "zh/"])
  for (const [, slug] of guides) {
    await stat(
      path.join(
        root,
        locale,
        slug === "index" ? "index.html" : `${slug}/index.html`,
      ),
    );
  }
await writeFile(path.join(root, "csp.json"), JSON.stringify(policies));
console.log(
  `Verified ${pages} static pages, local links and search index. Generated per-page script hashes.`,
);
