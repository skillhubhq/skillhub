import test from "node:test";
import assert from "node:assert/strict";
import { zipSync, unzipSync } from "fflate";
import { repackDraft } from "../src/draft-archive.ts";

test("editing a document preserves executable and ordinary file permissions", () => {
  const text = new TextEncoder();
  const original = zipSync({
    "SKILL.md": [text.encode("original"), { os: 3, attrs: 0o100644 << 16 }],
    "scripts/check.sh": [
      text.encode("#!/bin/sh\nexit 0\n"),
      { os: 3, attrs: 0o100755 << 16 },
    ],
  });
  const files = unzipSync(original);
  files["SKILL.md"] = text.encode("edited");
  const result = repackDraft(original, files);
  assert.equal(
    new TextDecoder().decode(unzipSync(result)["SKILL.md"]),
    "edited",
  );
  assert.deepEqual(
    unzipSync(result)["scripts/check.sh"],
    files["scripts/check.sh"],
  );
  const view = new DataView(result.buffer);
  let entry = view.getUint32(result.length - 6, true);
  assert.equal(result[entry + 5], 3);
  assert.equal(view.getUint32(entry + 38, true) >>> 16, 0o100644);
  entry +=
    46 +
    view.getUint16(entry + 28, true) +
    view.getUint16(entry + 30, true) +
    view.getUint16(entry + 32, true);
  assert.equal(view.getUint32(entry + 38, true) >>> 16, 0o100755);
});

test("unmatched files and unsupported archives fail instead of losing attributes", () => {
  const original = zipSync({ "SKILL.md": new Uint8Array([65]) });
  assert.throws(() =>
    repackDraft(original, { "new.txt": new Uint8Array([66]) }),
  );
  assert.throws(() => repackDraft(new Uint8Array(0), {}));
  const invalid = original.slice();
  new DataView(invalid.buffer).setUint16(invalid.length - 12, 0xffff, true);
  assert.throws(() => repackDraft(invalid, {}));
});
