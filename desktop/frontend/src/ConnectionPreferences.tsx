import { useState } from "react";
import { API, Field, useUI, type Source } from "./ui";
import { t } from "./i18n";

export const connectionName = (
  source?: Pick<Source, "label" | "display_name" | "alias">,
) => source?.label || source?.display_name || source?.alias || "";

export function ConnectionPreferences({ source }: { source: Source }) {
  const ui = useUI();
  const [name, setName] = useState(source.label || "");
  const [note, setNote] = useState(source.note || "");
  const [saved, setSaved] = useState(false);
  return (
    <form
      className="stack connection-preferences"
      onSubmit={(event) => {
        event.preventDefault();
        void ui.run(async () => {
          const updated = await API.UpdateSourcePreferences(
            source.alias,
            name,
            note,
          );
          setName(updated.label);
          setNote(updated.note);
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
        label={t("Connection name", "连接显示名")}
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
      <Field
        label={t("Note", "备注")}
        hint={t(
          "Keep passwords and tokens in the credential store, never in notes.",
          "备注中不要填写密码或令牌，请使用系统凭据库。",
        )}
      >
        <input
          name="connection-note"
          maxLength={300}
          value={note}
          onChange={(event) => {
            setNote(event.target.value);
            setSaved(false);
          }}
        />
      </Field>
      <div className="actions">
        {saved && (
          <span role="status">{t("Saved on this device", "已保存到本机")}</span>
        )}
        <button
          className="primary"
          disabled={
            ui.busy ||
            (name === (source.label || "") && note === (source.note || ""))
          }
        >
          {t("Save changes", "保存修改")}
        </button>
      </div>
    </form>
  );
}
