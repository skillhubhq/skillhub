import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { pages, prepare, resolveLink, rewriteLinks } from "./prepare.mjs";

test("canonical guide links keep route and anchor", () => {
  assert.equal(
    resolveLink("docs/README.md", "usage.md#install"),
    "/usage/#install",
  );
  assert.equal(
    resolveLink("docs/usage.md", "../desktop/README.md"),
    "/desktop/",
  );
  assert.equal(resolveLink("desktop/README.md", "../docs/README.md"), "/");
});
test("links in code examples remain literal", () => {
  const example =
    "`[Usage](usage.md)`\n```md\n[Usage](usage.md)\n```\n~~~md\n[Usage](usage.md)\n~~~\n    [Usage](usage.md)\n";
  assert.equal(
    rewriteLinks("docs/README.md", example + "[Usage](usage.md)\n"),
    example + "[Usage](/usage/)\n",
  );
});
test("repository resources remain on the public repository", () => {
  assert.equal(
    resolveLink("docs/testing.md", "../scripts/smoke.py"),
    "https://github.com/skillhubhq/skillhub/blob/main/scripts/smoke.py",
  );
  assert.equal(
    resolveLink("docs/usage.md", "https://example.com"),
    "https://example.com",
  );
  assert.equal(resolveLink("docs/usage.md", "#install"), "#install");
});

test("all published guides use English consistently", async () => {
  for (const [source] of pages) {
    const text = await readFile(
      new URL(`../../${source}`, import.meta.url),
      "utf8",
    );
    assert.doesNotMatch(text, /\p{Script=Han}/u, source);
  }
});

test("Chinese routes preserve corresponding guides and anchors", () => {
  assert.equal(
    resolveLink("docs/README.md", "evaluations.md#import-and-read", "zh"),
    "/zh/evaluations/#import-and-read",
  );
  assert.equal(
    resolveLink("desktop/README.md", "../docs/README.md", "zh"),
    "/zh/",
  );
  assert.equal(
    resolveLink(
      "docs/evaluations.md",
      "../schemas/evaluation-report.v1.json",
      "zh",
    ),
    "https://github.com/skillhubhq/skillhub/blob/main/schemas/evaluation-report.v1.json",
  );
});
test("every guide has a substantial Chinese counterpart", async () => {
  for (const [, slug] of pages) {
    const text = await readFile(
      new URL(`../../docs/zh/${slug}.md`, import.meta.url),
      "utf8",
    );
    assert.match(text, /^# .*\p{Script=Han}/u, slug);
    assert.ok((text.match(/\p{Script=Han}/gu) || []).length > 200, slug);
    assert.doesNotMatch(text, /TODO|translation pending|待翻译/i, slug);
  }
});

// 新增导航条目不能让后续中文标题按数组下标错配到其他页面。
test("generated Chinese navigation titles belong to their own documents", async () => {
  await prepare();
  for (const [, slug] of pages) {
    const source = await readFile(new URL(`../../docs/zh/${slug}.md`, import.meta.url), "utf8");
    const generated = await readFile(new URL(`../content/zh/${slug}.md`, import.meta.url), "utf8");
    assert.equal(JSON.parse(generated.match(/^title: (.+)$/m)[1]), source.match(/^# (.+)$/m)[1], slug);
    assert.doesNotMatch(generated, /^title: undefined$/m);
  }
});
