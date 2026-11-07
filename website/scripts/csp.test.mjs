import test from "node:test";
import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { inlineScriptHashes } from "./csp.mjs";

test("script hashes preserve exact content across HTML tag casing", () => {
  const source = "\nwindow.example = 'MiXeD';\n";
  const expected = `'sha256-${createHash("sha256").update(source).digest("base64")}'`;
  for (const tag of ["script", "SCRIPT", "ScRiPt"]) {
    assert.deepEqual(
      inlineScriptHashes(`<${tag} nonce="example">${source}</${tag} >`),
      [expected],
    );
  }
});

test("empty external scripts add no hashes and repeated content is deduplicated", () => {
  assert.deepEqual(inlineScriptHashes('<SCRIPT src="/bundle.js"></SCRIPT>'), []);
  assert.equal(inlineScriptHashes('<script>run()</script><SCRIPT>run()</SCRIPT>').length, 1);
  assert.deepEqual(inlineScriptHashes('<scripture>text</scripture>'), []);
});

test("HTML parser handles quoted delimiters, end-tag attributes and line ending normalization", () => {
  const expected = inlineScriptHashes('<script>run()\n</script>');
  assert.deepEqual(inlineScriptHashes('<SCRIPT data-example=">">run()\r\n</script\t\n bar>'), expected);
  assert.deepEqual(inlineScriptHashes('<!-- <script>run()</script> -->'), []);
  assert.deepEqual(inlineScriptHashes('<script src="/bundle.js">unused()</script>'), []);
});
