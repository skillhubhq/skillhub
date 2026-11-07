import { useEffect, useId, useRef, useState } from "react";
import type { AdminRow } from "../bindings/github.com/skillhubhq/skillhub/desktop/models";
import { API } from "./platform";
import { t } from "./i18n";

// 完整遍历服务端分页，搜索不会悄悄只覆盖第一页；切换 Hub 后丢弃旧请求结果。
export function useDirectory(
  source: string,
  kind: string,
  namespace = "",
  query = "",
) {
  const [rows, setRows] = useState<AdminRow[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [attempt, retry] = useState(0);
  useEffect(() => {
    let active = true;
    setRows([]);
    setError("");
    const enabled = !!kind && (!namespace || query.trim().length >= 2);
    setLoading(enabled);
    if (enabled)
      void (async () => {
        let token = "";
        const seen = new Set<string>();
        const found: AdminRow[] = [];
        try {
          do {
            const page =
              namespace && kind === "users"
                ? await API.LookupUsers(source, namespace, query, token)
                : await API.AdminList(source, kind, "", token);
            if (!active) return;
            found.push(...(page.rows ?? []));
            setRows([...found]);
            token = page.next_page;
            if (token && seen.has(token))
              throw new Error("Repeated directory cursor");
            seen.add(token);
          } while (token);
        } catch {
          if (active)
            setError(
              t(
                "Unable to load choices. Check your connection and directory access.",
                "无法加载选项，请检查连接与目录访问权限。",
              ),
            );
        } finally {
          if (active) setLoading(false);
        }
      })();
    return () => {
      active = false;
    };
  }, [source, kind, namespace, query, attempt]);
  return { rows, loading, error, retry: () => retry((n) => n + 1) };
}
export const directoryLabel = (row: AdminRow, kind: string) =>
  kind === "users" ? row.title || row.name : row.name;

export function DirectoryPicker({
  source,
  kind,
  label,
  value,
  onChange,
  disabled = false,
  exclude = [],
  namespace = "",
}: {
  source: string;
  kind: "users" | "groups" | "namespaces";
  label: string;
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  exclude?: string[];
  namespace?: string;
}) {
  const id = useId(),
    root = useRef<HTMLDivElement>(null);
  const [query, setQuery] = useState(""),
    [open, setOpen] = useState(false),
    [cursor, setCursor] = useState(0);
  const [lookup, setLookup] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setLookup(open ? query : value), 200);
    return () => clearTimeout(timer);
  }, [open, query, value, namespace]);
  const directory = useDirectory(
    source,
    disabled && !value ? "" : kind,
    namespace,
    lookup,
  );
  const searching = directory.loading || lookup !== (open ? query : value);
  const [chosen, setChosen] = useState<AdminRow | null>(null);
  const selected =
    directory.rows.find((r) => r.id === value) ||
    (chosen?.id === value ? chosen : null);
  const matches = directory.rows.filter(
    (r) =>
      !exclude.includes(r.id) &&
      (kind !== "users" || r.enabled || r.id === value) &&
      `${r.title} ${r.name} ${r.id}`
        .toLocaleLowerCase()
        .includes(query.trim().toLocaleLowerCase()),
  );
  const [limit, setLimit] = useState(30);
  const shown = matches.slice(0, limit);
  const choose = (row: AdminRow) => {
    setChosen(row);
    onChange(row.id);
    setOpen(false);
    setQuery("");
    setCursor(0);
  };
  return (
    <div
      className="entity-picker"
      ref={root}
      onBlur={(e) => {
        if (!e.currentTarget.contains(e.relatedTarget)) {
          setOpen(false);
          setQuery("");
        }
      }}
    >
      <label htmlFor={id}>{label}</label>
      <div className="directory-input">
        <input
          id={id}
          role="combobox"
          aria-expanded={open}
          aria-controls={`${id}-choices`}
          aria-autocomplete="list"
          aria-activedescendant={
            open && shown[cursor] ? `${id}-option-${cursor}` : undefined
          }
          autoComplete="off"
          disabled={disabled}
          value={
            open
              ? query
              : selected
                ? `${directoryLabel(selected, kind)}${kind === "users" ? ` · @${selected.name}` : ""}`
                : value
                  ? t("Unavailable selection", "选项不可用")
                  : ""
          }
          placeholder={t("Search and select…", "搜索并选择…")}
          onFocus={() => {
            setOpen(true);
            setQuery("");
            setCursor(0);
          }}
          onChange={(e) => {
            setQuery(e.target.value);
            setCursor(0);
            setLimit(30);
            setOpen(true);
            if (value) onChange("");
          }}
          onKeyDown={(e) => {
            if (e.key === "Escape") {
              e.preventDefault();
              setOpen(false);
              setQuery("");
            }
            if (e.key === "ArrowDown" || e.key === "ArrowUp") {
              e.preventDefault();
              setOpen(true);
              const next = Math.max(
                0,
                Math.min(
                  shown.length - 1,
                  cursor + (e.key === "ArrowDown" ? 1 : -1),
                ),
              );
              setCursor(next);
              root.current
                ?.querySelector(`[id="${id}-option-${next}"]`)
                ?.scrollIntoView({ block: "nearest" });
            }
            if (e.key === "Enter") {
              e.preventDefault();
              if (open && shown[cursor]) choose(shown[cursor]);
              else setOpen(true);
            }
          }}
        />
        {value && !disabled && (
          <button
            type="button"
            className="picker-clear"
            aria-label={`${t("Clear selection", "清除选择")} · ${label}`}
            onClick={() => {
              onChange("");
              setQuery("");
              setOpen(true);
              root.current?.querySelector("input")?.focus();
            }}
          >
            ×
          </button>
        )}
      </div>
      {open && !disabled && (
        <div className="directory-menu">
          <div id={`${id}-choices`} role="listbox" aria-label={label}>
            {shown.map((row, index) => (
              <button
                type="button"
                role="option"
                id={`${id}-option-${index}`}
                aria-selected={row.id === value}
                className={index === cursor ? "highlighted" : ""}
                key={row.id}
                onMouseEnter={() => setCursor(index)}
                onClick={() => choose(row)}
              >
                <strong>{directoryLabel(row, kind)}</strong>
                {kind === "users" && <small>@{row.name}</small>}
              </button>
            ))}
          </div>
          {matches.length > limit && (
            <button type="button" onClick={() => setLimit((n) => n + 30)}>
              {t("Show more matches", "显示更多匹配项")}
            </button>
          )}
          {searching && (
            <p role="status">{t("Loading choices…", "正在加载选项…")}</p>
          )}
          {directory.error ? (
            <div role="alert">
              <p>{directory.error}</p>
              <button type="button" onClick={directory.retry}>
                {t("Retry", "重试")}
              </button>
            </div>
          ) : (
            !searching &&
            !matches.length && (
              <p>
                {namespace && query.trim().length < 2
                  ? t(
                      "Type at least two characters to find a user.",
                      "输入至少两个字符查找用户。",
                    )
                  : t("No matches", "没有匹配结果")}
              </p>
            )
          )}
        </div>
      )}
    </div>
  );
}
