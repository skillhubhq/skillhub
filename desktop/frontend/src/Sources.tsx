import { needsSignIn } from "./AccessState";
import { t } from "./i18n";
import { useEffect, useState } from "react";
import type {
  IdentityView,
  LoginView,
} from "../bindings/github.com/skillhubhq/skillhub/desktop/models";
import { API, Empty, Field, Modal, useUI } from "./ui";
import type { CacheInfo } from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
export function Sources() {
  const ui = useUI();
  const [adding, setAdding] = useState(false),
    [managing, setManaging] = useState("");
  const [alias, setAlias] = useState(""),
    [url, setURL] = useState(""),
    [account, setAccount] = useState(""),
    [username, setUsername] = useState(""),
    [password, setPassword] = useState("");
  const [identities, setIdentities] = useState<Record<string, IdentityView>>(
      {},
    ),
    [device, setDevice] = useState<{ source: string; view: LoginView } | null>(
      null,
    );
  const [identityState, setIdentityState] = useState<Record<string, string>>(
    {},
  );
  const sourceKey = ui.sources.map((s) => s.alias).join("/");
  useEffect(() => {
    let active = true;
    setIdentities({});
    setIdentityState({});
    for (const source of ui.sources)
      void API.Identity(source.alias)
        .then((identity) => {
          if (active) {
            setIdentities((old) => ({ ...old, [source.alias]: identity }));
            setIdentityState((old) => ({
              ...old,
              [source.alias]: "signed-in",
            }));
          }
        })
        .catch((error) => {
          if (active)
            setIdentityState((old) => ({
              ...old,
              [source.alias]: needsSignIn(String(error))
                ? "signed-out"
                : "unavailable",
            }));
        });
    return () => {
      active = false;
    };
  }, [ui.revision, sourceKey]);
  useEffect(() => {
    if (!device) return;
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const result = await API.PollLogin(device.view.handle);
        if (stopped) return;
        if (result.pending) {
          timer = setTimeout(poll, device.view.interval_seconds * 1000);
        } else {
          setIdentities((old) => ({
            ...old,
            [device.source]: result.identity,
          }));
          setDevice(null);
          ui.changed(
            `${t("Signed in to", "已登录")} ${device.source}: ${result.identity.username}`,
          );
        }
      } catch (error) {
        if (!stopped) {
          setDevice(null);
          void ui.run(async () => {
            throw error;
          });
        }
      }
    };
    timer = setTimeout(poll, device.view.interval_seconds * 1000);
    return () => {
      stopped = true;
      clearTimeout(timer);
      void API.CancelLogin(device.view.handle);
    };
  }, [device]);
  return (
    <>
      <div className="section-heading connection-toolbar">
        <p className="muted">
          {t(
            "Each connection keeps its own account and access.",
            "每个连接独立保存账号与访问权限。",
          )}
        </p>
        <button className="primary" onClick={() => setAdding(true)}>
          {t("Connect a hub")}
        </button>
      </div>
      {adding && (
        <Modal
          title={t("Connect a hub")}
          onClose={() => {
            setAdding(false);
            setAlias("");
            setURL("");
          }}
        >
          <form
            className="stack"
            onSubmit={(e) => {
              e.preventDefault();
              void ui.run(async () => {
                await API.AddSource(alias, url);
                setAlias("");
                setURL("");
                setAdding(false);
                ui.changed(t("Hub connected."));
              });
            }}
          >
            <Field
              label={t("Local alias")}
              hint={t("Use lowercase letters, numbers and hyphens")}
            >
              <input
                required
                value={alias}
                placeholder="company"
                onChange={(e) => setAlias(e.target.value)}
              />
            </Field>
            <Field label={t("Hub address")}>
              <input
                type="url"
                required
                value={url}
                placeholder="https://skills.example.com"
                onChange={(e) => setURL(e.target.value)}
              />
            </Field>
            <div className="actions full">
              <button className="primary" disabled={ui.busy}>
                {t("Connect hub")}
              </button>
            </div>
          </form>
        </Modal>
      )}
      {ui.sources.length === 0 ? (
        <Empty title={t("No connected hubs")}>
          {t("Add your company hub or a public hub to explore skills.")}
        </Empty>
      ) : (
        <div className="connection-list">
          {ui.sources.map((source) => (
            <article className="source-card" key={source.alias}>
              <div className="connection-identity">
                <h2>{source.display_name}</h2>
                <p className="connection-address" title={source.url}>
                  {source.url}
                </p>
                <span className="connection-alias">{source.alias}</span>
              </div>
              <div className="connection-account">
                <span className="badge">
                  {source.private
                    ? t("Private community", "私有社区")
                    : t("Public community", "公开社区")}
                </span>
                <p>
                  {identities[source.alias]
                    ? identities[source.alias].username
                    : !identityState[source.alias]
                      ? t("Checking account…", "正在确认账号…")
                      : identityState[source.alias] === "signed-out"
                        ? t("Not signed in")
                        : t("Account not verified", "账号暂未验证")}
                </p>
                {identities[source.alias]?.administrator && (
                  <small>{t("Hub admin", "Hub 管理员")}</small>
                )}
              </div>
              <div className="connection-card-actions">
                <button
                  onClick={() =>
                    ui.navigate(
                      `discover?source=${encodeURIComponent(source.alias)}`,
                    )
                  }
                >
                  {t("Browse community", "浏览社区")}
                </button>
                <button
                  className="quiet"
                  onClick={() => setManaging(source.alias)}
                >
                  {t("Manage connection", "管理连接")}
                </button>
              </div>
              {managing === source.alias && (
                <Modal
                  title={source.display_name}
                  onClose={() => setManaging("")}
                >
                  <p className="connection-address">
                    {source.alias} · {source.url}
                  </p>
                  <details className="connection-technical">
                    <summary>{t("Hub identity")}</summary>
                    <code className="path">{source.registry_id}</code>
                  </details>
                  <div className="connection-actions">
                    <button
                      disabled={ui.busy}
                      onClick={() =>
                        void ui.run(async () => {
                          const view = await API.BeginLogin(source.alias);
                          setManaging("");
                          setDevice({ source: source.alias, view });
                        })
                      }
                    >
                      {t("Sign in with browser")}
                    </button>
                    <button
                      disabled={ui.busy}
                      onClick={() => {
                        setManaging("");
                        setAccount(source.alias);
                        setUsername("");
                        setPassword("");
                      }}
                    >
                      {t("Sign in with password")}
                    </button>
                    <button
                      disabled={ui.busy}
                      onClick={() =>
                        void ui.run(async () => {
                          const identity = await API.Identity(source.alias);
                          setIdentities((old) => ({
                            ...old,
                            [source.alias]: identity,
                          }));
                        })
                      }
                    >
                      {t("Check identity")}
                    </button>
                    <button
                      disabled={ui.busy}
                      onClick={() =>
                        ui.confirm(
                          `${t("Sign out")} ${source.alias}`,
                          t(
                            "Sign out to revoke this session and remove saved credentials.",
                          ),
                          async () => {
                            try {
                              await API.Logout(source.alias);
                            } finally {
                              setIdentities((old) => {
                                const next = { ...old };
                                delete next[source.alias];
                                return next;
                              });
                              ui.changed(t("Sign-in status refreshed."));
                            }
                          },
                        )
                      }
                    >
                      {t("Sign out")}
                    </button>
                    <button
                      className="text-danger"
                      disabled={ui.busy}
                      onClick={() =>
                        ui.confirm(
                          `${t("Remove connection")} ${source.alias}`,
                          t(
                            "Installed files will remain. Sign out first to revoke the remote session.",
                          ),
                          () => API.RemoveSource(source.alias),
                        )
                      }
                    >
                      {t("Remove connection")}
                    </button>
                  </div>
                </Modal>
              )}
            </article>
          ))}
        </div>
      )}
      <p className="connection-help">
        {t(
          "Installed skills stay on this device when you switch or remove a connection.",
          "切换或移除连接不会删除本机已安装的技能。",
        )}
      </p>
      {account && (
        <Modal
          title={`${t("Sign in")} ${account}`}
          onClose={() => {
            setAccount("");
            setPassword("");
          }}
        >
          <form
            className="stack"
            onSubmit={(e) => {
              e.preventDefault();
              void ui.run(async () => {
                const secret = password;
                setPassword("");
                const identity = await API.LoginPassword(
                  account,
                  username,
                  secret,
                );
                setIdentities((old) => ({ ...old, [account]: identity }));
                setAccount("");
                ui.changed(`${t("Signed in as")}: ${identity.username}`);
              });
            }}
          >
            <Field label={t("Username")}>
              <input
                required
                autoComplete="username"
                value={username}
                onChange={(e) => setUsername(e.target.value)}
              />
            </Field>
            <Field label={t("Password")}>
              <input
                required
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
            <p className="muted">
              {t(
                "Credentials are saved in your system store and shared with the CLI.",
              )}
            </p>
            <button className="primary" disabled={ui.busy}>
              {t("Sign in")}
            </button>
          </form>
        </Modal>
      )}
      {device && (
        <Modal
          title={`${t("Authorize", "授权")} ${device.source}`}
          onClose={() => setDevice(null)}
        >
          <p>
            {t("Open this hub in your browser and enter the following code.")}
          </p>
          <div className="device-code">{device.view.code}</div>
          <p className="path">{device.view.url}</p>
          <p className="muted">
            {t("Waiting for authorization ·")}
            {new Date(device.view.expires_at).toLocaleTimeString()}{" "}
            {t("expiry")}
          </p>
          <div className="actions">
            <button onClick={() => setDevice(null)}>
              {t("Cancel sign-in")}
            </button>
            <button
              className="primary"
              disabled={ui.busy}
              onClick={() =>
                void ui.run(() => API.OpenLoginBrowser(device.view.handle))
              }
            >
              {t("Open browser")}
            </button>
          </div>
        </Modal>
      )}
    </>
  );
}

