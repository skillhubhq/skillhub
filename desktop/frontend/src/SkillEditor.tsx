import { useEffect, useState } from "react";
import { Modal, useUI } from "./ui";
import { SkillDocument } from "./SkillDocument";
import { isWeb } from "./platform";
import { t } from "./i18n";
type TextFile = { text: string; digest: string };
export function SkillEditor({
  files,
  read,
  save,
  onClose,
}: {
  files: string[];
  read: (path: string) => Promise<TextFile>;
  save: (path: string, digest: string, text: string) => Promise<TextFile>;
  onClose: () => void;
}) {
  const ui = useUI();
  const [path, setPath] = useState("SKILL.md"),
    [original, setOriginal] = useState<TextFile | null>(null),
    [text, setText] = useState(""),
    [preview, setPreview] = useState(false),
    [discard, setDiscard] = useState(false),
    [error, setError] = useState("");
  const dirty = !!original && original.text !== text;
  useEffect(() => {
    let active = true;
    setOriginal(null);
    setError("");
    void read(path)
      .then((v) => {
        if (active) {
          setOriginal(v);
          setText(v.text);
        }
      })
      .catch(() => {
        if (active)
          setError(
            t(
              "This file cannot be edited. Choose a UTF-8 text file up to 1 MiB.",
              "此文件无法编辑，请选择不超过 1 MiB 的 UTF-8 文本文件。",
            ),
          );
      });
    return () => {
      active = false;
    };
  }, [path]);
  return (
    <Modal
      wide
      title={t("Edit skill draft", "编辑技能草稿")}
      onClose={() => (dirty ? setDiscard(true) : onClose())}
    >
      <p className="muted">
        {t(
          isWeb
            ? "This draft stays in this page until submitted. Keep your original ZIP before leaving."
            : "Edits are saved to a local draft copy. Publish a new version when it is ready.",
          isWeb
            ? "此草稿在提交前仅保留于当前页面，离开前请保留原始 ZIP。"
            : "修改保存在本地草稿副本中，完成后可提交新版本。",
        )}
      </p>
      {discard && (
        <div className="notice">
          <p>{t("Discard unsaved changes?", "放弃尚未保存的修改？")}</p>
          <button onClick={onClose}>
            {t("Discard and close", "放弃并关闭")}
          </button>
          <button onClick={() => setDiscard(false)}>
            {t("Keep editing", "继续编辑")}
          </button>
        </div>
      )}
      <div className="skill-editor-layout">
        <nav aria-label={t("Draft files", "草稿文件")}>
          {files.map((file) => (
            <button
              key={file}
              disabled={dirty || ui.busy}
              aria-current={file === path ? "page" : undefined}
              onClick={() => {
                setPath(file);
                setPreview(false);
              }}
              title={file}
            >
              {file}
            </button>
          ))}
        </nav>
        <section>
          <div className="section-heading">
            <code>{path}</code>
            <div className="content-tabs">
              <button aria-pressed={!preview} onClick={() => setPreview(false)}>
                {t("Edit", "编辑")}
              </button>
              <button
                disabled={!original || !path.endsWith(".md")}
                aria-pressed={preview}
                onClick={() => setPreview(true)}
              >
                {t("Preview", "预览")}
              </button>
            </div>
          </div>
          {error ? (
            <p role="alert">{error}</p>
          ) : original ? (
            preview ? (
              <div className="editor-preview">
                <SkillDocument text={text} controls={false} />
              </div>
            ) : (
              <textarea
                className="skill-text-editor"
                aria-label={t("File content", "文件内容")}
                spellCheck={false}
                value={text}
                onChange={(e) => setText(e.target.value)}
              />
            )
          ) : (
            <p role="status">{t("Loading…", "加载中…")}</p>
          )}
        </section>
      </div>
      <div className="actions">
        <span role="status" className="muted">
          {dirty
            ? t(
                "Unsaved changes · save before switching files",
                "有未保存修改，请保存后切换文件",
              )
            : t("Changes saved to this draft", "修改已保存到此草稿")}
        </span>
        <button
          disabled={!dirty || ui.busy}
          onClick={() => {
            if (original) setText(original.text);
          }}
        >
          {t("Discard changes", "放弃修改")}
        </button>
        <button
          className="primary"
          disabled={!dirty || ui.busy}
          onClick={() =>
            void ui.run(async () => {
              const result = await save(path, original!.digest, text);
              setOriginal(result);
            })
          }
        >
          {t("Save draft", "保存草稿")}
        </button>
      </div>
    </Modal>
  );
}
