import site from "../../../internal/product/site.json";
import { t } from "./i18n";
export const officialHubURL = site.officialHub;
export function isOfficialHub(address: string) {
  try {
    const url = new URL(address);
    return (
      url.origin === officialHubURL &&
      !url.username &&
      !url.password &&
      url.pathname === "/" &&
      !url.search &&
      !url.hash
    );
  } catch {
    return false;
  }
}
export function OfficialBadge({ url }: { url: string }) {
  return isOfficialHub(url) ? (
    <span
      className="official-badge"
      title={t("Operated by the SkillHub project", "由 SkillHub 项目运营")}
    >
      ✓ {t("Official", "官方")}
    </span>
  ) : null;
}
export function skillWebURL(
  hub: string,
  namespace: string,
  name: string,
  version: string,
) {
  return `${hub.replace(/\/$/, "")}/#discover?${new URLSearchParams({ skill: `${namespace}/${name}`, version, tab: "readme" })}`;
}
export function skillAppURL(hub: string, coordinate: string) {
  const index = coordinate.lastIndexOf("@");
  return `skillhub://open?${new URLSearchParams({ hub, skill: coordinate.slice(0, index), version: coordinate.slice(index + 1) })}`;
}
