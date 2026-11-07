import { Select } from "./Select";
import { Icon } from "./Icon";

export function PreferenceControls({
  language,
  theme,
  onLanguage,
  onTheme,
}: {
  language: "en" | "zh";
  theme: string;
  onLanguage: (value: "en" | "zh") => void;
  onTheme: (value: "light" | "dark" | "system") => void;
}) {
  const tr = (en: string, zh: string) => (language === "zh" ? zh : en);
  return (
    <>
      <label className="quick-language" title={tr("Language", "语言")}>
        <Icon name="language" />
        <Select
          aria-label={tr("Language", "语言")}
          value={language}
          onChange={(e) => onLanguage(e.target.value as "en" | "zh")}
        >
          <option value="en">English</option>
          <option value="zh">简体中文</option>
        </Select>
      </label>
      <div className="quick-theme" title={tr("Appearance", "外观")}>
        <Select
          aria-label={tr("Appearance", "外观")}
          value={theme}
          onChange={(e) =>
            onTheme(e.target.value as "light" | "dark" | "system")
          }
          triggerContent={
            <Icon
              name={
                theme === "dark" ? "moon" : theme === "light" ? "sun" : "device"
              }
            />
          }
        >
          <option value="light">{tr("Light", "浅色")}</option>
          <option value="dark">{tr("Dark", "深色")}</option>
          <option value="system">{tr("Follow system", "跟随系统")}</option>
        </Select>
      </div>
    </>
  );
}
