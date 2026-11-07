import { useState } from "react";
import { API, Modal, useUI } from "./ui";
import { useResource, useMutationKey } from "./community-ui";
import { dateLabel, t } from "./i18n";
import "./evaluation.css";

type Report = {
  suite: {
    name: string;
    version: string;
    cases: { id: string; input: string; expected: string }[];
  };
  skill_digest: string;
  agent: string;
  model: string;
  runner: string;
  settings: string;
  executed_at: string;
  results: {
    case_id: string;
    status: string;
    observed: string;
    baseline?: { status: string; observed: string };
  }[];
};
const outcome = (s: string) =>
  ({
    pass: t("Pass", "通过"),
    fail: t("Fail", "失败"),
    error: t("Error", "错误"),
    skipped: t("Skipped", "跳过"),
  })[s] || s;
export function EvaluationPanel({
  source,
  release,
  digest,
  compact = false,
}: {
  source: string;
  release: string;
  digest: string;
  compact?: boolean;
}) {
  const ui = useUI();
  const page = useResource(
    () => API.Evaluations(source, release),
    [source, release, ui.revision],
  );
  const [importing, setImporting] = useState(false),
    [selected, setSelected] = useState("");
  const list = page.value;
  const content = (
    <>
      <div className="evaluation-heading">
        <div>
          <h3>{t("Evaluation evidence", "评测证据")}</h3>
          <p>
            {t(
              "External results for this version. SkillHub has not run or independently verified them.",
              "当前版本的外部执行结果，SkillHub 未运行或独立验证。",
            )}
          </p>
        </div>
        {list?.can_manage && (
          <button onClick={() => setImporting(true)}>
            {t("Import results", "导入结果")}
          </button>
        )}
      </div>
      {page.error ? (
        <div role="alert">
          <p>{t("Evidence could not be loaded.", "评测证据加载失败。")}</p>
          <button onClick={page.reload}>{t("Retry", "重试")}</button>
        </div>
      ) : page.loading ? (
        <p>{t("Loading evidence…", "正在加载评测证据…")}</p>
      ) : !list?.supported ? (
        <p>
          {t(
            "This Hub does not support evaluation evidence.",
            "当前 Hub 不支持评测证据。",
          )}
        </p>
      ) : !list.evidence?.length ? (
        <p className="evaluation-empty">
          {t(
            "No evaluation evidence yet. A static scan is not a behavioral evaluation.",
            "尚无评测证据。静态扫描不代表技能效果评测。",
          )}
        </p>
      ) : (
        <div className="evaluation-list">
          {(list.evidence ?? []).map((e) => (
            <button
              className="evaluation-row"
              key={e.id}
              onClick={() => setSelected(e.id)}
            >
              <span>
                <strong>
                  {e.retracted
                    ? t("Retracted report", "已撤回的报告")
                    : `${e.suite_name} · ${e.suite_version}`}
                </strong>
                <small>
                  {e.retracted
                    ? dateLabel(e.imported_at)
                    : `${e.agent} · ${e.model}`}
                </small>
              </span>
              {!e.retracted && (
                <span className="evaluation-score">
                  <strong>
                    {e.passed}/{e.passed + e.failed + e.errors + e.skipped}{" "}
                    {t("passed", "通过")}
                  </strong>
                  <small>
                    {e.failed} {t("failed", "失败")} · {e.errors}{" "}
                    {t("errors", "错误")} · {e.skipped} {t("skipped", "跳过")}
                  </small>
                </span>
              )}
              <span aria-hidden="true">→</span>
            </button>
          ))}
        </div>
      )}
    </>
  );
  return (
    <section
      className={compact ? "evaluation-panel compact" : "evaluation-panel"}
      aria-label={t("Evaluation evidence", "评测证据")}
    >
      {compact ? (
        <details>
          <summary>
            {t("Evaluation evidence", "评测证据")}{" "}
            {list?.supported
              ? `· ${(list.evidence ?? []).filter((e) => !e.retracted).length}`
              : ""}
          </summary>
          {content}
        </details>
      ) : (
        content
      )}
      {importing && (
        <ImportDialog
          source={source}
          release={release}
          digest={digest}
          close={() => setImporting(false)}
          done={page.reload}
        />
      )}
      {selected && (
        <EvidenceDialog
          key={selected}
          source={source}
          id={selected}
          manage={!!list?.can_manage}
          close={() => setSelected("")}
          done={page.reload}
        />
      )}
    </section>
  );
}
function ImportDialog({
  source,
  release,
  digest,
  close,
  done,
}: {
  source: string;
  release: string;
  digest: string;
  close: () => void;
  done: () => void;
}) {
  const [document, setDocument] = useState(""),
    [report, setReport] = useState<Report | null>(null),
    [error, setError] = useState(""),
    [consent, setConsent] = useState(false),
    [busy, setBusy] = useState(false);
  const key = useMutationKey();
  async function choose(file?: File) {
    setError("");
    setReport(null);
    setDocument("");
    setConsent(false);
    if (!file) return;
    try {
      if (file.size > 256 * 1024)
        throw new Error(
          t(
            "Choose a JSON report no larger than 256 KiB.",
            "请选择不超过 256 KiB 的 JSON 报告。",
          ),
        );
      const text = await file.text();
      const r = JSON.parse(text);
      if (
        !r ||
        r.schema_version !== "skillhub.evaluation.v1" ||
        r.skill_digest !== digest ||
        typeof r.suite?.name !== "string" ||
        typeof r.suite?.version !== "string" ||
        !Array.isArray(r.suite?.cases) ||
        !Array.isArray(r.results) ||
        typeof r.agent !== "string" ||
        typeof r.model !== "string"
      )
        throw new Error(
          t(
            "The report format or skill digest does not match this version.",
            "报告格式或技能摘要与当前版本不匹配。",
          ),
        );
      setDocument(text);
      setReport(r);
    } catch (e) {
      setError(
        e instanceof SyntaxError
          ? t("Invalid JSON report.", "JSON 报告无效。")
          : String((e as Error).message),
      );
    }
  }
  return (
    <Modal
      title={t("Import evaluation results", "导入评测结果")}
      onClose={close}
    >
      <div className="evaluation-import">
        <p>
          {t(
            "Run tests in your chosen agent, then import a sanitized skillhub.evaluation.v1 JSON file. No agent or model runs here.",
            "在你选择的 Agent 中执行测试，再导入脱敏后的 skillhub.evaluation.v1 JSON 文件。这里不会运行 Agent 或模型。",
          )}
        </p>
        <label className="file-picker">
          {t("Choose JSON report", "选择 JSON 报告")}
          <input
            type="file"
            accept=".json,application/json"
            disabled={busy}
            onChange={(e) => void choose(e.target.files?.[0])}
          />
        </label>
        {report && (
          <p>
            {report.suite.name} · {report.suite.version}
            <br />
            {report.agent} · {report.model}
            <br />
            {report.results.length} {t("case results", "条用例结果")}
          </p>
        )}
        <label className="evaluation-consent">
          <input
            type="checkbox"
            checked={consent}
            disabled={!report || busy}
            onChange={(e) => setConsent(e.target.checked)}
          />
          {t(
            "I have removed private data and credentials. Anyone who can read this skill version can read this report, including anonymous visitors if it is public.",
            "我已移除私有数据和凭据。可阅读该技能版本的人也能阅读此报告；如果版本公开，则包括匿名访问者。",
          )}
        </label>
        {error && <p role="alert">{error}</p>}
        <div className="actions">
          <button disabled={busy} onClick={close}>
            {t("Cancel", "取消")}
          </button>
          <button
            className="primary"
            disabled={!report || !consent || busy}
            onClick={async () => {
              setBusy(true);
              setError("");
              try {
                await API.ImportEvaluation(
                  source,
                  release,
                  document,
                  await key({ source, release, document }),
                );
                done();
                close();
              } catch {
                setError(
                  t(
                    "Import failed. Check the document specification, version digest and your current permissions. You can retry safely.",
                    "导入失败。请检查文档规范、版本摘要和当前权限，可安全重试。",
                  ),
                );
              } finally {
                setBusy(false);
              }
            }}
          >
            {busy
              ? t("Importing…", "正在导入…")
              : t("Import results", "导入结果")}
          </button>
        </div>
      </div>
    </Modal>
  );
}
function EvidenceDialog({
  source,
  id,
  manage,
  close,
  done,
}: {
  source: string;
  id: string;
  manage: boolean;
  close: () => void;
  done: () => void;
}) {
  const resource = useResource(() => API.Evaluation(source, id), [source, id]);
  const [confirm, setConfirm] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const key = useMutationKey();
  const e = resource.value;
  let report: Report | null = null;
  try {
    if (e?.document_json) report = JSON.parse(e.document_json);
  } catch {}
  return (
    <Modal
      stable
      title={t("Evaluation report", "评测报告")}
      onClose={close}
      wide
    >
      {resource.error ? (
        <div role="alert">
          <p>
            {t(
              "Evidence is unavailable or access has changed.",
              "评测证据不可用或访问权限已变化。",
            )}
          </p>
          <button onClick={resource.reload}>{t("Retry", "重试")}</button>
        </div>
      ) : !e ? (
        <p>{t("Loading…", "正在加载…")}</p>
      ) : e.retracted ? (
        <p>
          {t(
            "This report was retracted. Its content is no longer available.",
            "此报告已撤回，正文已不再提供。",
          )}
        </p>
      ) : (
        report && (
          <div className="evaluation-report">
            <p className="evaluation-notice">
              {t(
                "Externally reported · Not independently verified by SkillHub",
                "外部提供 · 未经 SkillHub 独立验证",
              )}
            </p>
            <h3>
              {e.suite_name} · {e.suite_version}
            </h3>
            <p>
              {e.agent} · {e.model} · {e.runner}
            </p>
            <p>
              {t("Executed", "执行于")} {dateLabel(e.executed_at)} ·{" "}
              {t("Imported", "导入于")} {dateLabel(e.imported_at)}
            </p>
            <div className="evaluation-totals">
              <strong>
                {e.passed}/{e.passed + e.failed + e.errors + e.skipped}{" "}
                {t("passed", "通过")}
              </strong>
              <span>
                {e.failed} {t("failed", "失败")} · {e.errors}{" "}
                {t("errors", "错误")} · {e.skipped} {t("skipped", "跳过")}
              </span>
              {e.baseline_total > 0 && (
                <span>
                  {t("Without skill", "未使用技能")}: {e.baseline_passed}/
                  {e.baseline_total} {t("passed", "通过")}
                </span>
              )}
            </div>
            <details>
              <summary>{t("Reproduction details", "复现信息")}</summary>
              <dl>
                <dt>{t("Settings", "执行配置")}</dt>
                <dd>
                  <pre>{report.settings}</pre>
                </dd>
                <dt>{t("Skill digest", "技能摘要")}</dt>
                <dd>{report.skill_digest}</dd>
                <dt>{t("Suite digest", "用例集摘要")}</dt>
                <dd>{e.suite_digest}</dd>
                <dt>{t("Report digest", "报告摘要")}</dt>
                <dd>{e.digest}</dd>
              </dl>
            </details>
            <div className="evaluation-cases">
              {report.suite.cases.map((c) => {
                const r = report!.results.find((r) => r.case_id === c.id)!;
                return (
                  <details key={c.id}>
                    <summary>
                      <strong>{c.id}</strong>
                      <span className={`outcome-${r.status}`}>
                        {outcome(r.status)}
                      </span>
                    </summary>
                    <dl>
                      <dt>{t("Input", "输入")}</dt>
                      <dd>
                        <pre>{c.input}</pre>
                      </dd>
                      <dt>{t("Expected", "预期")}</dt>
                      <dd>
                        <pre>{c.expected}</pre>
                      </dd>
                      <dt>{t("Observed", "实际结果")}</dt>
                      <dd>
                        <pre>{r.observed}</pre>
                      </dd>
                      {r.baseline && (
                        <>
                          <dt>
                            {t("Without skill", "未使用技能")} ·{" "}
                            {outcome(r.baseline.status)}
                          </dt>
                          <dd>
                            <pre>{r.baseline.observed}</pre>
                          </dd>
                        </>
                      )}
                    </dl>
                  </details>
                );
              })}
            </div>
            {manage && (
              <div className="evaluation-retract">
                {confirm ? (
                  <>
                    <p>
                      {t(
                        "Retract this report? Its content will be removed. The audit digest remains. Existing backups and previously downloaded copies are unaffected.",
                        "撤回这份报告？正文将被移除，保留审计摘要；既有备份和此前下载的副本不会受影响。",
                      )}
                    </p>
                    <button disabled={busy} onClick={() => setConfirm(false)}>
                      {t("Cancel", "取消")}
                    </button>
                    <button
                      disabled={busy}
                      onClick={async () => {
                        setBusy(true);
                        try {
                          await API.RetractEvaluation(
                            source,
                            id,
                            await key({ source, id }),
                          );
                          done();
                          resource.reload();
                        } catch {
                          setError(
                            t("Could not retract report.", "报告撤回失败。"),
                          );
                        } finally {
                          setBusy(false);
                        }
                      }}
                    >
                      {t("Confirm retraction", "确认撤回")}
                    </button>
                  </>
                ) : (
                  <button onClick={() => setConfirm(true)}>
                    {t("Retract report", "撤回报告")}
                  </button>
                )}
              </div>
            )}
            {error && <p role="alert">{error}</p>}
          </div>
        )
      )}
    </Modal>
  );
}
