import { useId, useRef, useState } from "react";
import { API, Field, Modal, Empty, useUI } from "./ui";
import { ResourceState, useResource, useMutationKey } from "./community-ui";
import { dateLabel, t } from "./i18n";
import { ReportResolution } from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";

export function ReportButton({
  source,
  release,
  comment = "",
}: {
  source: string;
  release: string;
  comment?: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>{t("Report", "举报")}</button>
      {open && (
        <ReportDialog
          source={source}
          release={release}
          comment={comment}
          close={() => setOpen(false)}
        />
      )}
    </>
  );
}
function ReportDialog({
  source,
  release,
  comment,
  close,
}: {
  source: string;
  release: string;
  comment: string;
  close: () => void;
}) {
  const ui = useUI(),
    key = useMutationKey();
  const [reason, setReason] = useState(""),
    [sent, setSent] = useState(false);
  const formID = useId();
  const access = useResource(
    () => API.CommunityParticipation(source),
    [source],
  );
  return (
    <Modal
      stable="compact"
      title={t("Report content", "举报内容")}
      onClose={close}
      footer={
        !sent && access.value?.supported ? (
          <div className="actions">
            <button type="button" onClick={close}>
              {t("Cancel", "取消")}
            </button>
            <button
              type="submit"
              form={formID}
              className="primary"
              disabled={ui.busy || !reason.trim()}
            >
              {t("Send report", "提交举报")}
            </button>
          </div>
        ) : undefined
      }
    >
      {sent ? (
        <Empty title={t("Report received", "举报已提交")}>
          {t(
            "A Hub administrator will review it. Reporting does not automatically remove content.",
            "Hub 管理员将处理这条举报，举报不会自动移除内容。",
          )}
        </Empty>
      ) : (
        <ResourceState {...access} retry={access.reload}>
          {access.value?.supported ? (
            <form
              id={formID}
              onSubmit={(e) => {
                e.preventDefault();
                void ui.run(async () => {
                  await API.CommunityReport(
                    source,
                    release,
                    comment,
                    reason.trim(),
                    await key([source, release, comment, reason.trim()]),
                  );
                  setSent(true);
                });
              }}
            >
              <p>
                {t(
                  "What should the Hub administrators review?",
                  "请说明需要 Hub 管理员处理的问题。",
                )}
              </p>
              <Field label={t("Reason", "原因")}>
                <textarea
                  autoFocus
                  required
                  maxLength={1000}
                  rows={5}
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                />
              </Field>
            </form>
          ) : (
            access.value && (
              <p>
                {t(
                  "This Hub does not support content reports. Contact its operator.",
                  "此 Hub 尚不支持内容举报，请联系部署方。",
                )}
              </p>
            )
          )}
        </ResourceState>
      )}
    </Modal>
  );
}
export function ContentReports({ source }: { source: string }) {
  const ui = useUI(),
    key = useMutationKey();
  const [resolved, setResolved] = useState(false),
    [note, setNote] = useState("");
  const [selected, setSelected] = useState<
    | NonNullable<
        Awaited<ReturnType<typeof API.CommunityReports>>["items"]
      >[number]
    | null
  >(null);
  // 过滤条件、连接或刷新变化后，旧的翻页响应不能写回新列表。
  const scope = `${source}:${resolved}`;
  const request = useRef({ scope, revision: 0 });
  if (request.current.scope !== scope) request.current = { scope, revision: 0 };
  const records = useResource(
    () => API.CommunityReports(source, resolved, ""),
    [source, resolved],
  );
  async function resolve(action: ReportResolution) {
    if (!selected) return;
    await ui.run(async () => {
      await API.CommunityResolveReport(
        source,
        selected.id,
        action,
        note.trim(),
        await key([source, selected.id, action, note.trim()]),
      );
      setSelected(null);
      setNote("");
      records.reload();
    });
  }
  return (
    <section className="content-reports">
      <div className="toolbar">
        <h2>{t("Content reports", "内容举报")}</h2>
        <div className="tabs">
          {[false, true].map((done) => (
            <button
              key={String(done)}
              aria-pressed={resolved === done}
              className={resolved === done ? "selected" : ""}
              onClick={() => {
                setSelected(null);
                setResolved(done);
              }}
            >
              {done ? t("Resolved", "已处理") : t("Needs review", "待处理")}
            </button>
          ))}
        </div>
        <button
          disabled={ui.busy}
          onClick={() => {
            request.current.revision++;
            records.reload();
          }}
        >
          {t("Refresh", "刷新")}
        </button>
      </div>
      <ResourceState {...records} retry={records.reload}>
        {!records.value?.items?.length ? (
          <Empty title={t("No reports here", "暂无举报")} />
        ) : (
          <div className="report-list">
            {records.value.items.map((r) => (
              <button
                key={r.id}
                className="report-row"
                onClick={() => {
                  setSelected(r);
                  setNote("");
                }}
              >
                <span>
                  <strong>{r.coordinate}</strong>
                  <small>
                    {r.comment_id
                      ? t("Comment", "评论")
                      : t("Skill version", "技能版本")}{" "}
                    · {dateLabel(r.created_at)}
                  </small>
                </span>
                <span>{r.reason}</span>
              </button>
            ))}
          </div>
        )}
        {records.value?.next_page && (
          <button
            disabled={ui.busy}
            onClick={() =>
              void ui.run(async () => {
                const pending = request.current;
                const revision = pending.revision;
                const page = await API.CommunityReports(
                  source,
                  resolved,
                  records.value!.next_page,
                );
                if (
                  request.current !== pending ||
                  pending.revision !== revision
                )
                  return;
                records.setValue({
                  items: [
                    ...(records.value!.items ?? []),
                    ...(page.items ?? []),
                  ],
                  next_page: page.next_page,
                });
              })
            }
          >
            {t("Load more", "加载更多")}
          </button>
        )}
      </ResourceState>
      {selected && (
        <Modal
          stable
          title={t("Review report", "处理举报")}
          onClose={() => setSelected(null)}
        >
          <div className="report-review">
            <p className="report-coordinate">{selected.coordinate}</p>
            <p>
              <strong>{t("Reported reason", "举报原因")}</strong>
            </p>
            <p className="report-content">{selected.reason}</p>
            <button
              onClick={() => {
                const [skill, version] = selected.coordinate.split("@");
                setSelected(null);
                ui.navigate(
                  `discover?${new URLSearchParams({ source, skill, version })}`,
                );
              }}
            >
              {t("Open skill version", "查看技能版本")}
            </button>
            <p>
              <strong>{t("Current content", "当前内容")}</strong>
            </p>
            <blockquote className="report-content">
              {selected.content ||
                t("Content has been removed.", "内容已移除。")}
            </blockquote>
            {selected.resolved ? (
              <div className="report-resolution">
                <strong>
                  {selected.resolution.startsWith("remove_content: ")
                    ? t("Content removed", "内容已移除")
                    : t("Report dismissed", "举报已驳回")}
                </strong>
                <p className="report-content">
                  {selected.resolution.replace(
                    /^(remove_content|dismiss): /,
                    "",
                  )}
                </p>
              </div>
            ) : (
              <>
                <Field label={t("Private decision note", "内部处理说明")}>
                  <textarea
                    rows={3}
                    maxLength={1000}
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                  />
                </Field>
                <p className="muted">
                  {t(
                    "Only Hub administrators can read this note.",
                    "仅 Hub 管理员可以查看这条说明。",
                  )}
                </p>
                <p>
                  {selected.comment_id
                    ? t(
                        "Removal hides this comment while retaining its replies.",
                        "移除会隐藏这条评论，保留回复。",
                      )
                    : t(
                        "Removal withdraws this exact version. Installed copies are not deleted.",
                        "移除会撤下这个精确版本，不会删除已经安装的副本。",
                      )}
                </p>
                <div className="actions">
                  <button
                    disabled={ui.busy || !note.trim()}
                    onClick={() =>
                      void resolve(
                        ReportResolution.ReportResolution_REPORT_RESOLUTION_DISMISS,
                      )
                    }
                  >
                    {t("Dismiss report", "驳回举报")}
                  </button>
                  <button
                    className="danger"
                    disabled={ui.busy || !note.trim()}
                    onClick={() =>
                      void resolve(
                        ReportResolution.ReportResolution_REPORT_RESOLUTION_REMOVE_CONTENT,
                      )
                    }
                  >
                    {t("Remove content", "移除内容")}
                  </button>
                </div>
              </>
            )}
          </div>
        </Modal>
      )}
    </section>
  );
}
