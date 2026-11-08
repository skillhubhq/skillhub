import test from "node:test";
import assert from "node:assert/strict";
import { documentLink, headingAnchor } from "../src/documentLinks.ts";
test("package links resolve only within the archive", () => {
  assert.deepEqual(documentLink("../setup.md#tokens", "guides/use.md"), {
    kind: "package",
    path: "setup.md",
    anchor: "tokens",
  });
  assert.deepEqual(documentLink("#Setup", "SKILL.md"), {
    kind: "package",
    path: "SKILL.md",
    anchor: "Setup",
  });
  for (const href of [
    "../../outside",
    "%2e%2e/private",
    "file:///etc/passwd",
    "javascript:alert(1)",
    "//example.org",
    "https://user:pass@example.org",
    "foo%5c..%5cprivate",
  ])
    assert.equal(documentLink(href), null);
  assert.deepEqual(documentLink("https://example.org/docs"), {
    kind: "external",
    href: "https://example.org/docs",
  });
  assert.equal(headingAnchor("Setup & Tokens"), "setup--tokens");
});
