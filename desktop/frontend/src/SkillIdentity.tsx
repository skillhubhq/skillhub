import { SkillIcon as Kind } from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";
import { Icon } from "./Icon";
import { t } from "./i18n";

export type SkillIdentity = {
  name: string;
  display_name_en?: string;
  display_name_zh?: string;
  icon?: number;
};

// 只选择作者提供的译名，不自动翻译稳定名称。
export function skillTitle(skill: SkillIdentity): string {
  return t(
    skill.display_name_en || skill.display_name_zh || skill.name,
    skill.display_name_zh || skill.display_name_en || skill.name,
  );
}

export const skillIcons = [
  {
    id: Kind.SkillIcon_SKILL_ICON_UNSPECIFIED,
    glyph: "skill",
    en: "Default",
    zh: "默认",
  },
  {
    id: Kind.SkillIcon_SKILL_ICON_DOCUMENT,
    glyph: "submissions",
    en: "Document",
    zh: "文档",
  },
  { id: Kind.SkillIcon_SKILL_ICON_CODE, glyph: "code", en: "Code", zh: "代码" },
  {
    id: Kind.SkillIcon_SKILL_ICON_CHART,
    glyph: "chart",
    en: "Analysis",
    zh: "分析",
  },
  {
    id: Kind.SkillIcon_SKILL_ICON_WRITING,
    glyph: "writing",
    en: "Writing",
    zh: "写作",
  },
  {
    id: Kind.SkillIcon_SKILL_ICON_RESEARCH,
    glyph: "explore",
    en: "Research",
    zh: "研究",
  },
  {
    id: Kind.SkillIcon_SKILL_ICON_DESIGN,
    glyph: "grid",
    en: "Design",
    zh: "设计",
  },
  {
    id: Kind.SkillIcon_SKILL_ICON_WORKFLOW,
    glyph: "workflow",
    en: "Workflow",
    zh: "流程",
  },
  {
    id: Kind.SkillIcon_SKILL_ICON_TERMINAL,
    glyph: "terminal",
    en: "Terminal",
    zh: "终端",
  },
];
export function SkillSymbol({ icon = 0 }: { icon?: number }) {
  return (
    <span className="skill-emblem" aria-hidden="true">
      <Icon name={skillIcons.find((i) => i.id === icon)?.glyph || "skill"} />
    </span>
  );
}
export function SkillIconPicker({
  value,
  onChange,
}: {
  value: Kind;
  onChange: (value: Kind) => void;
}) {
  return (
    <fieldset className="skill-icon-picker full">
      <legend>{t("Skill icon", "技能图标")}</legend>
      <div className="skill-icon-options">
        {skillIcons.map((icon) => (
          <label key={icon.id} title={t(icon.en, icon.zh)}>
            <input
              type="radio"
              name="skill-icon"
              value={icon.id}
              checked={value === icon.id}
              onChange={() => onChange(icon.id)}
            />
            <span>
              <Icon name={icon.glyph} />
              <small>{t(icon.en, icon.zh)}</small>
            </span>
          </label>
        ))}
      </div>
    </fieldset>
  );
}
