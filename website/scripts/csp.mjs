import { createHash } from "node:crypto";
import { parse } from "parse5";

// This reads trusted static-export output; it is not an HTML sanitizer.
export function inlineScriptHashes(html) {
  const hashes = new Set();
  function visit(node) {
    if (node.tagName === "script" && !node.attrs.some((attr) => attr.name === "src")) {
      const text = node.childNodes
        .filter((child) => child.nodeName === "#text")
        .map((child) => child.value)
        .join("");
      if (text) hashes.add(`'sha256-${createHash("sha256").update(text).digest("base64")}'`);
    }
    for (const child of node.childNodes ?? []) visit(child);
    if (node.content) visit(node.content);
  }
  visit(parse(html));
  return [...hashes];
}
