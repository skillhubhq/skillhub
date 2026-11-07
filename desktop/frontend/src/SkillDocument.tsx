import { t } from "./i18n";
import { useState } from "react";
import Markdown from "react-markdown";
import remarkGfm from "remark-gfm";

// 阅读视图不载入图片或打开包内链接；原文始终可检查，HTML 保持文本。
export function SkillDocument({
  text,
  controls = true,
}: {
  text: string;
  controls?: boolean;
}) {
  const [raw, setRaw] = useState(false);
  const body = text.replace(/^---\r?\n[\s\S]*?\r?\n---(?:\r?\n|$)/, "");
  const large = text.length > 65536;
  return (
    <div className="skill-document">
      {controls && (
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
      )}
      {raw || large ? (
        <pre className="markdown">{text}</pre>
      ) : (
        <div className="document-prose">
          <Markdown
            remarkPlugins={[remarkGfm]}
            components={{
              a: ({ children }) => (
                <span className="document-reference">{children}</span>
              ),
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
