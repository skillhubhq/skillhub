import { useEffect, useState, useRef } from "react";
import { API, useUI } from "./ui";
import { t, useLanguage } from "./i18n";

type Preferences = Awaited<ReturnType<typeof API.DesktopPreferences>>;
export function DesktopLifecycle() {
  const ui = useUI();
  const language = useLanguage();
  const uiRef = useRef(ui);
  uiRef.current = ui;
  useEffect(() => {
    void API.DesktopPreferences()
      .then((v) => {
        if (v.language !== language)
          return API.SetDesktopPreferences({ ...v, language });
      })
      .catch(() => {});
  }, [language]);
  useEffect(() => {
    let stop: (() => void) | undefined;
    let disposed = false;
    const take = async () => {
      const ui = uiRef.current;
      if (ui.access === "checking") return;
      const action = await API.TakeDesktopAction();
      if (disposed || !action) return;
      if (action === "inbox") {
        if (ui.access === "member")
          window.dispatchEvent(new Event("skillhub-open-inbox"));
        else ui.navigate("sources");
      } else {
        if (action === "updates")
          sessionStorage.setItem("skillhub-check-update", "1");
        ui.navigate("settings");
        if (action === "updates")
          window.dispatchEvent(new Event("skillhub-check-update"));
      }
    };
    void import("@wailsio/runtime").then(({ Events }) => {
      if (disposed) return;
      stop = Events.On("desktop-action", () => {
        void take().catch(() => {});
      });
      void take().catch(() => {});
    });
    return () => {
      disposed = true;
      stop?.();
    };
  }, [ui.access]);
  return null;
}
export function DesktopBehavior() {
  const ui = useUI();
  const [value, setValue] = useState<Preferences | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    void ui.run(async () => setValue(await API.DesktopPreferences()));
  }, []);
  const change = (next: Preferences) => {
    setBusy(true);
    void ui
      .run(async () => {
        await API.SetDesktopPreferences(next);
        setValue(next);
      })
      .finally(() => setBusy(false));
  };
  return (
    <section className="preference-section">
      <h2>{t("Desktop behavior", "桌面行为")}</h2>
      {value && (
        <>
          <label className="check">
            <input
              type="checkbox"
              checked={value.keep_running}
              disabled={busy}
              onChange={(e) =>
                change({ ...value, keep_running: e.target.checked })
              }
            />
            {t("Keep running when the window closes", "关闭窗口后保持运行")}
          </label>
          <p>
            {t(
              "Reopen SkillHub from the menu bar or system tray. Choose Quit to exit.",
              "可从菜单栏或系统托盘重新打开，选择「退出」结束应用。",
            )}
          </p>
          <label className="check">
            <input
              type="checkbox"
              checked={value.start_at_login}
              disabled={busy}
              onChange={(e) =>
                change({ ...value, start_at_login: e.target.checked })
              }
            />
            {t("Start at login", "登录电脑时启动")}
          </label>
        </>
      )}
    </section>
  );
}
