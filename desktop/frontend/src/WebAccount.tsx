import { signInURL } from "./auth";
import { useEffect, useRef } from "react";
import { t } from "./i18n";
import { API, useUI } from "./ui";
import { Avatar, useResource } from "./community-ui";
import { isWeb } from "./platform";
import { Icon } from "./Icon";
export function WebAccount() {
  const ui = useUI();
  const menu = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    const dismiss = (e: PointerEvent) => {
      if (!menu.current?.contains(e.target as Node))
        menu.current?.removeAttribute("open");
    };
    document.addEventListener("pointerdown", dismiss);
    return () => document.removeEventListener("pointerdown", dismiss);
  }, []);
  const profile = useResource(
    () =>
      ui.activeSource
        ? API.CommunityGetProfile(ui.activeSource, "")
        : Promise.reject(new Error("No source")),
    [ui.activeSource, ui.revision],
  );
  function go(event: React.MouseEvent<HTMLButtonElement>, page: string) {
    event.currentTarget.closest("details")?.removeAttribute("open");
    ui.navigate(page);
  }
  return (
    <details
      ref={menu}
      className="account-menu"
      name="navigation-menu"
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          menu.current?.removeAttribute("open");
          menu.current?.querySelector("summary")?.focus();
        }
      }}
    >
      <summary
        aria-label={t("Account and preferences", "账号与偏好")}
        title={profile.value?.display_name || t("Account", "账号")}
      >
        {profile.value ? (
          <>
            <Avatar
              name={profile.value.display_name}
              image={profile.value.avatar}
            />
            <span className="account-name">{profile.value.display_name}</span>
          </>
        ) : (
          <>
            <Icon name="profile" />
            <span className="account-name">{t("Account", "账号")}</span>
          </>
        )}
        <span aria-hidden="true">⌄</span>
      </summary>
      <div className="account-popover">
        <p>
          {ui.sources.find((s) => s.alias === ui.activeSource)?.display_name ||
            t("Current community", "当前社区")}
          <small>
            {profile.value
              ? `@${profile.value.username}`
              : t("Not signed in", "尚未登录")}
          </small>
        </p>
        {profile.value && (
          <button onClick={(e) => go(e, "profile")}>{t("My profile")}</button>
        )}
        <button onClick={(e) => go(e, "settings")}>
          {t("Settings", "设置")}
        </button>
        {!isWeb && (
          <button onClick={(e) => go(e, "sources")}>{t("Connections")}</button>
        )}
        {isWeb && (
          <a
            href={profile.value ? "/logout" : signInURL()}
          >
            {profile.value ? t("Sign out") : t("Sign in")}
          </a>
        )}
      </div>
    </details>
  );
}
