import { AccessState } from "./AccessState";
import { ProvenanceDetails } from "./ProvenancePanel";
import { EvaluationPanel } from "./EvaluationPanel";
import { ScanPanel } from "./Governance";
import { Select } from "./Select";
import { SkillSymbol, skillTitle } from "./SkillIdentity";
import { dateLabel, t } from "./i18n";
import { Person, useResource } from "./community-ui";
import { useEffect, useRef, useState } from "react";
import type {
  IdentityView,
  ReleasePage,
} from "../bindings/github.com/skillhubhq/skillhub/desktop/models";
import type { ReleasePreview } from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
import { ReleaseStatus as Status } from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";
import {
  API,
  Empty,
  Field,
  PreviewBody,
  statusLabel,
  visibilityLabel,
  useUI,
} from "./ui";
export function ReviewWorkspace({
  source,
  identity,
  mine,
}: {
  source: string;
  identity: IdentityView;
  mine: boolean;
}) {
  const ui = useUI();
  const selectedID = useRef(
    new URLSearchParams(location.hash.split("?")[1] ?? "").get("release") || "",
  );
  const [reload, setReload] = useState(0);
  const [page, setPage] = useState<ReleasePage | null>(null),
    [selected, setSelected] = useState<ReleasePreview | null>(null),
    [reason, setReason] = useState(""),
    [query, setQuery] = useState(""),
    [filter, setFilter] = useState(
      selectedID.current
        ? "all"
        : mine
          ? "active"
          : String(Status.ReleaseStatus_RELEASE_STATUS_PENDING),
    );
  const documentRef = useRef<HTMLElement>(null);
  const [selecting, setSelecting] = useState("");
  const cleanup = useResource(
    () => API.SubmissionCleanupSupported(source),
    [source],
  );
  const effectiveFilter =
    filter === "active" && cleanup.value !== true ? "all" : filter;
  useEffect(() => {
    const followLink = () => {
      const [page, query = ""] = location.hash.slice(1).split("?");
      const params = new URLSearchParams(query),
        id = params.get("release");
      if (
        page !== (mine ? "submissions" : "review") ||
        params.get("source") !== source ||
        !id ||
        id === selectedID.current
      )
        return;
      selectedID.current = id;
      setReason("");
      setSearchText("");
      setQuery("");
      setFilter("all");
      appliedFilter.current = JSON.stringify(["", "all"]);
      setReload((n) => n + 1);
    };
    window.addEventListener("hashchange", followLink);
    return () => window.removeEventListener("hashchange", followLink);
  }, [source, mine]);

  useEffect(() => {
    if (selected) {
      const params = new URLSearchParams(location.hash.split("?")[1] ?? "");
      params.set("release", selected.release.id);
      history.replaceState(
        null,
        "",
        `#${mine ? "submissions" : "review"}?${params}`,
      );
    }
  }, [selected?.release.id]);
  const access = useResource(
    () =>
      selected
        ? API.NamespaceAccess(source, selected.release.namespace)
        : Promise.resolve(null),
    [source, selected?.release.namespace, ui.revision],
  );
  const scan = useResource(
    () =>
      selected
        ? API.ScanReport(source, selected.release.id)
        : Promise.resolve(null),
    [source, selected?.release.id, ui.revision],
  );
  const scanAllowsApproval =
    !!scan.value &&
    (!scan.value.supported ||
      (!!scan.value.engine &&
        scan.value.digest === selected?.release.digest &&
        !scan.value.blocking &&
        ((scan.value.complete && !scan.value.findings?.length) ||
          !!reason.trim())));
  const [loadError, setLoadError] = useState("");
  const [searchText, setSearchText] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => setQuery(searchText), 250);
    return () => clearTimeout(timer);
  }, [searchText]);
  const author = useResource(
    () =>
      selected
        ? API.CommunityGetProfile(source, selected.release.submitted_by)
        : Promise.resolve(null),
    [source, selected?.release.submitted_by],
  );
  const appliedFilter = useRef(JSON.stringify([query, filter]));
  const generation = useRef(0),
    mutation = useRef<{ key: string; id: string } | null>(null);
  const waitingForFilter = filter === "active" && cleanup.loading;
  useEffect(() => {
    if (waitingForFilter) return;
    let active = true;
    setLoadError("");
    const filterKey = JSON.stringify([query, effectiveFilter]);
    const requestGeneration = ++generation.current;
    if (appliedFilter.current !== filterKey) {
      appliedFilter.current = filterKey;
      selectedID.current = "";
      setPage(null);
      setSelected(null);
      setReason("");
    }
    setSelecting("");
    const requestedID = selectedID.current;
    void (async () => {
      try {
        const p = await API.FilterSubmissions(
          source,
          query,
          effectiveFilter === "all" || effectiveFilter === "active"
            ? 0
            : Number(effectiveFilter),
          mine,
          "",
          effectiveFilter === "active",
        );
        if (active) setPage(p);
        const target = requestedID || p.releases?.[0]?.id;
        if (
          active &&
          requestGeneration === generation.current &&
          target &&
          selectedID.current === requestedID
        ) {
          selectedID.current = target;
          setSelecting(target);
          const preview = await API.PreviewSubmission(source, target);
          if (mine && preview.release.submitted_by !== identity.id)
            throw new Error(
              t(
                "This submission belongs to another publisher.",
                "这次提交属于其他发布者。",
              ),
            );
          if (
            active &&
            requestGeneration === generation.current &&
            selectedID.current === target
          ) {
            setSelected(preview);
            setSelecting("");
            setPage((current) =>
              current
                ? {
                    ...current,
                    releases: (current.releases ?? []).map((r) =>
                      r.id === target ? preview.release : r,
                    ),
                  }
                : current,
            );
          }
        }
      } catch (e) {
        if (active && requestGeneration === generation.current) {
          setLoadError(String(e));
          setSelected(null);
          setSelecting("");
        }
      }
    })();
    return () => {
      active = false;
      generation.current++;
    };
  }, [
    source,
    ui.revision,
    reload,
    query,
    effectiveFilter,
    mine,
    waitingForFilter,
  ]);
  const rows = page?.releases ?? [];
  const select = async (id: string) => {
    if (id === selected?.release.id && !selecting) return;
    const n = ++generation.current;
    selectedID.current = id;
    setSelecting(id);
    setReason("");
    setLoadError("");
    try {
      const p = await API.PreviewSubmission(source, id);
      if (n !== generation.current) return;
      setSelected(p);
      setPage((current) =>
        current
          ? {
              ...current,
              releases: (current.releases ?? []).map((r) =>
                r.id === id ? p.release : r,
              ),
            }
          : current,
      );
      if (matchMedia("(max-width: 760px)").matches)
        requestAnimationFrame(() =>
          documentRef.current?.scrollIntoView({ block: "start" }),
        );
    } catch (e) {
      if (n === generation.current) {
        selectedID.current = selected?.release.id ?? "";
        setLoadError(String(e));
      }
    } finally {
      if (n === generation.current) setSelecting("");
    }
  };
  function deleteVersion() {
    if (!selected || selecting) return;
    const snapshot = selected.release;
    const coordinate =
      snapshot.namespace + "/" + snapshot.name + "@" + snapshot.version;
    ui.confirm(
      t("Delete version", "删除版本"),
      coordinate +
        " — " +
        t(
          "Remove this package and its evaluation reports. Skill discussion, likes, saves, audit records and installed copies remain.",
          "清理该版本的文件和评测报告。保留技能讨论、点赞、收藏、审计记录和已安装副本。",
        ),
      async () => {
        const key = JSON.stringify([source, snapshot.id, "delete"]);
        if (mutation.current?.key !== key)
          mutation.current = { key, id: await API.NewRequestID() };
        await API.DeleteSubmission(
          source,
          snapshot.id,
          coordinate,
          mutation.current.id,
        );
        mutation.current = null;
        selectedID.current = "";
        setSelected(null);
        setPage((p) =>
          p
            ? {
                ...p,
                releases: (p.releases ?? []).filter(
                  (r) => r.id !== snapshot.id,
                ),
              }
            : p,
        );
        const params = new URLSearchParams(location.hash.split("?")[1] ?? "");
        params.delete("release");
        history.replaceState(
          null,
          "",
          `#${mine ? "submissions" : "review"}?${params}`,
        );
        setReload((n) => n + 1);
      },
      "danger",
    );
  }
  function withdraw() {
    if (!selected || !reason.trim()) return;
    const snapshot = selected,
      text = reason.trim();
    ui.confirm(
      t("Withdraw version"),
      `${snapshot.release.namespace}/${snapshot.release.name} ${snapshot.release.version} — ${t("New downloads will stop. Installed copies remain. Reason:", "停止新下载，保留已安装副本。原因：")} ${text}`,
      async () => {
        const key = JSON.stringify([source, snapshot.release.id, text, "yank"]);
        if (mutation.current?.key !== key)
          mutation.current = { key, id: await API.NewRequestID() };
        const r = await API.Yank(
          source,
          snapshot.release.id,
          text,
          mutation.current.id,
        );
        mutation.current = null;
        setSelected({
          ...snapshot,
          release: { ...r, markdown: snapshot.release.markdown },
        });
        setPage(
          await API.FilterSubmissions(
            source,
            query,
            effectiveFilter === "all" || effectiveFilter === "active"
              ? 0
              : Number(effectiveFilter),
            mine,
            "",
            effectiveFilter === "active",
          ),
        );
      },
    );
  }
  function decide(approve: boolean) {
    if (!selected) return;
    const snapshot = selected;
    const text = reason.trim();
    ui.confirm(
      approve ? t("Approve release") : t("Request changes"),
      `${snapshot.release.namespace}/${snapshot.release.name} ${snapshot.release.version} — ${t("Audience")}：${visibilityLabel(snapshot.release.visibility)}。${text}`,
      async () => {
        const key = JSON.stringify([
          source,
          snapshot.release.id,
          text,
          approve,
        ]);
        if (mutation.current?.key !== key)
          mutation.current = { key, id: await API.NewRequestID() };
        const r = await API.Review(
          source,
          snapshot.release.id,
          text,
          mutation.current.id,
          approve,
        );
        mutation.current = null;
        setSelected({
          ...snapshot,
          release: { ...r, markdown: snapshot.release.markdown },
        });
        setPage(
          await API.FilterSubmissions(
            source,
            query,
            effectiveFilter === "all" || effectiveFilter === "active"
              ? 0
              : Number(effectiveFilter),
            mine,
            "",
            effectiveFilter === "active",
          ),
        );
      },
    );
  }
  const defaultFilter = mine
    ? cleanup.value === true
      ? "active"
      : "all"
    : String(Status.ReleaseStatus_RELEASE_STATUS_PENDING);
  const filtered = !!query.trim() || effectiveFilter !== defaultFilter;
  const resetFilters = () => {
    setSearchText("");
    setQuery("");
    setFilter(defaultFilter);
  };
  return (
    <div
      className={`review-workspace ${rows.length === 0 ? "single-pane" : ""} ${page && rows.length === 0 && !selected ? "is-empty" : ""}`}
    >
      <div className="review-toolbar">
        <div className="queue-heading">
          <h2>
            {mine ? t("Versions", "版本") : t("Submissions", "提交")}{" "}
            <small>
              {page ? rows.length : "…"}
              {page?.next_page ? "+" : ""}
            </small>
          </h2>
          <button disabled={ui.busy} onClick={() => setReload((n) => n + 1)}>
            {t("Refresh", "刷新")}
          </button>
        </div>
        <input
          aria-label={t("Find a release", "查找发布")}
          placeholder={t("Filter releases…", "筛选发布…")}
          value={searchText}
          onChange={(e) => {
            setSearchText(e.target.value);
          }}
        />
        <Select
          aria-label={t("Status")}
          value={effectiveFilter}
          onChange={(e) => {
            setFilter(e.target.value);
          }}
        >
          {mine && cleanup.value === true && (
            <option value="active">{t("Current", "当前版本")}</option>
          )}
          <option value="all">{t("All versions", "全部版本")}</option>
          <option value={Status.ReleaseStatus_RELEASE_STATUS_PENDING}>
            {t("In review")}
          </option>
          <option value={Status.ReleaseStatus_RELEASE_STATUS_PUBLISHED}>
            {t("Published")}
          </option>
          <option value={Status.ReleaseStatus_RELEASE_STATUS_YANKED}>
            {t("Withdrawn")}
          </option>
          <option value={Status.ReleaseStatus_RELEASE_STATUS_REJECTED}>
            {t("Changes requested")}
          </option>
        </Select>
        {filtered && rows.length > 0 && (
          <button onClick={resetFilters}>
            {t("Clear filters", "清除筛选")}
          </button>
        )}
      </div>
      {rows.length > 0 && (
        <section className="review-queue" aria-label={t("Submissions", "提交")}>
          {rows.map((r) => (
            <button
              className={`queue-item ${(selecting || selected?.release.id) === r.id ? "active" : ""}`}
              key={r.id}
              onClick={() => void select(r.id)}
            >
              <strong className="queue-identity">
                <SkillSymbol icon={r.icon} />
                {skillTitle(r)}
              </strong>
              <span className="queue-meta">
                <span>
                  {r.namespace}/{r.name} · v{r.version}
                </span>
                <span className={`queue-status status-${r.status}`}>
                  {statusLabel(r.status)}
                </span>
              </span>
            </button>
          ))}
          {page?.next_page && (
            <button
              disabled={ui.busy}
              onClick={() =>
                void ui.run(async () => {
                  const n = generation.current;
                  const p = await API.FilterSubmissions(
                    source,
                    query,
                    effectiveFilter === "all" || effectiveFilter === "active"
                      ? 0
                      : Number(effectiveFilter),
                    mine,
                    page.next_page,
                    effectiveFilter === "active",
                  );
                  if (n !== generation.current) return;
                  setPage({
                    releases: [...(page.releases ?? []), ...(p.releases ?? [])],
                    next_page: p.next_page,
                  });
                })
              }
            >
              {t("Load more submissions")}
            </button>
          )}
        </section>
      )}
      {loadError ? (
        <div className="review-load-error">
          <AccessState
            error={loadError}
            retry={() => setReload((n) => n + 1)}
          />
        </div>
      ) : selected ? (
        <>
          <section
            ref={documentRef}
            className={`review-document ${selecting ? "is-switching" : ""}`}
            aria-busy={!!selecting}
          >
            {selecting && (
              <div className="review-loading" role="status">
                {t("Loading…")}
              </div>
            )}
            <div className="review-document-body" inert={!!selecting}>
              <div className="review-heading">
                <h2 className="queue-identity">
                  <SkillSymbol icon={selected.release.icon} />
                  {skillTitle(selected.release)}
                </h2>
                <span className="muted">
                  {selected.release.namespace}/{selected.release.name} · v
                  {selected.release.version}
                </span>
              </div>
              {access.error && (
                <p role="alert">
                  {t("Unable to check permissions.", "无法检查操作权限。")}{" "}
                  <button onClick={access.reload}>{t("Try again")}</button>
                </p>
              )}
              <section
                className={`release-progress status-${selected.release.status}`}
                aria-label={t("Submission status", "提交状态")}
              >
                <div>
                  <span className="badge">
                    {statusLabel(selected.release.status)}
                  </span>
                  <span className="muted">
                    {t("Submitted", "提交于")}{" "}
                    {dateLabel(selected.release.created_at)}
                  </span>
                  <div className="actions wrap">
                    {selected.release.status ===
                      Status.ReleaseStatus_RELEASE_STATUS_PUBLISHED && (
                      <button
                        onClick={() =>
                          ui.navigate(
                            `discover?${new URLSearchParams({ source, skill: `${selected.release.namespace}/${selected.release.name}`, version: selected.release.version })}`,
                          )
                        }
                      >
                        {t("View published skill", "查看已发布技能")}
                      </button>
                    )}
                    {mine &&
                      [
                        Status.ReleaseStatus_RELEASE_STATUS_REJECTED,
                        Status.ReleaseStatus_RELEASE_STATUS_YANKED,
                        Status.ReleaseStatus_RELEASE_STATUS_PUBLISHED,
                      ].includes(selected.release.status) && (
                        <button
                          className="primary"
                          onClick={() =>
                            ui.navigate(
                              `publish?${new URLSearchParams({ source, from: selected.release.id })}`,
                            )
                          }
                        >
                          {t("Submit an updated version", "提交修改后的版本")}
                        </button>
                      )}
                  </div>
                </div>
                {selected.release.reason ? (
                  <details
                    className="release-feedback"
                    key={selected.release.id}
                  >
                    <summary>
                      <span className="feedback-label">
                        {selected.release.status ===
                        Status.ReleaseStatus_RELEASE_STATUS_YANKED
                          ? t("Withdrawal reason", "撤下原因")
                          : t("Review feedback", "审核反馈")}
                      </span>
                      <span className="feedback-excerpt">
                        {selected.release.reason}
                      </span>
                    </summary>
                    <blockquote>{selected.release.reason}</blockquote>
                  </details>
                ) : selected.release.status ===
                  Status.ReleaseStatus_RELEASE_STATUS_PENDING ? (
                  <p className="release-waiting">
                    {selected.release.submitted_by === identity.id
                      ? t("Waiting for independent review", "等待独立审核")
                      : t("Ready for your review", "等待你的审核")}{" "}
                    · {visibilityLabel(selected.release.visibility)}
                  </p>
                ) : null}
              </section>
              <div className="review-checks">
                <ScanPanel
                  source={source}
                  release={selected.release.id}
                  report={scan.value}
                  loading={scan.loading}
                  error={scan.error}
                  reload={scan.reload}
                />
                <EvaluationPanel
                  key={selected.release.id}
                  source={source}
                  release={selected.release.id}
                  digest={selected.release.digest}
                  compact
                />
              </div>
              <div className="review-content">
                <ProvenanceDetails
                  value={selected.release.provenance}
                  version={selected.release.version}
                  digest={selected.release.digest}
                />
                <PreviewBody preview={selected} />
              </div>
              <details className="review-properties">
                <summary>{t("Release details")}</summary>
                <dl className="properties">
                  {selected.release.display_name_en && (
                    <>
                      <dt>{t("English name", "英文名称")}</dt>
                      <dd lang="en">{selected.release.display_name_en}</dd>
                    </>
                  )}
                  {selected.release.display_name_zh && (
                    <>
                      <dt>{t("Chinese name", "中文名称")}</dt>
                      <dd lang="zh-CN">{selected.release.display_name_zh}</dd>
                    </>
                  )}
                  <dt>{t("Submitted by", "提交账号")}</dt>
                  <dd>
                    {author.value ? (
                      <Person source={source} person={author.value} />
                    ) : (
                      <span title={selected.release.submitted_by}>
                        {selected.release.submitted_by === identity.id
                          ? identity.username
                          : t("Profile unavailable", "主页暂不可用")}
                      </span>
                    )}
                  </dd>
                  <dt>{t("Status")}</dt>
                  <dd>{statusLabel(selected.release.status)}</dd>
                  <dt>{t("Audience")}</dt>
                  <dd>{visibilityLabel(selected.release.visibility)}</dd>
                  <dt>{t("Space")}</dt>
                  <dd>{selected.release.namespace}</dd>
                  <dt>{t("Exact version")}</dt>
                  <dd>{selected.release.version}</dd>
                  <dt>{t("Package digest")}</dt>
                  <dd className="path">{selected.release.digest}</dd>
                </dl>
                <p className="muted">
                  {t("Approval applies to this exact content and audience.")}
                </p>
              </details>
              <div className="review-decision">
                {cleanup.value &&
                  access.value?.can_publish &&
                  [
                    Status.ReleaseStatus_RELEASE_STATUS_REJECTED,
                    Status.ReleaseStatus_RELEASE_STATUS_YANKED,
                  ].includes(selected.release.status) && (
                    <button
                      className="danger-text"
                      disabled={!!selecting || ui.busy}
                      onClick={deleteVersion}
                    >
                      {t("Delete version…", "删除版本…")}
                    </button>
                  )}
                {selected.release.status ===
                  Status.ReleaseStatus_RELEASE_STATUS_PENDING &&
                  selected.release.submitted_by !== identity.id &&
                  access.value?.can_review && (
                    <>
                      <Field label={t("Review note (required for changes)")}>
                        <textarea
                          value={reason}
                          maxLength={1024}
                          onChange={(e) => setReason(e.target.value)}
                          placeholder={t(
                            "Share your feedback or the changes needed…",
                          )}
                        />
                      </Field>
                      <div className="actions">
                        <button
                          disabled={ui.busy || !reason.trim()}
                          onClick={() => decide(false)}
                        >
                          {t("Request changes")}
                        </button>
                        <button
                          className="primary"
                          disabled={ui.busy || !scanAllowsApproval}
                          onClick={() => decide(true)}
                        >
                          {t("Approve release")}
                        </button>
                      </div>
                    </>
                  )}
                {access.value?.can_publish &&
                  [
                    Status.ReleaseStatus_RELEASE_STATUS_PENDING,
                    Status.ReleaseStatus_RELEASE_STATUS_PUBLISHED,
                  ].includes(selected.release.status) && (
                    <details className="withdraw-release">
                      <summary>
                        {t("Withdraw this version…", "撤下当前版本…")}
                      </summary>
                      <Field label={t("Reason for withdrawal (required)")}>
                        <textarea
                          value={reason}
                          maxLength={1024}
                          onChange={(e) => setReason(e.target.value)}
                        />
                      </Field>
                      <button
                        disabled={ui.busy || !reason.trim()}
                        onClick={withdraw}
                      >
                        {t("Withdraw version")}
                      </button>
                    </details>
                  )}
              </div>
            </div>
          </section>
        </>
      ) : (!page || selecting) && !loadError ? (
        <section className="review-document" aria-busy="true">
          <div
            className="loading-skeleton"
            role="status"
            aria-label={t("Loading submissions…")}
          >
            <i />
            <i />
            <i />
          </div>
        </section>
      ) : (
        <Empty
          title={
            rows.length > 0
              ? t("Choose a submission")
              : filtered
                ? t("No matching submissions", "没有匹配的提交")
                : mine
                  ? t("No current releases", "暂无当前发布")
                  : t("All caught up", "暂无待审核内容")
          }
        >
          {rows.length > 0
            ? t(
                "Select a version to see its content, status and review feedback.",
                "选择一个版本，查看内容、状态与审核反馈。",
              )
            : filtered
              ? t(
                  "Try another search or clear the filters.",
                  "尝试其他搜索词，或清除筛选条件。",
                )
              : mine
                ? t(
                    "Your submissions and review feedback will appear here.",
                    "提交的技能及审核反馈将显示在这里。",
                  )
                : t(
                    "New submissions will appear here.",
                    "有新的提交时会显示在这里。",
                  )}
          {rows.length === 0 && (
            <span className="empty-actions">
              {filtered ? (
                <button onClick={resetFilters}>
                  {t("Clear filters", "清除筛选")}
                </button>
              ) : (
                <>
                  {mine && (
                    <button
                      className="primary"
                      onClick={() => ui.navigate("publish")}
                    >
                      {t("Publish a skill", "发布技能")}
                    </button>
                  )}
                  {effectiveFilter !== "all" && (
                    <button onClick={() => setFilter("all")}>
                      {t("View all versions", "查看全部版本")}
                    </button>
                  )}
                </>
              )}
            </span>
          )}
        </Empty>
      )}
    </div>
  );
}
