export type DocumentLink =
  | { kind: "external"; href: string }
  | { kind: "package"; path: string; anchor: string };

// 包内路径相对于当前文档解析，不能离开归档根目录。
export function documentLink(
  href: string,
  current = "SKILL.md",
): DocumentLink | null {
  if (/^https?:\/\//i.test(href)) {
    try {
      const u = new URL(href);
      return u.username || u.password
        ? null
        : { kind: "external", href: u.href };
    } catch {
      return null;
    }
  }
  if (/^[a-z][a-z0-9+.-]*:|^\/|[\\\x00-\x1f]/i.test(href)) return null;
  try {
    const [raw, fragment = ""] = href.split("#", 2);
    const decoded = decodeURIComponent(raw);
    if (/[\\\x00-\x1f?:]/.test(decoded) || decoded.startsWith("/")) return null;
    const parts = raw ? current.split("/").slice(0, -1) : current.split("/");
    if (raw)
      for (const p of decoded.split("/")) {
        if (p === "..") {
          if (!parts.length) return null;
          parts.pop();
        } else if (p && p !== ".") parts.push(p);
      }
    return parts.length
      ? {
          kind: "package",
          path: parts.join("/"),
          anchor: decodeURIComponent(fragment),
        }
      : null;
  } catch {
    return null;
  }
}
export function headingAnchor(value: string) {
  return value
    .toLowerCase()
    .replace(/[^\p{L}\p{N}_\-\s]/gu, "")
    .trim()
    .replace(/\s/g, "-");
}
