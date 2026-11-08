"use client";

import { useEffect, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { useTheme } from "next-themes";
import { Code } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import { createHubClients } from "../../sdk/typescript/src/index";
import { GitHubStar } from "../../desktop/frontend/src/GitHubStar";
import { HeaderFrame } from "../../desktop/frontend/src/HeaderFrame";
import { PreferenceControls } from "../../desktop/frontend/src/PreferenceControls";
import { Icon } from "../../desktop/frontend/src/Icon";

export function DocsHeader({ locale }: { locale: "en" | "zh" }) {
  const pathname = usePathname();
  const tr = (en: string, zh: string) => (locale === "zh" ? zh : en);
  useEffect(() => {
    document.documentElement.lang = locale === "zh" ? "zh-CN" : "en";
    try {
      localStorage.setItem("skillhub.language", locale);
      document.cookie = `skillhub-language=${locale}; Path=/; SameSite=Lax`;
    } catch {}
  }, [locale]);
  const [languageHref, setLanguageHref] = useState("/docs/");
  useEffect(() => {
    const update = () => {
      const path = location.pathname.replace(/^\/docs(?:\/zh)?(?=\/|$)/, "");
      setLanguageHref(
        `/docs${locale === "en" ? "/zh" : ""}${path || "/"}${location.hash}`,
      );
    };
    update();
    window.addEventListener("hashchange", update);
    return () => window.removeEventListener("hashchange", update);
  }, [locale, pathname]);
  const [site, setSite] = useState("SkillHub");
  const [profile, setProfile] = useState<{
    name: string;
    avatar: string;
  } | null>(null);
  const [identity, setIdentity] = useState<
    "checking" | "guest" | "member" | "unavailable"
  >("checking");
  const [failedAvatar, setFailedAvatar] = useState(false);
  const header = useRef<HTMLElement>(null);
  const account = useRef<HTMLDetailsElement>(null);
  const { theme, setTheme } = useTheme();
  useEffect(() => {
    const controller = new AbortController();
    const clients = createHubClients(
      createConnectTransport({
        baseUrl: location.origin,
        fetch: (input, init) =>
          fetch(input, { ...init, credentials: "same-origin" }),
      }),
    );
    const options = { signal: controller.signal };
    void clients.registry
      .getRegistry({}, options)
      .then((r) => {
        setSite(r.displayName || "SkillHub");
      })
      .catch(() => {});
    void clients.auth
      .getIdentity({}, options)
      .then(async () => {
        setIdentity("member");
        const { profile } = await clients.community
          .getProfile({}, options)
          .catch(() => ({ profile: undefined }));
        if (profile)
          setProfile({ name: profile.displayName, avatar: profile.avatar });
      })
      .catch((error) => {
        if (!controller.signal.aborted)
          setIdentity(
            error.code === Code.Unauthenticated ? "guest" : "unavailable",
          );
      });
    const dismiss = (event: PointerEvent) => {
      for (const menu of [account.current])
        if (menu && !menu.contains(event.target as Node)) menu.open = false;
    };
    document.addEventListener("pointerdown", dismiss);
    const observer = new ResizeObserver(() => {
      document.documentElement.style.setProperty(
        "--site-header-height",
        `${header.current?.offsetHeight || 72}px`,
      );
    });
    if (header.current) observer.observe(header.current);
    return () => {
      controller.abort();
      observer.disconnect();
      document.removeEventListener("pointerdown", dismiss);
    };
  }, []);
  const initials =
    profile?.name
      .trim()
      .split(/\s+/)
      .map((s) => [...s][0])
      .slice(0, 2)
      .join("")
      .toUpperCase() || "?";
  function closeOnEscape(event: React.KeyboardEvent<HTMLDetailsElement>) {
    if (event.key === "Escape") {
      event.currentTarget.open = false;
      event.currentTarget.querySelector("summary")?.focus();
    }
  }
  return (
    <header ref={header} className="navigation-shell docs-site-header">
      <HeaderFrame
        brand={
          <a
            className="brand home-link site-home"
            href="/"
            aria-label={`${site} · ${tr("Home", "首页")}`}
          >
            <img
              className="site-mark docs-brand-mark"
              src="/docs/mark.svg"
              alt=""
              width="30"
              height="33"
            />
            <span className="site-name">{site}</span>
          </a>
        }
        navigation={
          <nav
            className="primary-navigation"
            aria-label={tr("Main navigation", "主导航")}
          >
            <a href="/#discover">{tr("Community", "社区")}</a>
            <a href="/#submissions">{tr("Studio", "创作")}</a>
            <a href="/#downloads">{tr("Downloads", "下载")}</a>
            <a
              href={locale === "zh" ? "/docs/zh/" : "/docs/"}
              aria-current="page"
            >
              {tr("Docs", "文档")}
            </a>
          </nav>
        }
        utilities={
          <>
            <GitHubStar language={locale} />
            <PreferenceControls
              language={locale}
              theme={theme ?? "system"}
              onLanguage={(value) => {
                if (value !== locale) location.href = languageHref;
              }}
              onTheme={setTheme}
            />
            <a
              className="utility-settings"
              href="/#settings"
              aria-label={tr("Settings", "设置")}
              title={tr("Settings", "设置")}
            >
              <Icon name="management" />
            </a>
            <details
              ref={account}
              className="docs-account"
              name="docs-menu"
              onKeyDown={closeOnEscape}
            >
              <summary
                aria-label={tr("Account and preferences", "账号与偏好")}
                title={profile?.name || tr("Account", "账号")}
              >
                {profile ? (
                  <span className="docs-avatar">
                    {profile.avatar && !failedAvatar ? (
                      <img
                        src={profile.avatar}
                        alt=""
                        onError={() => setFailedAvatar(true)}
                      />
                    ) : (
                      initials
                    )}
                  </span>
                ) : (
                  <Icon name="profile" />
                )}
              </summary>
              <div className="docs-account-popover">
                <strong>
                  {profile?.name ||
                    (identity === "member"
                      ? tr("Your account", "你的账号")
                      : identity === "checking"
                        ? tr("Checking account…", "正在检查账号…")
                        : identity === "unavailable"
                          ? tr("Account unavailable", "账号暂不可用")
                          : tr("Not signed in", "尚未登录"))}
                </strong>
                <span>{site}</span>
                {identity === "member" && (
                  <a href="/#profile">{tr("My profile", "我的主页")}</a>
                )}
                <a href="/#settings">{tr("Settings", "设置")}</a>
                {identity === "guest" && (
                  <a
                    href={
                      "/login?next=" +
                      encodeURIComponent(
                        locale === "zh" ? "/docs/zh/" : "/docs/",
                      )
                    }
                  >
                    {tr("Sign in", "登录")}
                  </a>
                )}
                {identity === "member" && (
                  <a href="/logout">{tr("Sign out", "退出登录")}</a>
                )}
              </div>
            </details>
          </>
        }
      />
    </header>
  );
}
