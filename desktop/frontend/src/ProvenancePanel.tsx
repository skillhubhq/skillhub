import { t } from "./i18n";
import { ProjectLink } from "./Downloads";
import { Field } from "./ui";
import {
  safeProvenanceURL,
  emptyProvenance,
  type Provenance,
} from "./provenance";
import type { ReactNode } from "react";
function SourceLink({ href, children }: { href: string; children: ReactNode }) {
  const safe = safeProvenanceURL(href);
  return safe ? (
    <ProjectLink href={safe}>{children}</ProjectLink>
  ) : (
    <span>{children}</span>
  );
}
export function UpstreamAuthor({ value }: { value: Provenance }) {
  return (
    <span className="upstream-author">
      <SourceLink href={value.author_url}>{value.author_name}</SourceLink>
      <small>{t("Community curated", "社区收录")}</small>
    </span>
  );
}
export function ProvenanceDetails({
  value,
  version,
  digest,
  reviewNote,
}: {
  value?: Provenance | null;
  version?: string;
  digest?: string;
  reviewNote?: string;
}) {
  if (!value) return null;
  return (
    <section
      className="provenance-details"
      aria-label={t("Source and attribution", "来源与署名")}
    >
      <span className="small-label">{t("Original author", "原作者")}</span>
      <UpstreamAuthor value={value} />
      <dl>
        <dt>{t("Upstream", "上游来源")}</dt>
        <dd>
          <SourceLink href={value.source_url}>
            {t("View original files ↗", "查看原始文件 ↗")}
          </SourceLink>
        </dd>
        <dt>{t("License", "许可证")}</dt>
        <dd>
          {value.license}
          <small>{value.license_file}</small>
        </dd>
        {version && (
          <>
            <dt>{t("Hub version", "收录版本")}</dt>
            <dd>{version}</dd>
          </>
        )}
        <dt>{t("Upstream commit", "上游提交")}</dt>
        <dd>
          <code title={value.revision}>{value.revision.slice(0, 12)}</code>
        </dd>
        <dt>{t("Changes", "内容改动")}</dt>
        <dd>
          {value.modified
            ? value.changes
            : t("Unmodified upstream files", "保留上游原始文件")}
        </dd>
      </dl>
      {reviewNote && (
        <details className="provenance-proof">
          <summary>{t("Curator review notes", "收录审核说明")}</summary>
          <p>{reviewNote}</p>
        </details>
      )}
      <details className="provenance-proof">
        <summary>{t("Verification details", "核对信息")}</summary>
        <dl>
          <dt>{t("Full commit", "完整提交")}</dt>
          <dd>
            <code>{value.revision}</code>
          </dd>
          {digest && (
            <>
              <dt>{t("Package SHA-256", "包 SHA-256")}</dt>
              <dd>
                <code>{digest}</code>
              </dd>
            </>
          )}
        </dl>
        <p>
          {t(
            "Attribution is declared by the curator and reviewed for this version. It does not imply upstream endorsement.",
            "来源由收录维护者声明并随版本审核，不代表上游作者认可或参与此次收录。",
          )}
        </p>
      </details>
    </section>
  );
}
export function ProvenanceEditor({
  value,
  onChange,
}: {
  value: Provenance | null;
  onChange: (p: Provenance | null) => void;
}) {
  const field = (
    key: keyof Provenance,
    label: string,
    placeholder: string,
    maxLength = 2048,
  ) => (
    <Field label={label}>
      <input
        required
        value={String(value?.[key] ?? "")}
        maxLength={maxLength}
        placeholder={placeholder}
        onChange={(e) => onChange({ ...value!, [key]: e.target.value })}
      />
    </Field>
  );
  return (
    <section className="provenance-editor full">
      <label className="check">
        <input
          type="checkbox"
          checked={!!value}
          onChange={(e) =>
            onChange(e.target.checked ? emptyProvenance() : null)
          }
        />
        {t("Curate a skill from an upstream repository", "从上游仓库收录技能")}
      </label>
      {value && (
        <>
          <p className="muted">
            {t(
              "Keep upstream files and license notices. Your publishing space maintains this copy; your account records the submission. Every curated version requires independent review.",
              "保留上游文件和许可证声明。发布空间负责维护收录内容，提交操作由你的账号记录。每个收录版本都须独立审核。",
            )}
          </p>
          <div className="form-grid">
            {field(
              "author_name",
              t("Original author", "原作者"),
              "Original author or organization",
              120,
            )}
            {field(
              "author_url",
              t("Author homepage", "作者主页"),
              "https://github.com/author",
            )}
            {field(
              "source_url",
              t("Pinned source URL", "固定版本的来源地址"),
              "https://github.com/owner/repo/tree/commit/skill",
            )}
            {field(
              "revision",
              t("Full upstream commit", "上游完整 commit"),
              "40 or 64 hexadecimal characters",
              64,
            )}
            {field(
              "license",
              t("License identifier", "许可证标识"),
              "Apache-2.0",
              80,
            )}
            {field(
              "license_file",
              t("License file in package", "包内许可证文件"),
              "LICENSE.txt",
              512,
            )}
            <label className="check full">
              <input
                type="checkbox"
                checked={value.modified}
                onChange={(e) =>
                  onChange({
                    ...value,
                    modified: e.target.checked,
                    changes: e.target.checked ? value.changes : "",
                  })
                }
              />
              {t("I modified the upstream files", "我修改了上游文件")}
            </label>
            {value.modified && (
              <Field label={t("Describe the changes", "说明改动")}>
                <textarea
                  required
                  value={value.changes}
                  maxLength={1000}
                  onChange={(e) =>
                    onChange({ ...value, changes: e.target.value })
                  }
                />
              </Field>
            )}
          </div>
        </>
      )}
    </section>
  );
}
