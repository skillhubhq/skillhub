import { DesktopBehavior } from "./DesktopLifecycle";
import { ProjectLink, projectURL } from "./Downloads";
import { LocalCache } from "./Sources";
import { Select } from "./Select";
import { AccountSecurity } from "./AccountSecurity";
import { Field, useUI } from "./ui";
import { t, useLanguage, setLanguage } from "./i18n";
import { useTheme, setTheme, type Theme } from "./preferences";
import { isWeb } from "./platform";
import { ApplicationUpdates } from "./ApplicationUpdates";
export function Settings() {
  const ui = useUI(),
    language = useLanguage(),
    theme = useTheme();
  return (
    <div className="preferences-page">
      <section className="preference-section">
        <h2>{t("Appearance and language", "外观与语言")}</h2>
        <div className="form-grid preferences-fields">
          <Field label={t("Appearance", "外观")}>
            <Select
              value={theme}
              onChange={(e) => setTheme(e.target.value as Theme)}
            >
              <option value="system">{t("Follow system", "跟随系统")}</option>
              <option value="light">{t("Light", "浅色")}</option>
              <option value="dark">{t("Dark", "深色")}</option>
            </Select>
          </Field>
          <Field label={t("Language", "语言")}>
            <Select
              value={language}
              onChange={(e) => setLanguage(e.target.value as "en" | "zh")}
            >
              <option value="en">English</option>
              <option value="zh">简体中文</option>
            </Select>
          </Field>
        </div>
      </section>
      <section className="preference-section">
        <h2>
          {isWeb
            ? t("This community", "当前社区")
            : t("Your communities", "你的社区")}
        </h2>
        <div className="actions">
          {ui.access === "member" && (
            <button onClick={() => ui.navigate("profile")}>
              {t("My profile")}
            </button>
          )}
          {!isWeb && (
            <button onClick={() => ui.navigate("sources")}>
              {t("Manage connections", "管理连接与账号")}
            </button>
          )}
          {!isWeb && ui.workspaceAccess?.manage && (
            <button onClick={() => ui.navigate("management")}>
              {t("Hub settings")}
            </button>
          )}
        </div>
      </section>
      {!isWeb && <DesktopBehavior />}
      <AccountSecurity key={ui.activeSource} />
      {!isWeb && <LocalCache />}
      {!isWeb && <ApplicationUpdates />}
      <section className="preference-section">
        <h2>{t("About SkillHub", "关于 SkillHub")}</h2>
        <div className="actions">
          <ProjectLink
            href={
              isWeb
                ? `${location.origin}/docs/`
                : `${projectURL}/tree/main/docs`
            }
          >
            {t("Documentation", "使用文档")}
          </ProjectLink>
          <ProjectLink href={projectURL}>GitHub</ProjectLink>
          <ProjectLink href={`${projectURL}/blob/main/LICENSE`}>
            Apache-2.0
          </ProjectLink>
        </div>
      </section>
    </div>
  );
}
