import { useEffect, useState } from "react";
import { API, Modal, useUI } from "./ui";
import { useResource, useMutationKey } from "./community-ui";
import { dateLabel, t } from "./i18n";
import type { ScanInfo } from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
import { PlatformRole } from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";
import "./governance.css";
export function ScanPanel({
  source,
  release,
  report,
  loading,
  error,
  reload,
}: {
  source: string;
  release: string;
  report: ScanInfo | null;
  loading: boolean;
  error: string;
  reload: () => void;
}) {
  const ui = useUI(),
    key = useMutationKey();
  if (report && !report.supported)
    return (
      <p className="muted">
        {t(
          "This Hub does not provide scan reports.",
          "此 Hub 尚未提供扫描报告。",
        )}
      </p>
    );
  const label = error
    ? t("Scan unavailable", "扫描暂不可用")
    : loading
      ? t("Loading scan…", "正在读取扫描…")
      : !report?.engine
        ? t("Scan required before approval", "批准前需要扫描")
        : report.blocking
          ? t("Blocked by security scan", "安全扫描阻止发布")
          : !report.complete || report.findings?.length
            ? t("Scan findings need review", "扫描发现待人工确认项")
            : t("No built-in rules matched", "未命中内置检查规则");
  return (
    <details className="scan-panel" key={release}>
      <summary>
        <strong>{t("Package scan", "包扫描")}</strong>
        <span>{label}</span>
      </summary>
      <p className="muted">
        {t(
          "Offline static checks only. Files are never executed. A clean report is not a security guarantee.",
          "仅进行离线静态检查，不执行文件。未命中规则并不代表安全保证。",
        )}
      </p>
      {error && (
        <p role="alert">
          {t("Could not load the report.", "无法读取报告。 ")}{" "}
          <button onClick={reload}>{t("Try again")}</button>
        </p>
      )}
      {report?.engine && (
        <p className="muted">
          {report.engine} · {dateLabel(report.scanned_at)}
        </p>
      )}
      <ul>
        {report?.findings?.map((f, i) => (
          <li key={i}>
            <strong>{f.rule}</strong> ·{" "}
            <code>
              {f.path}
              {f.line ? `:${f.line}` : ""}
            </code>
            <p>{f.message}</p>
          </li>
        ))}
      </ul>
      {!!report?.engine &&
        (!report.complete || !!report.findings?.length) &&
        !report.blocking && (
          <p>
            {t(
              "Inspect the findings and record a review note before approval.",
              "批准前需人工确认这些发现，并填写审核说明。",
            )}
          </p>
        )}
      {report?.blocking && (
        <p>
          {t(
            "Remove the blocked content and submit a new version. This version cannot be approved.",
            "移除阻断内容后提交新版本；当前版本不能获批。",
          )}
        </p>
      )}
      <button
        disabled={ui.busy || loading}
        onClick={() =>
          void ui.run(async () => {
            await API.ScanRelease(
              source,
              release,
              await key([source, release, report?.scanned_at]),
            );
            reload();
          })
        }
      >
        {report?.engine
          ? t("Scan again", "重新扫描")
          : t("Scan package", "扫描技能包")}
      </button>
    </details>
  );
}
export function ReviewInbox({ source }: { source: string }) {
  const [open, setOpen] = useState(false);
  const ui = useUI();
  const tasks = useResource(
    () => API.ReviewTasks(source, ""),
    [source, ui.revision],
  );
  useEffect(() => {
    const refresh = () => {
      if (document.visibilityState === "visible") tasks.reload();
    };
    const timer = setInterval(refresh, 60000);
    window.addEventListener("focus", refresh);
    return () => {
      clearInterval(timer);
      window.removeEventListener("focus", refresh);
    };
  }, [source]);
  return (
    <>
      <button
        onClick={() => setOpen(true)}
        title={
          tasks.value?.total
            ? `${tasks.value.total} ${t("awaiting your review", "条待你审核")}`
            : t("Review notifications", "审核通知")
        }
      >
        {t("Inbox", "通知")}
        {tasks.value?.total ? ` · ${tasks.value.total}` : ""}
      </button>
      {open && (
        <InboxDialog
          source={source}
          close={() => {
            setOpen(false);
            tasks.reload();
          }}
        />
      )}
    </>
  );
}
function InboxDialog({ source, close }: { source: string; close: () => void }) {
  const ui = useUI(),
    [token, setToken] = useState("");
  const inbox = useResource(
    () => API.Notifications(source, token),
    [source, token, ui.revision],
  );
  return (
    <Modal title={t("Review notifications", "审核通知")} onClose={close}>
      <div className="review-inbox-content">
        <ReviewTaskList source={source} close={close} />
        <h3>{t("Review outcomes", "审核结果")}</h3>
        <p className="muted">
          {ui.sources.find((s) => s.alias === source)?.display_name || source} ·{" "}
          {t("Feedback on your submissions", "你的提交审核反馈")}
        </p>
        {inbox.loading ? (
          <p role="status">{t("Loading…")}</p>
        ) : inbox.error ? (
          <p role="alert">
            {inbox.error}{" "}
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
          <p>{t("No review updates yet.", "暂无审核更新。")}</p>
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
                    close();
                    ui.navigate(
                      `submissions?${new URLSearchParams({ source, release: n.release_id })}`,
                    );
                  })
                }
              >
                <strong>{n.coordinate}</strong>
                <span>
                  {n.approved
                    ? t("Approved", "已批准")
                    : t("Changes requested", "需要修改")}{" "}
                  · {dateLabel(n.created_at)}
                  {!n.read && ` · ${t("Unread", "未读")}`}
                </span>
                <span>{t("View feedback →", "查看反馈 →")}</span>
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
          <button onClick={inbox.reload}>{t("Refresh")}</button>
        </div>
      </div>
    </Modal>
  );
}
export function PlatformRoles({
  source,
  id,
  name,
  changed,
}: {
  source: string;
  id: string;
  name: string;
  changed: () => void;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>
        {t("Platform duties", "平台职责")}
      </button>
      {open && (
        <RoleDialog
          source={source}
          id={id}
          name={name}
          changed={changed}
          close={() => setOpen(false)}
        />
      )}
    </>
  );
}
function RoleDialog({
  source,
  id,
  name,
  close,
  changed,
}: {
  source: string;
  id: string;
  name: string;
  close: () => void;
  changed: () => void;
}) {
  const ui = useUI(),
    key = useMutationKey();
  const access = useResource(
    () => API.PlatformAccess(source, id),
    [source, id],
  );
  const options: [PlatformRole, string, string][] = [
    [
      PlatformRole.PlatformRole_PLATFORM_ROLE_USER_ADMIN,
      t("People administrator", "人员管理员"),
      t(
        "Create and approve ordinary accounts. No role grants or additional skill access.",
        "创建、审批和管理普通账号；不授予角色或额外技能访问权。",
      ),
    ],
    [
      PlatformRole.PlatformRole_PLATFORM_ROLE_AUDITOR,
      t("Auditor", "审计员"),
      t(
        "Read audit logs. No account changes or additional skill access.",
        "只读审计日志；不能修改账号，也不增加技能访问权。",
      ),
    ],
  ];
  return (
    <Modal
      title={`${t("Platform duties", "平台职责")} · ${name}`}
      onClose={close}
    >
      <p className="muted">
        {t(
          "Changes revoke this user’s Hub sessions. Namespace roles remain separate.",
          "职责变更将注销该用户所有 Hub 会话；命名空间角色独立管理。",
        )}
      </p>
      {access.loading ? (
        <p>{t("Loading…")}</p>
      ) : access.error ? (
        <p role="alert">
          {access.error}
          <button onClick={access.reload}>{t("Try again")}</button>
        </p>
      ) : !access.value?.supported ? (
        <p>
          {t(
            "This Hub does not support platform duties yet.",
            "此 Hub 尚不支持平台职责。",
          )}
        </p>
      ) : (
        <div className="platform-duties">
          {options.map(([role, title, description]) => (
            <label className="check" key={role}>
              <input
                type="checkbox"
                checked={access.value?.roles?.includes(role) ?? false}
                disabled={ui.busy}
                onChange={(e) => {
                  const granted = e.target.checked;
                  ui.confirm(
                    title,
                    t(
                      "Apply this change and revoke active sessions?",
                      "应用变更并注销现有会话？",
                    ),
                    async () => {
                      await API.SetPlatformRole(
                        source,
                        id,
                        role,
                        granted,
                        await key([source, id, role, granted]),
                      );
                      access.reload();
                      changed();
                    },
                    "primary",
                  );
                }}
              />
              <span>
                <strong>{title}</strong>
                <small>{description}</small>
              </span>
            </label>
          ))}
        </div>
      )}
    </Modal>
  );
}

