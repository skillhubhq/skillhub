import { useState } from "react";
import type { NamespaceLifecycle } from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
import { API, Modal, Field, useUI } from "./ui";
import { t } from "./i18n";

export function TeamLifecycle({
  source,
  name,
  changed,
}: {
  source: string;
  name: string;
  changed: () => void;
}) {
  const ui = useUI();
  const [open, setOpen] = useState(false),
    [plan, setPlan] = useState<NamespaceLifecycle | null>(null),
    [confirm, setConfirm] = useState("");
  const [pending, setPending] = useState(false),
    [retryKey, setRetryKey] = useState<{
      action: string;
      revision: string;
      key: string;
    } | null>(null);
  const load = () =>
    void ui.run(async () => {
      setPlan(await API.NamespaceLifecycle(source, name));
    });
  const apply = (action: string) =>
    void ui.run(async () => {
      if (!plan || pending) return;
      setPending(true);
      try {
        const key =
          retryKey?.action === action && retryKey.revision === plan.revision
            ? retryKey.key
            : await API.NewRequestID();
        setRetryKey({ action, revision: plan.revision, key });
        await API.ChangeNamespaceLifecycle(
          source,
          name,
          action,
          plan.revision,
          confirm,
          key,
        );
        setOpen(false);
        setPlan(null);
        setRetryKey(null);
        changed();
        ui.changed(t("Team updated", "团队已更新"));
      } finally {
        setPending(false);
      }
    });
  return (
    <>
      <button
        disabled={ui.busy}
        onClick={() => {
          setOpen(true);
          setConfirm("");
          setPlan(null);
          load();
        }}
      >
        {t("Team lifecycle", "团队归档与清理")}
      </button>
      {open && (
        <Modal
          title={t("Team lifecycle", "团队归档与清理")}
          onClose={() => {
            if (!pending) setOpen(false);
          }}
        >
          <p>
            <strong>{name}</strong>
          </p>
          {!plan ? (
            <>
              <p role="status">
                {t(
                  "Load the current impact before making changes.",
                  "操作前请加载当前影响范围。",
                )}
              </p>
              <button disabled={ui.busy} onClick={load}>
                {t("Refresh", "刷新")}
              </button>
            </>
          ) : (
            <>
              <p>
                {t("Status", "状态")}:{" "}
                <strong>
                  {plan.archived
                    ? t("Archived", "已归档")
                    : t("Active", "使用中")}
                </strong>
              </p>
              <dl className="team-lifecycle-impact">
                <div>
                  <dt>{t("Skill versions", "技能版本")}</dt>
                  <dd>{plan.releases}</dd>
                </div>
                <div>
                  <dt>{t("Role grants", "权限授权")}</dt>
                  <dd>{plan.grants}</dd>
                </div>
                <div>
                  <dt>{t("Comments and replies", "评论与回复")}</dt>
                  <dd>{plan.comments}</dd>
                </div>
                <div>
                  <dt>{t("Evaluation reports", "评测报告")}</dt>
                  <dd>{plan.evidence}</dd>
                </div>
              </dl>
              <p>
                {t(
                  "Archiving removes this team from discovery and stops publishing, reviews and interactions. Authorized readers can still open existing links and download published versions.",
                  "归档后从发现页隐藏团队，停止发布、审核和互动。已有权限的用户仍可通过链接查看内容、下载已发布版本。",
                )}
              </p>
              <p className="muted">
                {t(
                  "Installed files, shared user accounts, access groups and audit records are preserved.",
                  "本机已安装文件、共享用户账号、权限组和审计记录均保留。",
                )}
              </p>
              <div className="actions">
                <button
                  disabled={ui.busy || pending}
                  onClick={() => apply(plan.archived ? "restore" : "archive")}
                >
                  {plan.archived
                    ? t("Restore team", "恢复团队")
                    : t("Archive team", "归档团队")}
                </button>
                <button disabled={ui.busy || pending} onClick={load}>
                  {t("Refresh impact", "刷新影响范围")}
                </button>
              </div>
              {plan.archived && (
                <section className="team-lifecycle-danger">
                  <h3>{t("Permanently delete team", "永久清理团队")}</h3>
                  <p>
                    {t(
                      "Deletes every version, discussion, review notification, evaluation report, team profile and role grant. Unshared packages are removed. This cannot be undone in the product; recovery requires an operator backup. The namespace cannot be reused.",
                      "删除全部版本、讨论、审核通知、评测报告、团队资料和授权，并清理无其他引用的包。产品内无法撤销，只能由运维从备份恢复。命名空间不可重新使用。",
                    )}
                  </p>
                  <Field
                    label={t(
                      "Type the namespace to confirm",
                      "输入命名空间确认",
                    )}
                  >
                    <input
                      value={confirm}
                      disabled={pending}
                      onChange={(e) => setConfirm(e.target.value)}
                      autoComplete="off"
                      spellCheck={false}
                    />
                  </Field>
                  <button
                    className="danger"
                    disabled={ui.busy || pending || confirm !== name}
                    onClick={() => apply("purge")}
                  >
                    {t("Permanently delete team", "永久清理团队")}
                  </button>
                </section>
              )}
            </>
          )}
        </Modal>
      )}
    </>
  );
}
