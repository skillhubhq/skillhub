import { ConnectionPreferences, connectionName } from "./ConnectionPreferences";
import { OfficialBadge, officialHubURL, isOfficialHub } from "./official";
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
  const [loginRetrying, setLoginRetrying] = useState(false);
  const [adding, setAdding] = useState(false),
    [managing, setManaging] = useState(
      () =>
        new URLSearchParams(location.hash.split("?")[1] ?? "").get("manage") ??
        "",
    );
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
  const [identityEpoch, setIdentityEpoch] = useState(0);
  useEffect(() => {
    const recheck = () => setIdentityEpoch((value) => value + 1);
    window.addEventListener("online", recheck);
    window.addEventListener("focus", recheck);
    return () => {
      window.removeEventListener("online", recheck);
      window.removeEventListener("focus", recheck);
    };
  }, []);
  const sourceKey = ui.sources.map((s) => s.alias).join("/");
  useEffect(() => {
    let active = true;
    const aliases = new Set(ui.sources.map((source) => source.alias));
    setIdentities((old) =>
      Object.fromEntries(
        Object.entries(old).filter(([alias]) => aliases.has(alias)),
      ),
    );
    setIdentityState((old) =>
      Object.fromEntries(
        Object.entries(old).filter(([alias]) => aliases.has(alias)),
      ),
    );
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
          if (active) {
            setIdentities((old) => {
              const next = { ...old };
              delete next[source.alias];
              return next;
            });
            setIdentityState((old) => ({
              ...old,
              [source.alias]: needsSignIn(String(error))
                ? "signed-out"
                : "unavailable",
            }));
          }
        });
    return () => {
      active = false;
    };
  }, [ui.revision, sourceKey, identityEpoch]);
  useEffect(() => {
    if (!device) return;
    setLoginRetrying(false);
    let stopped = false;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const result = await API.PollLogin(device.view.handle);
        if (stopped) return;
        setLoginRetrying(false);
        if (result.pending) {
          timer = setTimeout(poll, device.view.interval_seconds * 1000);
        } else {
          setIdentities((old) => ({
            ...old,
            [device.source]: result.identity,
          }));
          setDevice(null);
          ui.refresh();
          ui.changed(
            `${t("Signed in to", "已登录")} ${device.source}: ${result.identity.username}`,
          );
        }
      } catch (error) {
        if (
          !stopped &&
          /unavailable|deadline.exceeded|unexpected EOF|network|connection reset|fetch failed/i.test(
            String(error),
          ) &&
          Date.now() < Date.parse(device.view.expires_at)
        ) {
          setLoginRetrying(true);
          timer = setTimeout(
            poll,
            Math.max(3, device.view.interval_seconds) * 1000,
          );
          return;
        }
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
      {!ui.sources.some((s) => isOfficialHub(s.url)) && (
        <section className="official-connection">
          <div>
            <strong>
              SkillHub <OfficialBadge url={officialHubURL} />
            </strong>
            <p>
              {t(
                "Discover skills from the public community. No account is needed to browse.",
                "发现公共社区中的技能，无需登录即可浏览。",
              )}
            </p>
          </div>
          <button
            onClick={() => {
              setAlias(
                ui.sources.some((s) => s.alias === "official")
                  ? "skillhub-official"
                  : "official",
              );
              setURL(officialHubURL);
              setAdding(true);
            }}
          >
            {t("Connect official Hub", "连接官方 Hub")}
          </button>
        </section>
      )}
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
                <h2>
                  {connectionName(source)} <OfficialBadge url={source.url} />
                </h2>
                <p className="connection-address" title={source.url}>
                  {source.url}
                </p>
                {source.label && (
                  <small className="connection-hub-name">
                    {t("Hub name", "站点名称")}: {source.display_name}
                  </small>
                )}
                <small className="connection-alias">
                  {t("CLI ID", "CLI 标识")}: <code>{source.alias}</code>
                </small>
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
                        : t(
                            "Connection unavailable — try again",
                            "连接暂时不可用，请重试",
                          )}
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
                  title={t("Manage connection", "管理连接")}
                  onClose={() => setManaging("")}
                >
                  <section className="connection-server">
                    <strong>
                      {source.display_name} <OfficialBadge url={source.url} />
                    </strong>
                    <p className="connection-address">{source.url}</p>
                    <button
                      className="quiet"
                      disabled={ui.busy}
                      onClick={() =>
                        void ui.run(async () => {
                          await API.RefreshSource(source.alias);
                          ui.changed(
                            t("Hub information refreshed.", "站点信息已刷新。"),
                          );
                        })
                      }
                    >
                      {t("Refresh Hub information", "刷新站点信息")}
                    </button>
                  </section>
                  <h3>{t("Account and access", "账号与访问")}</h3>
                  <p>
                    {t("Signed-in account", "登录账号")}:{" "}
                    {identities[source.alias]?.username ||
                      (!identityState[source.alias]
                        ? t("Checking account…", "正在确认账号…")
                        : identityState[source.alias] === "signed-out"
                          ? t("Not signed in")
                          : t(
                              "Connection unavailable — try again",
                              "连接暂时不可用，请重试",
                            ))}
                  </p>
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
                          ui.refresh();
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
                  <ConnectionPreferences source={source} />
                  <details className="connection-technical">
                    <summary>
                      {t("Connection identifiers", "连接标识与地址")}
                    </summary>
                    <p>
                      {t("CLI ID", "CLI 标识")}: <code>{source.alias}</code>
                    </p>
                    <p className="muted">
                      {t(
                        "This stable ID is used by CLI commands and project files. Editing the connection name leaves these references and your login intact. To connect another address, add a separate connection.",
                        "稳定标识供 CLI 和项目文件引用。修改显示名不会改变这些引用或登录状态。使用其他地址时，请添加独立连接。",
                      )}
                    </p>
                    <code className="path">{source.registry_id}</code>
                  </details>
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
                ui.refresh();
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
          {loginRetrying && (
            <p role="status">
              {t(
                "Connection interrupted. Retrying sign-in…",
                "连接暂时中断，正在恢复登录…",
              )}
            </p>
          )}
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
