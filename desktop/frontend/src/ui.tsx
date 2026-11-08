import { lockModalScroll } from "./modalScroll";
import { Select } from "./Select";
import { t } from "./i18n";
import {
  NamespaceRole as Role,
  ReleaseStatus as Status,
  Visibility,
} from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";
import {
  createContext,
  useContext,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { API } from "./platform";
import { SkillDocument } from "./SkillDocument";
import { headingAnchor } from "./documentLinks";
import type { ChangePlan } from "../bindings/github.com/skillhubhq/skillhub/desktop/models";
import type {
  PreviewContent,
  ReleasePreview,
  Source,
} from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
export { API };
export type { Source };
export type UI = {
  busy: boolean;
  error: string;
  revision: number;
  sources: Source[];
  activeSource: string;
  access?: "checking" | "member" | "guest" | "unavailable";
  run: <T>(task: () => Promise<T>) => Promise<T | undefined>;
  changed: (message: string) => void;
  refresh: () => void;
  plan: (task: () => Promise<ChangePlan>) => void;
  preview: (task: () => Promise<ReleasePreview>) => void;
  confirm: (
    title: string,
    detail: string,
    action: () => Promise<unknown>,
    tone?: "primary" | "danger",
  ) => void;
  navigate: (page: string) => void;
};
export const UIContext = createContext<UI>(null!);
export const useUI = () => useContext(UIContext);
export const splitList = (value: string) =>
  value
    .split(/[,，]/)
    .map((v) => v.trim())
    .filter(Boolean);
const statusLabels: Partial<Record<Status, string>> = {
  [Status.ReleaseStatus_RELEASE_STATUS_PENDING]: "In review",
  [Status.ReleaseStatus_RELEASE_STATUS_PUBLISHED]: "Published",
  [Status.ReleaseStatus_RELEASE_STATUS_REJECTED]: "Changes requested",
  [Status.ReleaseStatus_RELEASE_STATUS_YANKED]: "Withdrawn",
};
const visibilityLabels: Partial<Record<Visibility, string>> = {
  [Visibility.Visibility_VISIBILITY_PUBLIC]: "Public",
  [Visibility.Visibility_VISIBILITY_MEMBERS]: "Hub members",
  [Visibility.Visibility_VISIBILITY_GROUPS]: "Access groups",
};
const roleLabels: Partial<Record<Role, string>> = {
  [Role.NamespaceRole_NAMESPACE_ROLE_READER]: "Reader",
  [Role.NamespaceRole_NAMESPACE_ROLE_MAINTAINER]: "Maintainer",
  [Role.NamespaceRole_NAMESPACE_ROLE_REVIEWER]: "Reviewer",
  [Role.NamespaceRole_NAMESPACE_ROLE_ADMIN]: "Admin",
};
export const statusLabel = (value: Status) =>
  t(statusLabels[value] ?? "Unknown status");
export const visibilityLabel = (value: Visibility) =>
  t(visibilityLabels[value] ?? "Unknown audience");
export const roleLabel = (value: Role) =>
  t(roleLabels[value] ?? "Unknown role");
export const toolLabel = (value: string) =>
  value === "codex" ? "Codex" : "Claude Code";
export function Field({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
      {hint && <small>{hint}</small>}
    </label>
  );
}
export function Empty({
  title,
  children,
}: {
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">◇</div>
      <h2>{title}</h2>
      <p>{children}</p>
    </div>
  );
}
export function SourceSelect({
  value,
  onChange,
  all = false,
}: {
  value: string;
  onChange: (v: string) => void;
  all?: boolean;
}) {
  const { sources } = useUI();
  return (
    <Select
      aria-label={t("Choose hub")}
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      <option value="">
        {all ? t("All connected hubs") : t("Choose a hub")}
      </option>
      {sources.map((s) => (
        <option key={s.alias} value={s.alias}>
          {s.label || s.display_name} · {s.alias}
        </option>
      ))}
    </Select>
  );
}
export function ToolSelect({
  value,
  onChange,
}: {
  value: string;
  onChange: (v: string) => void;
}) {
  return (
    <Select
      aria-label={t("Tool")}
      value={value}
      onChange={(e) => onChange(e.target.value)}
    >
      <option value="codex">Codex</option>
      <option value="claude-code">Claude Code</option>
    </Select>
  );
}
export function DirectoryField({
  value,
  onChange,
  label = t("Project folder"),
  required = false,
}: {
  value: string;
  onChange: (v: string) => void;
  label?: string;
  required?: boolean;
}) {
  const ui = useUI();
  return (
    <Field label={label}>
      <div className="input-action">
        <input
          aria-label={label}
          value={value}
          required={required}
          placeholder={t("Choose or enter a folder path")}
          onChange={(e) => onChange(e.target.value)}
        />
        <button
          type="button"
          aria-label={`${t("Browse", "浏览")} ${label}`}
          disabled={ui.busy}
          onClick={() =>
            void ui.run(async () => {
              const path = await API.ChooseDirectory();
              if (path) onChange(path);
            })
          }
        >
          {t("Browse…")}
        </button>
      </div>
    </Field>
  );
}
export function Modal({
  title,
  children,
  onClose,
  wide = false,
  stable = false,
  toolbar,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
  wide?: boolean;
  stable?: boolean | "compact";
  toolbar?: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const id = useId();
  const { busy, error } = useUI();
  useLayoutEffect(() => {
    const dialog = ref.current!;
    const trigger = document.activeElement;
    const unlock = lockModalScroll();
    dialog.showModal();
    return () => {
      // 在 React 移除 dialog 前关闭，避免 Chromium 丢失原始焦点目标。
      dialog.close();
      // 等待节点移除和顶层模态状态解除，再恢复滚动与焦点。
      queueMicrotask(() => {
        if (
          !dialog.open &&
          trigger instanceof HTMLElement &&
          trigger.isConnected
        )
          trigger.focus({ preventScroll: true });
        unlock();
      });
    };
  }, []);
  return (
    <dialog
      ref={ref}
      className={`app-modal${wide ? " wide" : ""}${stable ? " modal-stable" : ""}${stable === "compact" ? " modal-compact" : ""}`}
      aria-labelledby={id}
      onCancel={(e) => {
        e.preventDefault();
        e.stopPropagation();
        if (!busy) onClose();
      }}
    >
      <div className="modal-head">
        <h2 id={id}>{title}</h2>
        <button
          type="button"
          aria-label={t("Close dialog")}
          disabled={busy}
          onClick={onClose}
        >
          ×
        </button>
      </div>
      {toolbar && <div className="modal-toolbar">{toolbar}</div>}
      <div className="modal-body">
        {error && (
          <div className="notice error" role="alert">
            {error}
          </div>
        )}
        <fieldset disabled={busy}>{children}</fieldset>
      </div>
    </dialog>
  );
}
export function PreviewBody({
  preview,
  view = "all",
}: {
  preview: ReleasePreview;
  view?: "all" | "readme" | "files";
}) {
  const ui = useUI();
  const [file, setFile] = useState<PreviewContent | null>(null);
  const content = useRef<HTMLDivElement>(null);
  const [anchor, setAnchor] = useState("");
  const documentRequest = useRef(0);
  const openDocument = (path: string, fragment: string) => {
    const request = ++documentRequest.current;
    void ui.run(async () => {
      if (!(preview.files ?? []).some((f) => f.path === path))
        throw new Error(
          t("Linked file is not in this package.", "链接文件不在此技能包中。"),
        );
      const value = await API.ReadPreviewFile(preview.target, path);
      if (request !== documentRequest.current) return;
      setAnchor(fragment);
      setFile(value);
    });
  };
  useEffect(() => {
    documentRequest.current++;
    setFile(null);
    return () => {
      documentRequest.current++;
    };
  }, [preview.target.source, preview.release.id, preview.release.digest]);
  useEffect(() => {
    if (file) {
      const heading =
        anchor &&
        Array.from(
          content.current?.querySelectorAll("h1,h2,h3,h4,h5,h6") || [],
        ).find((h) => headingAnchor(h.textContent || "") === anchor);
      (heading || content.current)?.scrollIntoView({ block: "nearest" });
    }
  }, [file]);
  const r = preview.release;
  return (
    <>
      {view === "all" && (
        <div className="chips">
          <span>
            {r.namespace}/{r.name}
          </span>
          <span>v{r.version}</span>
          <span>{statusLabel(r.status)}</span>
          <span>{visibilityLabel(r.visibility)}</span>
        </div>
      )}
      {view !== "readme" && (
        <details className="integrity-details">
          <summary>{t("Package identity", "包身份信息")}</summary>
          <p className="path">{r.digest}</p>
        </details>
      )}
      {view === "all" && (r.groups ?? []).length > 0 && (
        <p className="muted">
          {t("Audience groups:")} {(r.groups ?? []).join(", ")}
        </p>
      )}
      {view !== "files" && !file && (
        <SkillDocument key={r.id} text={r.markdown} onNavigate={openDocument} />
      )}
      {file && (
        <div ref={content} className="package-document">
          <button
            onClick={() => {
              documentRequest.current++;
              setFile(null);
              setAnchor("");
            }}
          >
            {t("Back to overview", "返回概览")}
          </button>
          <h3>{file.path}</h3>
          {file.text ? (
            /\.md$/i.test(file.path) ? (
              <SkillDocument
                key={file.path}
                text={file.content}
                path={file.path}
                onNavigate={openDocument}
              />
            ) : (
              <pre className="markdown">
                {file.content || t("(empty file)")}
              </pre>
            )
          ) : (
            <p>
              {t(
                "This file cannot be previewed as text.",
                "此文件无法以文本预览。",
              )}
            </p>
          )}
        </div>
      )}
      {view !== "readme" && (
        <details open={view === "files" ? true : undefined}>
          <summary>
            {t("Package files ·")} {(preview.files ?? []).length}
          </summary>
          <ul className="file-list">
            {(preview.files ?? []).map((f) => (
              <li key={f.path}>
                <button
                  type="button"
                  disabled={ui.busy}
                  onClick={() =>
                    void ui.run(async () => {
                      setFile(
                        await API.ReadPreviewFile(preview.target, f.path),
                      );
                    })
                  }
                >
                  <code>{f.path}</code>
                </button>
                <span>
                  {f.size.toLocaleString()} B
                  {f.executable ? t(" · Executable (not run)") : ""}
                </span>
              </li>
            ))}
          </ul>
        </details>
      )}
      {view !== "readme" && r.reason && (
        <p className="notice release-note">
          {t("Review / withdrawal note:")}
          {r.reason}
        </p>
      )}
    </>
  );
}
