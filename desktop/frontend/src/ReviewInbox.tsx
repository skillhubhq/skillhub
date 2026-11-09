import { connectionName } from "./ConnectionPreferences";
import { useEffect, useState } from "react";
import { API, Modal, useUI } from "./ui";
import { useResource } from "./community-ui";
import { ReviewTaskList } from "./Governance";
import { Icon } from "./Icon";
import { dateLabel, t } from "./i18n";
import "./governance.css";

export function ReviewInbox({ source }: { source: string }) {
  const [open, setOpen] = useState(false);
  const ui = useUI();
  const tasks = useResource(
    () =>
      ui.workspaceAccess?.review
        ? API.ReviewTasks(source, "")
        : Promise.resolve(null),
    [source, ui.workspaceAccess?.review],
    ui.revision,
  );
  const notices = useResource(
    () => API.Notifications(source, ""),
    [source, ui.workspaceAccess?.review],
    ui.revision,
  );
  const count = notices.value?.unread ?? 0;
  useEffect(() => {
    const refresh = () => {
      if (document.visibilityState === "visible") {
        tasks.reload();
        notices.reload();
      }
    };
    const timer = setInterval(refresh, 60000);
    window.addEventListener("focus", refresh);
    document.addEventListener("visibilitychange", refresh);
    return () => {
      clearInterval(timer);
      window.removeEventListener("focus", refresh);
      document.removeEventListener("visibilitychange", refresh);
    };
  }, [source]);
  const label = notices.error
    ? t("Inbox — could not refresh", "通知 — 暂时无法刷新")
    : count
      ? `${t("Inbox", "通知")} · ${count} ${t("unread", "条未读")}`
      : t("Inbox", "通知");
  return (
    <>
      <button
        className="utility-inbox"
        aria-label={label}
        title={label}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
      >
        <Icon name="inbox" />
        {!notices.error && count > 0 && (
          <span className="inbox-count" aria-hidden="true">
            {count > 99 ? "99+" : count}
          </span>
        )}
        {!!notices.error && (
          <span className="inbox-unavailable" aria-hidden="true">
            !
          </span>
        )}
      </button>
      {open && (
        <InboxDialog
          source={source}
          initialTab={
            count || !ui.workspaceAccess?.review ? "outcomes" : "tasks"
          }
          close={() => {
            setOpen(false);
            tasks.reload();
            notices.reload();
          }}
        />
      )}
    </>
  );
}
function InboxDialog({
  source,
  initialTab,
  close,
}: {
  source: string;
  initialTab: "tasks" | "outcomes";
  close: () => void;
}) {
  const ui = useUI();
  const [tab, setTab] = useState(initialTab);
  const hub = ui.sources.find((s) => s.alias === source);
  return (
    <Modal
      title={t("Inbox", "通知")}
      onClose={close}
      stable
      toolbar={
        <div className="review-inbox-content">
          <p className="inbox-context">
            <strong>{connectionName(hub) || source}</strong>
          </p>
          <div
            className="inbox-tabs"
            role="group"
            aria-label={t("Inbox views", "通知分类")}
          >
            {ui.workspaceAccess?.review && (
              <button
                aria-pressed={tab === "tasks"}
                onClick={() => setTab("tasks")}
              >
                {t("Needs your review", "待你审核")}
              </button>
            )}
            <button
              aria-pressed={tab === "outcomes"}
              onClick={() => setTab("outcomes")}
            >
              {t("Updates", "动态")}
            </button>
          </div>
        </div>
      }
    >
      {ui.workspaceAccess?.review && (
        <div className="inbox-panel" hidden={tab !== "tasks"}>
          <ReviewTaskList source={source} close={close} />
        </div>
      )}
      <div className="inbox-panel" hidden={tab !== "outcomes"}>
        <ReviewOutcomes source={source} close={close} />
      </div>
    </Modal>
  );
}
function ReviewOutcomes({
  source,
  close,
}: {
  source: string;
  close: () => void;
}) {
  const ui = useUI();
  const [token, setToken] = useState("");
  const inbox = useResource(
    () => API.Notifications(source, token),
    [source, token],
    ui.revision,
  );
  return (
    <section aria-label={t("Updates", "动态")}>
      <div className="section-heading">
        <h3>{t("Latest updates", "最近更新")}</h3>
        <button
          disabled={inbox.loading || !inbox.value?.unread}
          onClick={() =>
            void ui.run(async () => {
              await API.MarkAllNotificationsRead(source);
              inbox.reload();
              ui.refresh();
            })
          }
        >
          {t("Mark all read", "全部已读")}
        </button>
        <button disabled={inbox.loading} onClick={inbox.reload}>
          {t("Refresh")}
        </button>
      </div>
      {inbox.loading ? (
        <p role="status">{t("Loading…")}</p>
      ) : inbox.error ? (
        <p role="alert">
          {t("Could not load review updates.", "暂时无法读取审核结果。")}{" "}
          <button onClick={inbox.reload}>{t("Try again")}</button>
        </p>
      ) : !inbox.value?.supported ? (
        <p>
          {t(
            "This Hub does not support notifications yet.",
            "此 Hub 尚不支持通知。",
          )}
        </p>
      ) : !inbox.value.items?.length ? (
        <div className="inbox-empty">
          <Icon name="inbox" />
          <strong>{t("No updates yet.", "暂无动态。")}</strong>
          <p>
            {t(
              "Replies and updates on your skills will appear here.",
              "与你相关的回复和技能动态会显示在这里。",
            )}
          </p>
        </div>
      ) : (
        <div className="review-inbox-list review-outcomes">
          {inbox.value.items.map((n) => (
            <button
              key={n.id}
              className={n.read ? "" : "unread"}
              disabled={ui.busy}
              onClick={() =>
                void ui.run(async () => {
                  await API.MarkNotificationRead(source, n.id);
                  if (n.unavailable) {
                    inbox.reload();
                    return;
                  }
                  close();
                  if (n.kind === "reply") {
                    ui.navigate(
                      `discover?${new URLSearchParams({ source, skill: n.coordinate.split("@")[0], tab: "discussion" })}`,
                    );
                    return;
                  }
                  ui.navigate(
                    `submissions?${new URLSearchParams({ source, release: n.release_id })}`,
                  );
                })
              }
            >
              <strong>
                {n.unavailable
                  ? t("Content unavailable", "内容已不可用")
                  : n.coordinate}
              </strong>
              <span className="inbox-outcome-meta">
                <span
                  className={`outcome-status ${n.approved ? "approved" : "changes"}`}
                >
                  {n.kind === "reply"
                    ? t("New reply", "新回复")
                    : n.approved
                      ? t("Approved", "已批准")
                      : t("Changes requested", "需要修改")}
                </span>
                <time>{dateLabel(n.created_at)}</time>
                {!n.read && <span>{t("Unread", "未读")}</span>}
              </span>
              <span className="inbox-open">
                {n.unavailable ? t("Mark read", "标为已读") : t("Open", "打开")}
                <Icon name="arrow" />
              </span>
            </button>
          ))}
        </div>
      )}
      <div className="actions">
        {token && (
          <button onClick={() => setToken("")}>{t("Latest", "最新")}</button>
        )}
        {inbox.value?.next_page && (
          <button onClick={() => setToken(inbox.value!.next_page)}>
            {t("Next page")}
          </button>
        )}
      </div>
    </section>
  );
}
