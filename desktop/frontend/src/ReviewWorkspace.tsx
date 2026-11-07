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
      mine ? "all" : String(Status.ReleaseStatus_RELEASE_STATUS_PENDING),
    );
  const documentRef = useRef<HTMLElement>(null);
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
      setSelected(null);
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
      documentRef.current?.scrollIntoView({ block: "start" });
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
  useEffect(() => {
    let active = true;
    setPage(null);
    setLoadError("");
    const filterKey = JSON.stringify([query, filter]);
    if (appliedFilter.current !== filterKey) {
      appliedFilter.current = filterKey;
      selectedID.current = "";
      setSelected(null);
      setReason("");
    }
    const requestedID = selectedID.current;
    void (async () => {
      try {
        const p = await API.FilterSubmissions(
          source,
          query,
          filter === "all" ? 0 : Number(filter),
          mine,
          "",
        );
        if (active) setPage(p);
        const target = requestedID || p.releases?.[0]?.id;
        if (active && target && selectedID.current === requestedID) {
          selectedID.current = target;
          const preview = await API.PreviewSubmission(source, target);
          if (mine && preview.release.submitted_by !== identity.id)
            throw new Error(
              t(
                "This submission belongs to another publisher.",
                "这次提交属于其他发布者。",
              ),
            );
          if (active && selectedID.current === target) setSelected(preview);
        }
      } catch (e) {
        if (active) setLoadError(String(e));
      }
    })();
    return () => {
      active = false;
      generation.current++;
    };
  }, [source, ui.revision, reload, query, filter, mine]);
  const rows = page?.releases ?? [];
  const select = async (id: string) => {
    const n = ++generation.current;
    selectedID.current = id;
    setSelected(null);
    setReason("");
    await ui.run(async () => {
      const p = await API.PreviewSubmission(source, id);
      if (n === generation.current) setSelected(p);
    });
  };
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
            filter === "all" ? 0 : Number(filter),
            mine,
            "",
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
            filter === "all" ? 0 : Number(filter),
            mine,
            "",
          ),
        );
      },
    );
  }
  return (
    <div className="review-workspace">
      <section className="review-queue">
        <div className="queue-heading">
          <h2>
            {t("Versions", "版本")}{" "}
            <small>
              {rows.length}
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
          value={filter}
          onChange={(e) => {
            setFilter(e.target.value);
          }}
        >
          <option value="all">{t("All statuses")}</option>
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
        {loadError ? (
          <p role="alert">{loadError}</p>
        ) : !page ? (
          <p className="muted">{t("Loading submissions…")}</p>
        ) : rows.length === 0 ? (
          <div className="queue-empty">
            <p className="muted">{t("No submissions match this filter.")}</p>
            {mine && (
              <button onClick={() => ui.navigate("publish")}>
                {t("Publish a skill", "发布技能")}
              </button>
            )}
          </div>
        ) : (
          rows.map((r) => (
            <button
              className={`queue-item ${selected?.release.id === r.id ? "active" : ""}`}
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
          ))
        )}
        {page?.next_page && (
          <button
            disabled={ui.busy}
            onClick={() =>
              void ui.run(async () => {
                const n = generation.current;
                const p = await API.FilterSubmissions(
                  source,
                  query,
                  filter === "all" ? 0 : Number(filter),
                  mine,
                  page.next_page,
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
      {selected ? (
        <>
          <section ref={documentRef} className="review-document">
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
                <details className="release-feedback" key={selected.release.id}>
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
            <ScanPanel
              source={source}
              release={selected.release.id}
              report={scan.value}
              loading={scan.loading}
              error={scan.error}
              reload={scan.reload}
            />
            <div className="review-content">
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
                <dt>{t("Published by")}</dt>
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
          </section>
        </>
      ) : (
        <Empty title={t("Choose a submission")}>
          {mine
            ? t(
                "Select a version to see its content, status and review feedback.",
                "选择一个版本，查看内容、状态与审核反馈。",
              )
            : t(
                "Read the content, check the audience, then share your decision.",
              )}
        </Empty>
      )}
    </div>
  );
}
