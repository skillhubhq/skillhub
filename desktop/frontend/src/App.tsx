import { OpenSkillLink } from "./OpenSkillLink";
import { Downloads, ProjectFooter } from "./Downloads";
import { AccessState, needsSignIn } from "./AccessState";
import { useResource } from "./community-ui";
import { Community } from "./Community";
import { Navigation } from "./Navigation";
import { t, useLanguage } from "./i18n";
import { useCallback, useEffect, useState } from "react";
import type { ChangePlan } from "../bindings/github.com/skillhubhq/skillhub/desktop/models";
import type {
  ProjectRestoreResult,
  ReleasePreview,
  Source,
} from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
import { API, Modal, PreviewBody, UIContext, toolLabel, type UI } from "./ui";
import { Sources } from "./Sources";
import { Discover } from "./Discover";
import { Installed, Projects } from "./Local";
import { Configuration } from "./Configuration";
import { Management } from "./Management";
import { isWeb } from "./platform";
import { Settings } from "./Settings";
import { useNativeAppearance } from "./preferences";
const remotePages = new Set([
  "discover",
  "teams",
  "bookmarks",
  "profile",
  "publish",
  "submissions",
  "review",
  "management",
]);
const pages = [
  {
    id: "downloads",
    title: "Downloads",
    description: "",
  },
  {
    id: "settings",
    title: "Settings",
    description: "",
  },
  {
    id: "profile",
    title: "Profile",
    description: "A little about the person behind the work.",
  },
  {
    id: "discover",
    title: "Skill library",
    description: "Find the skills that fit your work.",
  },
  {
    id: "teams",
    title: "Teams",
    description: "Meet the people behind the skills.",
  },
  {
    id: "bookmarks",
    title: "Saved",
    description: "Your private collection, ready when you need it.",
  },
  ...(!isWeb
    ? [
        {
          id: "installed",
          title: "Installed",
          description: "Your skills, across tools and projects.",
        },
        {
          id: "projects",
          title: "Projects",
          description: "",
        },
        { id: "configuration", title: "Local configuration", description: "" },
      ]
    : []),
  {
    id: "publish",
    title: "Publish",
    description: "Share what works. Start with a skill package.",
  },
  {
    id: "submissions",
    title: "My releases",
    description: "Your releases and feedback, in one place.",
  },
  {
    id: "review",
    title: "Reviews",
    description: "A closer look before a skill reaches your team.",
  },
  ...(!isWeb
    ? [
        {
          id: "sources",
          title: "Connections",
          description: "",
        },
      ]
    : []),
  {
    id: "management",
    title: "Hub settings",
    description: "Manage spaces, categories, access and audit history.",
  },
];
function explain(error: unknown) {
  const text = error instanceof Error ? error.message : String(error);
  if (/community quota reached/i.test(text))
    return t(
      "A community limit has been reached. Check your submission usage or contact the Hub operator.",
      "已达到社区配额，请查看投稿用量或联系 Hub 部署方。",
    );
  if (/local modifications/.test(text))
    return t(
      "Local changes detected. Save or restore them before making a new plan.",
    );
  if (/operating system credential store unavailable/.test(text))
    return t("Unlock your system credential store and try again.");
  if (
    /unauthenticated|authentication required|credential|login required/.test(
      text,
    )
  )
    return isWeb
      ? t("Sign in to see this page.", "登录后查看此页面。")
      : t("Check your sign-in in Connections, then try again.");
  if (/permission[_ ]denied|forbidden/i.test(text))
    return t(
      "You do not have permission. Contact the hub or space administrator.",
    );
  if (/stale|changed after planning|preview expired/.test(text))
    return t("This record changed or the plan expired. Refresh and try again.");
  if (/no newer stable/.test(text))
    return t("You have the latest available stable version.");
  return text;
}
export default function App() {
  useLanguage();
  const appearanceError = useNativeAppearance();
  const [rememberedSource, setRememberedSource] = useState(() => {
    try {
      return localStorage.getItem("skillhub.community") ?? "";
    } catch {
      return "";
    }
  });
  const [route, setRoute] = useState(() =>
      pages.some((p) => p.id === location.hash.slice(1).split("?")[0])
        ? location.hash.slice(1)
        : "discover",
    ),
    [sources, setSources] = useState<Source[]>([]),
    [revision, setRevision] = useState(0),
    [pending, setPending] = useState(0),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  const page = route.split("?")[0];
  const routeSource = new URLSearchParams(route.split("?")[1] ?? "").get(
    "source",
  );
  const activeSource =
    routeSource ??
    (sources.some((s) => s.alias === rememberedSource)
      ? rememberedSource
      : (sources[0]?.alias ?? ""));
  const remotePage = remotePages.has(page);
  const session = useResource(
    async () =>
      activeSource
        ? { ...(await API.Identity(activeSource)), source: activeSource }
        : null,
    [activeSource, revision],
  );
  const [verifiedSource, setVerifiedSource] = useState("");
  useEffect(() => {
    if (!session.loading)
      setVerifiedSource(session.value?.id ? session.value.source : "");
  }, [session.loading, session.value]);
  const access = session.loading
    ? verifiedSource === activeSource && activeSource
      ? "member"
      : "checking"
    : session.value?.id && session.value.source === activeSource
      ? "member"
      : session.error && !needsSignIn(session.error)
        ? "unavailable"
        : "guest";
  const aggregateSearch =
    page === "discover" &&
    !isWeb &&
    new URLSearchParams(route.split("?")[1] ?? "").get("scope") === "all";
  const requiresSignIn =
    remotePage &&
    !aggregateSearch &&
    (!!sources.find((s) => s.alias === activeSource)?.private ||
      ["bookmarks", "publish", "submissions", "review", "management"].includes(
        page,
      ) ||
      (page === "profile" &&
        !new URLSearchParams(route.split("?")[1] ?? "").get("id")));
  const accessBlocked = requiresSignIn && access !== "member";
  useEffect(() => {
    if (remotePage && activeSource) {
      setRememberedSource(activeSource);
      try {
        localStorage.setItem("skillhub.community", activeSource);
      } catch {}
    }
  }, [activeSource, remotePage]);
  const [plan, setPlan] = useState<ChangePlan | null>(null),
    [outcome, setOutcome] = useState<ProjectRestoreResult[] | null>(null),
    [consumed, setConsumed] = useState(false),
    [preview, setPreview] = useState<ReleasePreview | null>(null);
  const [confirmation, setConfirmation] = useState<{
    title: string;
    detail: string;
    action: () => Promise<unknown>;
    tone: "primary" | "danger";
  } | null>(null);
  const run = useCallback(
    async <T,>(task: () => Promise<T>): Promise<T | undefined> => {
      setPending((n) => n + 1);
      setError("");
      try {
        return await task();
      } catch (e) {
        setError(explain(e));
        return undefined;
      } finally {
        setPending((n) => n - 1);
      }
    },
    [],
  );
  const changed = useCallback((text: string) => {
    setMessage(text);
    setRevision((n) => n + 1);
  }, []);
  useEffect(() => {
    const onHash = () => {
      const next = location.hash.slice(1);
      setRoute(
        pages.some((p) => p.id === next.split("?")[0]) ? next : "discover",
      );
      setError("");
      setMessage("");
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  useEffect(() => {
    void run(async () => setSources((await API.ListSources()) ?? []));
  }, [revision, run]);
  const closePlan = () => {
    if (plan) void API.DiscardPlan(plan.handle);
    setPlan(null);
    setOutcome(null);
    setConsumed(false);
  };
  const ui: UI = {
    busy: pending > 0,
    error,
    revision,
    sources,
    activeSource,
    access,
    run,
    changed,
    refresh: () => setRevision((value) => value + 1),
    navigate: (next) => {
      const [nextPage, query = ""] = next.split("?");
      if (remotePages.has(nextPage) && activeSource) {
        const params = new URLSearchParams(query);
        if (!params.has("source")) params.set("source", activeSource);
        next = `${nextPage}?${params}`;
      }
      setError("");
      setMessage("");
      setRoute(next);
      location.hash = next;
    },
    plan: (task) => {
      void run(async () => {
        const value = await task();
        setOutcome(null);
        setConsumed(false);
        setPlan(value);
      });
    },
    preview: (task) => {
      void run(async () => setPreview(await task()));
    },
    confirm: (title, detail, action, tone = "danger") =>
      setConfirmation({ title, detail, action, tone }),
  };
  const current = pages.find((p) => p.id === page)!;
  return (
    <UIContext.Provider value={ui}>
      <div
        className={`shell community-shell ${isWeb ? "web-shell" : "desktop-shell"}`}
      >
        {!isWeb && <OpenSkillLink />}
        <Navigation page={page} />
        <main className={`page-${page}`}>
          <header className="page-header" hidden={accessBlocked}>
            <div className={page === "discover" ? "discovery-page-label" : ""}>
              <h1>
                {page === "discover"
                  ? t("Explore skills", "发现技能")
                  : t(current.title)}
              </h1>
              {![
                "submissions",
                "review",
                "management",
                "installed",
                "downloads",
              ].includes(page) && (
                <p className="subtitle">
                  {page === "discover"
                    ? t(
                        "Browse workflows from people across your teams.",
                        "浏览团队成员分享的工作方法。",
                      )
                    : t(current.description)}
                </p>
              )}
            </div>
          </header>
          {appearanceError && (
            <p className="notice error" role="alert">
              {t(appearanceError, "窗口外观更新失败。")}
            </p>
          )}
          {error && (
            <div role="alert" className="notice error">
              {t(error)}
              <button
                aria-label={t("Dismiss error")}
                onClick={() => setError("")}
              >
                ×
              </button>
            </div>
          )}
          {message && (
            <div role="status" className="notice success">
              {t(message)}
              <button
                aria-label={t("Dismiss notification")}
                onClick={() => setMessage("")}
              >
                ×
              </button>
            </div>
          )}
          {ui.busy && (
            <div className="activity" role="status">
              {t("Working…")}
            </div>
          )}
          <section key={`${route}/${remotePage ? activeSource : "local"}`}>
            <fieldset>
              {accessBlocked ? (
                access === "checking" ? (
                  <p className="access-loading" role="status">
                    {t("Checking access…", "正在确认访问权限…")}
                  </p>
                ) : (
                  <AccessState
                    error={
                      access === "guest"
                        ? "authentication required"
                        : session.error
                    }
                    retry={session.reload}
                  />
                )
              ) : remotePage &&
                sources.length > 0 &&
                !sources.some((s) => s.alias === activeSource) ? (
                <div className="quiet-empty">
                  <h2>
                    {t("This connection is unavailable", "这个连接不可用")}
                  </h2>
                  <p>
                    {t(
                      "Choose another community or reconnect this hub in Settings.",
                      "请选择其他社区，或在设置中重新连接这个 Hub。",
                    )}
                  </p>
                  <button
                    onClick={() =>
                      ui.navigate(
                        isWeb
                          ? `discover?source=${encodeURIComponent(sources[0]?.alias ?? "hub")}`
                          : "sources",
                      )
                    }
                  >
                    {t("Connections")}
                  </button>
                </div>
              ) : page === "downloads" ? (
                <Downloads />
              ) : page === "settings" ? (
                <Settings />
              ) : ["teams", "bookmarks", "profile"].includes(page) ? (
                <Community mode={page} />
              ) : page === "sources" ? (
                <Sources />
              ) : page === "discover" ? (
                <Discover />
              ) : page === "installed" ? (
                <Installed />
              ) : page === "projects" ? (
                <Projects />
              ) : page === "configuration" ? (
                <Configuration />
              ) : (
                <Management mode={page} />
              )}
            </fieldset>
          </section>
          <ProjectFooter />
        </main>
      </div>
      {plan && (
        <Modal
          title={
            outcome?.every((r) => r.applied)
              ? t("Changes complete", "更改已完成")
              : t("Review installation")
          }
          onClose={closePlan}
          wide
        >
          {!outcome && (
            <p className="muted">
              {t("Check the version and destination. Plan expires at")}{" "}
              {new Date(plan.expires_at).toLocaleTimeString()}.
            </p>
          )}
          <div className="plan-list">
            {(plan.changes ?? []).map((p) => (
              <article key={p.target}>
                <div>
                  <span className="badge">
                    {p.action === "remove"
                      ? t("Uninstall")
                      : p.action === "update"
                        ? p.before?.version === p.after?.version &&
                          p.before?.digest === p.after?.digest
                          ? t("Reinstall")
                          : t("Update")
                        : t("Install")}
                  </span>
                  <h3>{p.after?.name ?? p.before?.name}</h3>
                  <p>
                    {toolLabel(p.after?.tool ?? p.before?.tool ?? "")} ·{" "}
                    {p.before?.version ?? t("Not installed")} →{" "}
                    {p.after?.version ?? t("Remove")}
                  </p>
                  <p className="path">{p.target}</p>
                </div>
              </article>
            ))}
          </div>
          {outcome && (
            <div className="notice">
              {outcome.map((r, i) => (
                <p key={i}>
                  {r.applied ? t("Completed") : t("Not completed")} · {r.target}
                  {r.error && ` — ${explain(r.error)}`}
                </p>
              ))}
            </div>
          )}
          {consumed && !outcome && (
            <p className="notice">
              {t("This plan has ended. Close it and create a new one.")}
            </p>
          )}
          <div className="actions">
            <button disabled={ui.busy} onClick={closePlan}>
              {consumed ? t("Close") : t("Cancel")}
            </button>
            {outcome?.length && outcome.every((r) => r.applied) ? (
              <button
                className="primary"
                onClick={() => {
                  closePlan();
                  ui.navigate("installed");
                }}
              >
                {t("Open my library", "查看本地技能")}
              </button>
            ) : null}
            {!consumed && (
              <button
                className="primary"
                disabled={ui.busy || (plan.changes ?? []).length === 0}
                onClick={() =>
                  void run(async () => {
                    setConsumed(true);
                    const results = await API.ApplyPlan(plan.handle);
                    if (!results)
                      throw new Error(
                        t(
                          "Installation result unavailable. Refresh before trying again.",
                        ),
                      );
                    setOutcome(results);
                    changed(
                      results.every((r) => r.applied)
                        ? t("Installation complete.")
                        : t(
                            "Some targets could not be updated. Check their results before retrying.",
                          ),
                    );
                  })
                }
              >
                {(plan.changes ?? []).every(
                  (change) => change.action === "install",
                )
                  ? t("Install now", "确认安装")
                  : t("Apply changes", "确认更改")}
              </button>
            )}
          </div>
        </Modal>
      )}
      {preview && (
        <Modal
          stable
          title={t("Package preview")}
          onClose={() => setPreview(null)}
          wide
        >
          <PreviewBody preview={preview} />
        </Modal>
      )}
      {confirmation && (
        <Modal title={confirmation.title} onClose={() => setConfirmation(null)}>
          <p>{confirmation.detail}</p>
          <div className="actions">
            <button disabled={ui.busy} onClick={() => setConfirmation(null)}>
              {t("Cancel")}
            </button>
            <button
              className={confirmation.tone}
              disabled={ui.busy}
              onClick={() =>
                void run(async () => {
                  await confirmation.action();
                  setConfirmation(null);
                  changed(t("Changes saved."));
                })
              }
            >
              {t("Confirm")}
            </button>
          </div>
        </Modal>
      )}
    </UIContext.Provider>
  );
}