export function LocalCache() {
  const ui = useUI();
  const [info, setInfo] = useState<CacheInfo | null>(null);
  useEffect(() => {
    void ui.run(async () => setInfo(await API.CacheInfo()));
  }, [ui.revision]);
  return (
    <section className="preference-section cache-section">
      <div className="section-heading">
        <div>
          <h2>{t("Download cache")}</h2>
          <p className="muted">
            {t("Shared with the CLI. Access is checked on every use.")}
          </p>
        </div>
        <button
          disabled={ui.busy || !info?.entries}
          onClick={() =>
            ui.confirm(
              t("Clear download cache"),
              t(
                "Only cached packages are removed. Installations, projects and accounts remain.",
              ),
              async () => setInfo(await API.ClearCache()),
            )
          }
        >
          {t("Clear cache")}
        </button>
      </div>
      {info && (
        <>
          <p>
            {`${info.entries} ${t("packages", "个缓存包")} · ${(info.bytes / 1048576).toFixed(1)} MiB / ${info.limit_bytes / 1048576} MiB`}
          </p>
          <p className="muted">
            {t(
              `Up to ${info.limit_entries} packages. Least recently used packages are cleared first.`,
              `最多 ${info.limit_entries} 个缓存包，优先清理最久未使用的内容。`,
            )}
          </p>
          <details>
            <summary>{t("Cache location")}</summary>
            <p className="path">{info.directory}</p>
          </details>
        </>
      )}
    </section>
  );
}
