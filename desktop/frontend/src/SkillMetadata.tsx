import { useState } from "react";
import type { SkillView } from "../bindings/github.com/skillhubhq/skillhub/desktop/models";
import { API, Field, Modal, splitList, useUI } from "./ui";
import { Select } from "./Select";
import { SkillIconPicker } from "./SkillIdentity";
import { useResource } from "./community-ui";
import { t } from "./i18n";

export function SkillMetadata({
  source,
  skill,
  saved,
}: {
  source: string;
  skill: SkillView;
  saved: () => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      {skill.can_edit && (
        <button onClick={() => setOpen(true)}>
          {t("Edit skill details", "编辑技能资料")}
        </button>
      )}
      {open && (
        <Editor
          source={source}
          skill={skill}
          close={() => setOpen(false)}
          saved={saved}
        />
      )}
    </>
  );
}
function Editor({
  source,
  skill,
  close,
  saved,
}: {
  source: string;
  skill: SkillView;
  close: () => void;
  saved: () => void;
}) {
  const ui = useUI();
  const [form, setForm] = useState({
    namespace: skill.namespace,
    name: skill.name,
    expected_revision: skill.revision,
    display_name_en: skill.display_name_en,
    display_name_zh: skill.display_name_zh,
    description: skill.description,
    icon: skill.icon,
    category: skill.category,
    tags: skill.tags ?? [],
  });
  const categories = useResource(async () => {
    let token = "";
    const rows = [];
    do {
      const page = await API.AdminList(source, "categories", "", token);
      rows.push(...(page.rows ?? []));
      token = page.next_page;
    } while (token);
    return rows.filter((r) => r.enabled);
  }, [source]);
  return (
    <Modal title={t("Edit skill details", "编辑技能资料")} onClose={close}>
      <form
        className="form-grid"
        onSubmit={(e) => {
          e.preventDefault();
          void ui.run(async () => {
            await API.UpdateSkillMetadata(source, form);
            close();
            saved();
          });
        }}
      >
        <Field label={t("English name", "英文名称")}>
          <input
            value={form.display_name_en}
            maxLength={80}
            onChange={(e) =>
              setForm({ ...form, display_name_en: e.target.value })
            }
          />
        </Field>
        <Field label={t("Chinese name", "中文名称")}>
          <input
            value={form.display_name_zh}
            maxLength={80}
            onChange={(e) =>
              setForm({ ...form, display_name_zh: e.target.value })
            }
          />
        </Field>
        <Field label={t("Description", "简介")}>
          <textarea
            required
            maxLength={1024}
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
          />
        </Field>
        <SkillIconPicker
          value={form.icon}
          onChange={(icon) => setForm({ ...form, icon })}
        />
        <Field label={t("Category", "分类")}>
          <Select
            value={form.category}
            onChange={(e) => setForm({ ...form, category: e.target.value })}
          >
            <option value="">{t("Uncategorized", "未分类")}</option>
            {categories.value?.map((c) => (
              <option key={c.name} value={c.name}>
                {c.title || c.name}
              </option>
            ))}
          </Select>
        </Field>
        <Field label={t("Tags", "标签")}>
          <input
            defaultValue={form.tags.join(", ")}
            onChange={(e) =>
              setForm({ ...form, tags: splitList(e.target.value) })
            }
          />
        </Field>
        <p className="muted">
          {t(
            "Shown across the skill's pages. Package contents stay tied to their published version.",
            "用于技能的各个页面。包内容仍对应其发布版本。",
          )}
        </p>
        <div className="actions">
          <button type="button" onClick={close}>
            {t("Cancel")}
          </button>
          <button className="primary" disabled={ui.busy} type="submit">
            {t("Save")}
          </button>
        </div>
      </form>
    </Modal>
  );
}
