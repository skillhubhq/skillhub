import { createHash } from "node:crypto";

// This reads trusted static-export output; it is not an HTML sanitizer.
export function inlineScriptHashes(html) {
  return [...new Set(
    [...html.matchAll(/<script\b[^>]*>([\s\S]*?)<\/script\s*>/gi)]
      .filter((match) => match[1])
      .map((match) => `'sha256-${createHash("sha256").update(match[1]).digest("base64")}'`),
  )];
}
