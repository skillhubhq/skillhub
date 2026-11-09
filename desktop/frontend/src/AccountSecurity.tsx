import { AccessTokens } from "./AccessTokens";
import { useState } from "react";
import { API, Field, Modal, useUI } from "./ui";
import { ResourceState, useResource } from "./community-ui";
import { isWeb } from "./platform";
import { signInURL } from "./auth";
import { t } from "./i18n";

export function AccountSecurity() {
  const ui = useUI(),
    source = ui.activeSource;
  const identity = useResource(
    () => (source ? API.Identity(source) : Promise.resolve(null)),
    [source, ui.revision],
  );
  const [editing, setEditing] = useState(false);
  const close = () => {
    setEditing(false);
    setCurrent("");
    setPassword("");
    setConfirm("");
  };
  const [current, setCurrent] = useState(""),
    [password, setPassword] = useState(""),
    [confirm, setConfirm] = useState(""),
    [done, setDone] = useState(false);
  const valid =
    new TextEncoder().encode(password).length >= 12 &&
    new TextEncoder().encode(password).length <= 72 &&
    password === confirm;
  return (
    <section className="preference-section">
      <h2>{t("Account security", "账号安全")}</h2>
      <p>
        {ui.sources.find((s) => s.alias === source)?.display_name} ·{" "}
        {identity.value?.username}
      </p>
      {done ? (
        <div role="status">
          <p>
            {t(
              "Password changed. All sessions have been signed out. Sign in again with your new password.",
              "密码已修改，所有会话均已退出。请使用新密码重新登录。",
            )}
          </p>
          {isWeb ? (
            <a href={signInURL()}>{t("Sign in")}</a>
          ) : (
            <button onClick={() => ui.navigate("sources")}>
              {t("Go to Connections")}
            </button>
          )}
        </div>
      ) : (
        <ResourceState {...identity} retry={identity.reload}>
          {identity.value?.federated ? (
            <p>
              {t(
                "Your password and multi-factor authentication are managed by your enterprise sign-in provider.",
                "你的密码和多因素认证由企业身份提供者管理。",
              )}
            </p>
          ) : (
            identity.value && (
              <>
                <button onClick={() => setEditing(true)}>
                  {t("Change password", "修改密码")}
                </button>
                {editing && (
                  <Modal
                    title={t("Change password", "修改密码")}
                    onClose={close}
                  >
                    <form
                      className="stack"
                      onSubmit={(e) => {
                        e.preventDefault();
                        if (!valid) return;
                        void ui.run(async () => {
                          await API.ChangePassword(source, current, password);
                          setCurrent("");
                          setPassword("");
                          setConfirm("");
                          setDone(true);
                          setEditing(false);
                          ui.changed(
                            t(
                              "Password changed. Sign in again.",
                              "密码已修改，请重新登录。",
                            ),
                          );
                        });
                      }}
                    >
                      <Field label={t("Current password", "当前密码")}>
                        <input
                          required
                          type="password"
                          autoComplete="current-password"
                          value={current}
                          onChange={(e) => setCurrent(e.target.value)}
                        />
                      </Field>
                      <Field
                        label={t("New password", "新密码")}
                        hint={t(
                          "12–72 bytes. Changing it signs out all devices.",
                          "12–72 字节。修改后所有设备都会退出登录。",
                        )}
                      >
                        <input
                          required
                          type="password"
                          autoComplete="new-password"
                          value={password}
                          onChange={(e) => setPassword(e.target.value)}
                        />
                      </Field>
                      <Field label={t("Confirm new password", "确认新密码")}>
                        <input
                          required
                          type="password"
                          autoComplete="new-password"
                          value={confirm}
                          onChange={(e) => setConfirm(e.target.value)}
                        />
                      </Field>
                      {confirm && password !== confirm && (
                        <p role="alert">
                          {t("Passwords do not match.", "两次密码不一致。")}
                        </p>
                      )}
                      <button
                        className="primary"
                        disabled={ui.busy || !valid || !current}
                      >
                        {t("Change password", "修改密码")}
                      </button>
                    </form>
                  </Modal>
                )}
              </>
            )
          )}
        </ResourceState>
      )}
      {identity.value && !done && <AccessTokens key={source} source={source} />}
    </section>
  );
}