export function ReviewTaskList({
  source,
  close,
}: {
  source: string;
  close: () => void;
}) {
  const ui = useUI(),
    [token, setToken] = useState("");
  const tasks = useResource(
    () => API.ReviewTasks(source, token),
    [source, token, ui.revision],
  );
  if (tasks.loading)
    return (
      <p role="status">{t("Loading review tasks…", "正在读取审核待办…")}</p>
    );
  if (tasks.error)
    return (
      <p role="alert">
        {t("Review tasks unavailable", "审核待办暂不可用")}{" "}
        <button onClick={tasks.reload}>{t("Try again")}</button>
      </p>
    );
  if (!tasks.value?.supported) return null;
  return (
    <section className="review-tasks">
      <div className="section-heading">
        <h3>
          {t("Needs your review", "待你审核")} · {tasks.value.total}
        </h3>
        <button onClick={tasks.reload}>{t("Refresh")}</button>
      </div>
      {tasks.value.oldest_at && (
        <p className="muted">
          {t("Oldest submission", "最早提交")} ·{" "}
          {dateLabel(tasks.value.oldest_at)}
        </p>
      )}
      {!tasks.value.total && (
        <p className="muted">
          {t("You're all caught up.", "当前没有待你审核的提交。")}
        </p>
      )}
      <div className="review-inbox-list">
        {(tasks.value.tasks ?? []).map((task) => (
          <button
            key={task.release_id}
            onClick={() => {
              close();
              ui.navigate(
                `review?${new URLSearchParams({ source, release: task.release_id })}`,
              );
            }}
          >
            <strong>{task.coordinate}</strong>
            <span>
              {dateLabel(task.created_at)} · {t("Review →", "前往审核 →")}
            </span>
          </button>
        ))}
      </div>
      <div className="actions">
        {token && (
          <button onClick={() => setToken("")}>
            {t("First page", "首页")}
          </button>
        )}
        {tasks.value.next_page && (
          <button onClick={() => setToken(tasks.value!.next_page)}>
            {t("Next page")}
          </button>
        )}
      </div>
    </section>
  );
}
export function FeaturedControl({
  source,
  skill,
  changed,
}: {
  source: string;
  skill: { namespace: string; name: string; featured: boolean };
  changed: (v: boolean) => void;
}) {
  const ui = useUI(),
    key = useMutationKey();
  const access = useResource(
    () => API.PlatformAccess(source, ""),
    [source, ui.revision],
  );
  if (!access.value?.can_curate)
    return skill.featured ? (
      <p className="featured-label">{t("Featured", "精选")}</p>
    ) : null;
  return (
    <div className="featured-control">
      <button
        disabled={ui.busy}
        onClick={() =>
          void ui.run(async () => {
            await API.SetFeaturedSkill(
              source,
              skill.namespace,
              skill.name,
              !skill.featured,
              await key([source, skill.namespace, skill.name, !skill.featured]),
            );
            changed(!skill.featured);
          })
        }
      >
        {skill.featured
          ? t("Remove from featured", "取消精选")
          : t("Feature this skill", "设为精选")}
      </button>
      <small className="muted">
        {t(
          "Hub curation · does not change access",
          "Hub 精选 · 不改变访问权限",
        )}
      </small>
    </div>
  );
}
