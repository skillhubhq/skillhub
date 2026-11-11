import { diffLines } from "./versionDiff";
import { useState } from "react";
import { API, Field, Modal, useUI } from "./ui";
import { useResource, ResourceState } from "./community-ui";
import { Select } from "./Select";
import { t } from "./i18n";
import "./distribution.css";

export function VersionComparison({
  coordinate,
  release,
}: {
  coordinate: string;
  release: { id: string; version: string };
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>
        {t("Compare versions", "比较版本")}
      </button>
      {open && (
        <ComparisonDialog
          key={release.id}
          coordinate={coordinate}
          release={release}
          close={() => setOpen(false)}
        />
      )}
    </>
  );
}
function ComparisonDialog({
  coordinate,
  release,
  close,
}: {
  coordinate: string;
  release: { id: string; version: string };
  close: () => void;
}) {
  const ui = useUI();
  const [before, setBefore] = useState("");
  const versions = useResource(
    () => API.Releases(coordinate, ""),
    [coordinate],
  );
  const choices =
    versions.value?.releases?.filter((v) => v.id !== release.id) ?? [];
  const baseline = before || choices[0]?.id || "";
  const difference = useResource(
    () =>
      baseline
        ? API.CompareReleases(coordinate, baseline, release.id)
        : Promise.resolve(null),
    [coordinate, baseline, release.id],
  );
  return (
    <Modal
      title={t("Compare versions", "比较版本")}
      onClose={close}
      stable
      wide
    >
      <div className="stack version-comparison">
        <ResourceState {...versions} retry={versions.reload}>
          {choices.length ? (
            <Field label={t("Compare with", "基线版本")}>
              <Select
                value={baseline}
                onChange={(e) => setBefore(e.target.value)}
              >
                {choices.map((v) => (
                  <option key={v.id} value={v.id}>
                    {v.version}
                  </option>
                ))}
              </Select>
            </Field>
          ) : (
            <p>
              {t(
                "No other published version is available.",
                "暂无其他可比较的已发布版本。",
              )}
            </p>
          )}
          {versions.value?.next_page && (
            <button
              onClick={() =>
                void ui.run(async () => {
                  const p = await API.Releases(
                    coordinate,
                    versions.value!.next_page,
                  );
                  versions.setValue({
                    ...p,
                    releases: [
                      ...(versions.value!.releases ?? []),
                      ...(p.releases ?? []),
                    ],
                  });
                })
              }
            >
              {t("More versions")}
            </button>
          )}
        </ResourceState>
        {baseline && (
          <ResourceState {...difference} retry={difference.reload}>
            <p>
              {choices.find((v) => v.id === baseline)?.version} →{" "}
              {release.version} · {difference.value?.files?.length ?? 0}{" "}
              {t("changed files", "个文件有变化")}
            </p>
            {difference.value?.files?.length === 0 && (
              <p>{t("Package files are identical.", "包内文件内容一致。")}</p>
            )}
            {difference.value?.files?.map((file) => (
              <details
                key={file.path}
                className="version-file"
                open={file.path === "SKILL.md"}
              >
                <summary>
                  {file.path}{" "}
                  <span>
                    {file.change === "added"
                      ? t("Added", "新增")
                      : file.change === "removed"
                        ? t("Removed", "删除")
                        : t("Modified", "修改")}
                  </span>
                </summary>
                {file.before_executable !== file.after_executable && (
                  <p>
                    {t("Executable permission changed", "可执行权限已变化")}
                  </p>
                )}
                {file.text_available ? (
                  <pre
                    className="version-unified"
                    tabIndex={0}
                    aria-label={t("File changes", "文件差异")}
                  >
                    {diffLines(file.before_text, file.after_text).map(
                      (line, index) => (
                        <span key={index} className={`diff-${line.kind}`}>
                          <span aria-hidden="true">
                            {line.kind === "added"
                              ? "+ "
                              : line.kind === "removed"
                                ? "− "
                                : "  "}
                          </span>
                          {line.text}
                        </span>
                      ),
                    )}
                  </pre>
                ) : (
                  <p>
                    {t(
                      "Binary or large file. Inspect it in the Files view.",
                      "二进制或较大的文件，请在文件视图中查看。",
                    )}
                  </p>
                )}
              </details>
            ))}
          </ResourceState>
        )}
      </div>
    </Modal>
  );
}
