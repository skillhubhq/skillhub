import type { Provenance as WireProvenance } from "../../../sdk/typescript/src/index";
import type { Record as Provenance } from "../bindings/github.com/skillhubhq/skillhub/internal/provenance/models";
export type { Provenance };
export const emptyProvenance = (): Provenance => ({
  author_name: "",
  author_url: "",
  source_url: "",
  revision: "",
  license: "",
  license_file: "LICENSE",
  modified: false,
  changes: "",
});
export const provenanceInfo = (p?: WireProvenance): Provenance | undefined =>
  p
    ? {
        author_name: p.authorName,
        author_url: p.authorUrl,
        source_url: p.sourceUrl,
        revision: p.revision,
        license: p.license,
        license_file: p.licenseFile,
        modified: p.modified,
        changes: p.changes,
      }
    : undefined;
export const provenanceWire = (p?: Provenance | null) =>
  p
    ? {
        authorName: p.author_name,
        authorUrl: p.author_url,
        sourceUrl: p.source_url,
        revision: p.revision,
        license: p.license,
        licenseFile: p.license_file,
        modified: p.modified,
        changes: p.changes,
      }
    : undefined;
// 外部 Hub 的声明也是不可信输入，链接渲染不接受脚本协议或内嵌凭据。
export function safeProvenanceURL(value: string) {
  try {
    const u = new URL(value);
    return u.protocol === "https:" &&
      !!u.hostname &&
      !u.username &&
      !u.password &&
      !u.search &&
      !u.hash &&
      value.length <= 2048 &&
      !/[\u0000-\u0020\u007f-\u009f]/.test(value)
      ? value
      : "";
  } catch {
    return "";
  }
}
export function validProvenance(p: Provenance, files: string[]) {
  return (
    !!p.author_name.trim() &&
    [...p.author_name].length <= 120 &&
    !!safeProvenanceURL(p.author_url) &&
    !!safeProvenanceURL(p.source_url) &&
    /^(?:[a-f0-9]{40}|[a-f0-9]{64})$/.test(p.revision) &&
    /^[A-Za-z0-9][A-Za-z0-9.+-]{0,79}$/.test(p.license) &&
    files.includes(p.license_file) &&
    (!p.modified || !!p.changes.trim()) &&
    [...p.changes].length <= 1000
  );
}
