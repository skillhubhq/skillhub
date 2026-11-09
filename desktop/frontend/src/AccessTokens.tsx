import { useState } from "react";
import { API, Field, Modal, useUI } from "./ui";
import { useResource, ResourceState } from "./community-ui";
import { Select } from "./Select";
import { CopyValue } from "./SkillActions";
import { t } from "./i18n";

export function AccessTokens({ source }: { source: string }) {
  const ui = useUI(),
    list = useResource(() => API.AccessTokens(source), [source]);
  const [open, setOpen] = useState(false),
    [secret, setSecret] = useState(""),
    [name, setName] = useState(""),
    [scope, setScope] = useState("read"),
    [namespace, setNamespace] = useState(""),
    [days, setDays] = useState("30");
  const spaces = useResource(async () => {
    let token = "";
    const rows = [];
    do {
      const p = await API.AdminList(source, "submission-namespaces", "", token);
      rows.push(...(p.rows ?? []));
      token = p.next_page;
    } while (token);
    return rows;
  }, [source]);
  return (
    <section className="preference-section">
      <div className="section-heading">
        <h3>{t("Automation tokens", "自动化令牌")}</h3>
        <button onClick={() => setOpen(true)}>
          {t("Create token", "创建令牌")}
        </button>
      </div>
      <p>
        {t(
          "Use a token in CI or scripts. Access follows your current Hub permissions.",
          "用于 CI 或脚本，访问范围受你当前的 Hub 权限约束。",
        )}
      </p>
      <ResourceState {...list} retry={list.reload}>
        {!list.value?.length && (
          <p>{t("No automation tokens.", "暂无自动化令牌。")}</p>
        )}
        <div className="stack">
          {list.value?.map((v) => (
            <div className="section-heading" key={v.id}>
              <div>
                <strong>{v.name}</strong>
                <p className="muted">
                  {v.scope === "publish"
                    ? `${t("Publish", "发布")} · ${v.namespace}`
                    : t("Read only", "只读")}{" "}
                  ·{" "}
                  {v.revoked
                    ? t("Revoked", "已撤销")
                    : Number(v.expires_at) * 1000 < Date.now()
                      ? t("Expired", "已过期")
                      : `${t("Expires", "到期")} ${new Date(Number(v.expires_at) * 1000).toLocaleDateString()}`}
                </p>
                <small>
                  {v.last_used_at
                    ? `${t("Last used", "最近使用")} ${new Date(Number(v.last_used_at) * 1000).toLocaleString()}`
                    : t("Not used yet", "尚未使用")}
                </small>
              </div>
              {!v.revoked && (
                <button
                  disabled={ui.busy}
                  onClick={() =>
                    void ui.run(async () => {
                      await API.RevokeAccessToken(source, v.id);
                      list.reload();
                    })
                  }
                >
                  {t("Revoke", "撤销")}
                </button>
              )}
            </div>
          ))}
        </div>
      </ResourceState>
      {open && (
        <Modal
          title={t("Create automation token", "创建自动化令牌")}
          onClose={() => {
            setOpen(false);
            setSecret("");
            setName("");
          }}
        >
          {secret ? (
            <div className="stack">
              <p>
                {t(
                  "Copy this token now. It will not be shown again.",
                  "请现在保存令牌，关闭后无法再次查看。",
                )}
              </p>
              <CopyValue label={t("Token", "令牌")} value={secret} />
              <p>
                {t(
                  "CLI: provide it through SKILLHUB_TOKEN_SOURCE, replacing SOURCE with your connection alias in uppercase.",
                  "CLI：通过 SKILLHUB_TOKEN_SOURCE 环境变量提供，将 SOURCE 替换为连接别名的大写形式。",
                )}
              </p>
            </div>
          ) : (
            <form
              className="stack"
              onSubmit={(e) => {
                e.preventDefault();
                void ui.run(async () => {
                  const v = await API.CreateAccessToken(
                    source,
                    name,
                    scope,
                    namespace,
                    Number(days),
                  );
                  setSecret(v.secret);
                  list.reload();
                });
              }}
            >
              <Field label={t("Name", "名称")}>
                <input
                  required
                  maxLength={64}
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder={t("Release pipeline", "发布流水线")}
                />
              </Field>
              <Field label={t("Access", "访问范围")}>
                <Select
                  value={scope}
                  onChange={(e) => {
                    setScope(e.target.value);
                    setNamespace("");
                  }}
                >
                  <option value="read">{t("Read only", "只读")}</option>
                  <option value="publish" disabled={!spaces.value?.length}>
                    {t("Read and submit to one team", "读取及向指定团队投稿")}
                  </option>
                </Select>
              </Field>
              {scope === "publish" && (
                <Field label={t("Team", "团队")}>
                  <Select
                    required
                    value={namespace}
                    onChange={(e) => setNamespace(e.target.value)}
                  >
                    <option value="">{t("Choose a team", "选择团队")}</option>
                    {spaces.value?.map((v) => (
                      <option key={v.id} value={v.id}>
                        {v.name || v.id}
                      </option>
                    ))}
                  </Select>
                </Field>
              )}
              <Field label={t("Expires after", "有效期")}>
                <Select value={days} onChange={(e) => setDays(e.target.value)}>
                  {[7, 30, 90].map((v) => (
                    <option key={v} value={v}>
                      {v} {t("days", "天")}
                    </option>
                  ))}
                </Select>
              </Field>
              <button
                className="primary"
                disabled={
                  ui.busy || !name.trim() || (scope === "publish" && !namespace)
                }
              >
                {t("Create token", "创建令牌")}
              </button>
            </form>
          )}
        </Modal>
      )}
    </section>
  );
}
