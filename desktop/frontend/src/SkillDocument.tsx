import { t } from "./i18n";
import { useState, useRef, type ReactNode } from "react";
import { ProjectLink } from "./Downloads";
import { documentLink, headingAnchor } from "./documentLinks";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";

// 外链交给浏览器；包内引用通过已授权的归档读取，HTML 保持文本。
export function SkillDocument({
  text,
  controls = true,
  actions,
  path = "SKILL.md",
  onNavigate,
}: {
  text: string;
  controls?: boolean;
  actions?: ReactNode;
  path?: string;
  onNavigate?: (path: string, anchor: string) => void;
}) {
  const root = useRef<HTMLDivElement>(null);
  const [raw, setRaw] = useState(false);
  const body = text.replace(/^---\r?\n[\s\S]*?\r?\n---(?:\r?\n|$)/, "");
  const large = text.length > 65536;
  return (
    <div className="skill-document" ref={root}>
      {controls && (
        <div className="document-toolbar">
          <div
            className="document-modes"
            role="group"
            aria-label={t("Document view")}
          >
            <button
              aria-pressed={!raw && !large}
              disabled={large}
              onClick={() => setRaw(false)}
            >
              {t("Read")}
            </button>
            <button aria-pressed={raw || large} onClick={() => setRaw(true)}>
              {t("Source")}
            </button>
          </div>
          {actions && <div className="document-actions">{actions}</div>}
        </div>
      )}
      {raw || large ? (
        <pre className="markdown">{text}</pre>
      ) : (
        <div className="document-prose">
          <Markdown
            remarkPlugins={[remarkGfm]}
            components={{
              a: ({ children, href }) => {
                const link = documentLink(href || "", path);
                if (!link) return <span>{children}</span>;
                if (link.kind === "external")
                  return <ProjectLink href={link.href}>{children}</ProjectLink>;
                if (!onNavigate && link.path !== path)
                  return (
                    <span
                      title={t(
                        "Open the package preview to read this file.",
                        "请在包预览中查看此文件。",
                      )}
                    >
                      {children}
                    </span>
                  );
                return (
                  <a
                    href={href}
                    onClick={(event) => {
                      event.preventDefault();
                      if (link.path === path && link.anchor) {
                        const heading = Array.from(
                          root.current?.querySelectorAll("h1,h2,h3,h4,h5,h6") ||
                            [],
                        ).find(
                          (h) =>
                            headingAnchor(h.textContent || "") === link.anchor,
                        );
                        heading?.scrollIntoView({ block: "nearest" });
                      } else onNavigate?.(link.path, link.anchor);
                    }}
                  >
                    {children}
                  </a>
                );
              },
              img: ({ alt }) => (
                <span className="muted">
                  {t("[Image:")}
                  {alt || t("not loaded")}]
                </span>
              ),
            }}
          >
            {body}
          </Markdown>
        </div>
      )}
    </div>
  );
}
