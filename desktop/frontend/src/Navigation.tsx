import { noWorkspaceAccess } from "./workspaceAccess";
import { connectionName } from "./ConnectionPreferences";
import { OfficialBadge } from "./official";
import { GitHubStar } from "./GitHubStar";
import { HeaderFrame } from "./HeaderFrame";
import { ReviewInbox } from "./ReviewInbox";
import { Select } from "./Select";
import { PreferenceControls } from "./PreferenceControls";
import { useEffect, useLayoutEffect, useRef } from "react";
import { useUI } from "./ui";
import { isWeb } from "./platform";
import { Icon } from "./Icon";
import { WebAccount } from "./WebAccount";
import { t, setLanguage, useLanguage } from "./i18n";
import { setTheme, useTheme } from "./preferences";

const community = ["discover", "teams", "bookmarks", "profile"];
const studio = ["publish", "submissions", "review", "management"];
const library = ["installed", "projects", "configuration"];
export function Navigation({ page }: { page: string }) {
  const headerRef = useRef<HTMLElement>(null);
  useLayoutEffect(() => {
    const header = headerRef.current;
    const shell = header?.parentElement;
    if (!header || !shell) return;
    const measure = () =>
      shell.style.setProperty(
        "--navigation-height",
        `${header.getBoundingClientRect().height}px`,
      );
    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(header);
    return () => {
      observer.disconnect();
      shell.style.removeProperty("--navigation-height");
    };
  }, []);
  const language = useLanguage();
  const ui = useUI();
  const permissions = ui.workspaceAccess ?? noWorkspaceAccess;
  const studioPage = permissions.publish
    ? "submissions"
    : permissions.review
      ? "review"
      : "management";
  const privateGate =
    isWeb &&
    !!ui.sources.find((s) => s.alias === ui.activeSource)?.private &&
    ui.access !== "member";
  const group =
    page === "downloads"
      ? "downloads"
      : community.includes(page)
        ? "community"
        : studio.includes(page)
          ? "studio"
          : library.includes(page)
            ? "library"
            : "settings";
  const allLinks =
    group === "downloads"
      ? []
      : group === "community"
        ? [
            ["discover", t("Explore")],
            ["teams", t("Teams")],
            ["bookmarks", t("Saved")],
          ]
        : group === "studio"
          ? [
              ["submissions", t("My releases")],
              ["review", t("Reviews")],
              ["management", t("Hub settings")],
            ]
          : group === "library"
            ? [
                ["installed", t("Installed")],
                ["projects", t("Projects")],
                ["configuration", t("Configuration", "配置")],
              ]
            : [
                ["settings", t("Preferences", "偏好设置")],
                ...(!isWeb ? [["sources", t("Connections")]] : []),
              ];
  const links = allLinks.filter(([id]) =>
    id === "submissions"
      ? permissions.publish
      : id === "review"
        ? permissions.review
        : id === "management"
          ? permissions.manage
          : true,
  );
  const remote = group === "community" || group === "studio";
  const siteName =
    ui.sources.find((s) => s.alias === ui.activeSource)?.display_name ||
    "SkillHub";
  return (
    <header ref={headerRef} className="navigation-shell">
      <HeaderFrame
        brand={
          <button
            className={`brand home-link ${isWeb ? "site-home" : "app-home"}`}
            aria-label={
              isWeb
                ? `${siteName} · ${t("Home", "首页")}`
                : t("SkillHub home", "SkillHub 首页")
            }
            title={isWeb ? siteName : "SkillHub"}
            onClick={() => ui.navigate("discover")}
          >
            <img
              className={`${isWeb ? "site-mark" : "brand-logo"} brand-light`}
              src={`./assets/brand/${isWeb ? "mark" : "logo"}-color.svg`}
              alt=""
              width={isWeb ? 30 : 126}
              height="33"
            />
            <img
              className={`${isWeb ? "site-mark" : "brand-logo"} brand-dark`}
              src={`./assets/brand/${isWeb ? "mark" : "logo"}-white.svg`}
              alt=""
              width={isWeb ? 30 : 126}
              height="33"
            />
            {isWeb && <span className="site-name">{siteName}</span>}
          </button>
        }
        navigation={
          <nav
            className="primary-navigation"
            aria-label={t("Main navigation", "主导航")}
          >
            <button
              aria-current={group === "community" ? "page" : undefined}
              onClick={() => ui.navigate("discover")}
            >
              {t("Community", "社区")}
            </button>
            {!isWeb && (
              <button
                aria-current={group === "library" ? "page" : undefined}
                onClick={() => ui.navigate("installed")}
              >
                {t("My library", "本地技能")}
              </button>
            )}
            {(permissions.publish ||
              permissions.review ||
              permissions.manage) && (
              <button
                aria-current={group === "studio" ? "page" : undefined}
                onClick={() => ui.navigate(studioPage)}
              >
                {t("Studio", "创作")}
              </button>
            )}
            {isWeb && (
              <button
                aria-current={page === "downloads" ? "page" : undefined}
                onClick={() => ui.navigate("downloads")}
              >
                {t("Downloads", "下载")}
              </button>
            )}
            {isWeb && (
              <a
                className="docs-link"
                href={language === "zh" ? "/docs/zh/" : "/docs/"}
              >
                {t("Docs", "文档")}
              </a>
            )}
          </nav>
        }
        utilities={
          <>
            {isWeb && <GitHubStar language={language} />}
            <QuickPreferences />
            <button
              className="utility-settings"
              title={t("Settings", "设置")}
              aria-label={t("Settings", "设置")}
              aria-current={group === "settings" ? "page" : undefined}
              onClick={() => ui.navigate("settings")}
            >
              <Icon name="management" />
            </button>
            {ui.access === "member" && ui.activeSource && (
              <ReviewInbox
                key={`${ui.activeSource}/${ui.accountID}`}
                source={ui.activeSource}
              />
            )}
            <WebAccount />
          </>
        }
      />
      {links.length > 0 && !privateGate && (
        <div className="context-navigation">
          <nav aria-label={t("Section navigation", "区域导航")}>
            {links.map(([id, label]) => (
              <button
                key={id}
                aria-current={page === id ? "page" : undefined}
                onClick={() => ui.navigate(id)}
              >
                {label}
              </button>
            ))}
          </nav>
          <div className="context-actions">
            {remote && !isWeb && <HubContext page={page} />}
            {remote && permissions.publish && page !== "publish" && (
              <button
                className="publish-shortcut"
                aria-current={page === "publish" ? "page" : undefined}
                onClick={() => ui.navigate("publish")}
              >
                <Icon name="publish" />
                {t("Publish", "发布")}
              </button>
            )}
          </div>
        </div>
      )}
    </header>
  );
}
function QuickPreferences() {
  return (
    <PreferenceControls
      language={useLanguage()}
      theme={useTheme()}
      onLanguage={setLanguage}
      onTheme={setTheme}
    />
  );
}

