import { useEffect, useState } from "react";
import { API, Field } from "./ui";
import { Select } from "./Select";
import { SkillSymbol, skillTitle } from "./SkillIdentity";
import { t } from "./i18n";
import type { SkillView } from "../bindings/github.com/skillhubhq/skillhub/desktop/models";
import type { ReleaseInfo } from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
export function ProjectSkillPicker({
  source,
  namespace,
  name,
  version,
  onChange,
}: {
  source: string;
  namespace: string;
  name: string;
  version: string;
  onChange: (namespace: string, name: string, version: string) => void;
}) {
  const [query, setQuery] = useState(""),
    [skills, setSkills] = useState<SkillView[]>([]),
    [releases, setReleases] = useState<ReleaseInfo[]>([]),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(false),
    [next, setNext] = useState(""),
    [cursor, setCursor] = useState("");
  useEffect(() => {
    let active = true;
    setSkills([]);
    setNext("");
    setError("");
    setLoading(!!source);
    const timer = setTimeout(() => {
      if (source)
        void API.Search(source, query, "", [], cursor, 0, false)
          .then((pages) => {
            if (!active) return;
            const p = pages?.find((p) => p.source === source);
            if (p?.error) throw new Error(p.error);
            setSkills(p?.skills ?? []);
            setNext(p?.next_page ?? "");
          })
          .catch(() => {
            if (active)
              setError(
                t(
                  "Unable to load skills. Check this connection.",
                  "无法加载技能，请检查此连接。",
                ),
              );
          })
          .finally(() => {
            if (active) setLoading(false);
          });
    }, 200);
    return () => {
      active = false;
      clearTimeout(timer);
    };
  }, [source, query, cursor]);
  useEffect(() => {
    let active = true;
    setReleases([]);
    if (source && namespace && name)
      void (async () => {
        let token = "";
        const found: ReleaseInfo[] = [];
        do {
          const page = await API.Releases(
            `${source}/${namespace}/${name}`,
            token,
          );
          if (!active) return;
          found.push(...(page.releases ?? []));
          token = page.next_page;
        } while (token);
        setReleases(found);
      })().catch(() => {
        if (active) setError(t("Unable to load versions.", "无法加载版本。"));
      });
    return () => {
      active = false;
    };
  }, [source, namespace, name]);
  return (
    <div className="full project-skill-picker">
      <Field label={t("Search skills", "搜索技能")}>
        <input
          disabled={!source}
          value={query}
          placeholder={t("Search by name or purpose…", "按名称或用途搜索…")}
          onChange={(e) => {
            setQuery(e.target.value);
            setCursor("");
          }}
        />
      </Field>
      {error ? (
        <p role="alert">{error}</p>
      ) : loading ? (
        <p role="status">{t("Loading…", "加载中…")}</p>
      ) : (
        <div className="project-skill-results">
          {skills.map((s) => (
            <button
              type="button"
              key={`${s.namespace}/${s.name}`}
              aria-pressed={namespace === s.namespace && name === s.name}
              onClick={() => onChange(s.namespace, s.name, s.version)}
            >
              <SkillSymbol icon={s.icon} />
              <span>
                <strong>{skillTitle(s)}</strong>
                <small>
                  {s.namespace}/{s.name}
                </small>
              </span>
            </button>
          ))}
          {!skills.length && source && (
            <p>{t("No matching skills", "没有匹配的技能")}</p>
          )}
        </div>
      )}
      {cursor && (
        <button type="button" onClick={() => setCursor("")}>
          {t("First page", "返回第一页")}
        </button>
      )}
      {next && (
        <button type="button" onClick={() => setCursor(next)}>
          {t("Next page")}
        </button>
      )}
      {name && (
        <Field label={t("Exact version")}>
          <Select
            required
            value={version}
            onChange={(e) => onChange(namespace, name, e.target.value)}
          >
            {!releases.some((r) => r.version === version) && (
              <option value={version}>
                {namespace}/{name} · {version}
              </option>
            )}
            {releases.map((r) => (
              <option key={r.id} value={r.version}>
                {namespace}/{name} · {r.version}
              </option>
            ))}
          </Select>
        </Field>
      )}
    </div>
  );
}
