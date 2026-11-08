import { useState } from "react";
import { API, Field, useUI, type Source } from "./ui";
import { t } from "./i18n";

export const connectionName = (
  source?: Pick<Source, "label" | "display_name" | "alias">,
) => source?.label || source?.display_name || source?.alias || "";

export function ConnectionPreferences({ source }: { source: Source }) {
  const ui = useUI();
  const [name, setName] = useState(source.label || "");
  const [saved, setSaved] = useState(false);
  return (
    <details className="connection-custom-name">
      <summary>{t("Custom name", "自定义名称")}</summary>
      <form
        className="stack connection-preferences"
        onSubmit={(event) => {
          event.preventDefault();
          void ui.run(async () => {
            const updated = await API.UpdateSourcePreferences(
              source.alias,
              name,
              source.note || "",
            );
            setName(updated.label);
            setSaved(true);
            ui.changed(
              t(
                "Connection preferences saved on this device.",
                "连接设置已保存在本机。",
              ),
            );
          });
        }}
      >
        <Field
          label={t("Name on this device", "本机显示名称")}
          hint={t(
            "Only on this device. Leave blank to use the Hub name.",
            "仅本机使用，留空则使用站点名称。",
          )}
        >
          <input
            name="connection-name"
            maxLength={80}
            value={name}
            placeholder={source.display_name}
            onChange={(event) => {
              setName(event.target.value);
              setSaved(false);
            }}
          />
        </Field>
        <div className="actions">
          {saved && (
            <span role="status">
              {t("Saved on this device", "已保存到本机")}
            </span>
          )}
          <button
            className="primary"
            disabled={ui.busy || name === (source.label || "")}
          >
            {t("Save changes", "保存修改")}
          </button>
        </div>
      </form>
    </details>
  );
}