function HubContext({ page }: { page: string }) {
  const ui = useUI(),
    menu = useRef<HTMLDetailsElement>(null);
  const current = ui.sources.find((s) => s.alias === ui.activeSource);
  useEffect(() => {
    const dismiss = (e: PointerEvent) => {
      if (!menu.current?.contains(e.target as Node))
        menu.current?.removeAttribute("open");
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, []);
  return (
    <details
      className="hub-context"
      ref={menu}
      name="navigation-menu"
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          menu.current?.removeAttribute("open");
          menu.current?.querySelector("summary")?.focus();
        }
      }}
    >
      <summary
        aria-label={t("Switch Hub", "切换 Hub")}
        title={
          current
            ? `${t("Switch Hub", "切换 Hub")} · ${current.display_name} · ${current.alias}`
            : t("Connect a hub", "连接 Hub")
        }
      >
        <span className="hub-switch-label">Hub</span>
        <span className="hub-switch-name">
          {connectionName(current) || t("Connect a hub", "连接 Hub")}
        </span>
        {current && <OfficialBadge url={current.url} />}
        <span className="menu-chevron" aria-hidden="true">
          ⌄
        </span>
      </summary>
      <div className="hub-popover">
        <p>{t("Hubs and accounts", "Hub 与账号")}</p>
        {ui.sources.map((s) => (
          <button
            key={s.alias}
            aria-pressed={s.alias === ui.activeSource}
            disabled={ui.busy}
            onClick={() => {
              menu.current?.removeAttribute("open");
              ui.navigate(`${page}?source=${encodeURIComponent(s.alias)}`);
            }}
          >
            <span>
              <strong>
                {connectionName(s)} <OfficialBadge url={s.url} />
              </strong>
              <small>
                {t("CLI ID", "CLI 标识")}: {s.alias}
              </small>
            </span>
            {s.alias === ui.activeSource && <Icon name="review" />}
          </button>
        ))}
        <div className="hub-menu-footer">
          <small>
            {t(
              "Each connection keeps its own account and access.",
              "每个连接保留独立的账号与权限。",
            )}
          </small>
          <button
            onClick={() => {
              menu.current?.removeAttribute("open");
              ui.navigate("sources");
            }}
          >
            <Icon name="publish" />
            {t("Manage connections", "管理连接")}
          </button>
        </div>
      </div>
    </details>
  );
}
