import { ContentReports } from "./Moderation";
import { PlatformRoles } from "./Governance";
import { Select } from "./Select";
import { signInURL } from "./auth";
import { useResource } from "./community-ui";
import {
  DirectoryPicker,
  useDirectory,
  directoryLabel,
} from "./DirectoryPicker";
import { t } from "./i18n";
import {
  NamespaceRole as Role,
  PlatformRole as Duty,
  PrincipalKind as Principal,
  ReleaseStatus as Status,
  Visibility,
} from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";
import { useEffect, useRef, useState } from "react";
import { isWeb } from "./platform";
import { PublishWizard } from "./PublishWizard";
import { ReviewWorkspace } from "./ReviewWorkspace";
import type {
  AdminForm,
  AdminPage,
  AdminRow,
  IdentityView,
  PublishForm,
  ReleasePage,
} from "../bindings/github.com/skillhubhq/skillhub/desktop/models";
import type {
  ReleaseInfo,
  ReleasePreview,
} from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
import {
  API,
  Modal,
  DirectoryField,
  Empty,
  Field,
  PreviewBody,
  roleLabel,
  splitList,
  statusLabel,
  useUI,
  visibilityLabel,
} from "./ui";
const tabs = [
  ["submissions", "Submissions"],
  ["publish", "Publish"],
  ["namespaces", "Namespace"],
  ["groups", "Access groups"],
  ["users", "Users"],
  ["categories", "Category"],
  ["grants", "Role grants"],
  ["audit", "Audit log"],
  ["reports", "Content reports"],
];
function useMutationID() {
  const value = useRef<{ key: string; id: string } | null>(null);
  return {
    get: async (input: unknown) => {
      const key = JSON.stringify(input);
      if (value.current?.key !== key)
        value.current = { key, id: await API.NewRequestID() };
      return value.current.id;
    },
    clear: () => {
      value.current = null;
    },
  };
}
export function Management({ mode = "management" }: { mode?: string }) {
  const ui = useUI();
  const source = ui.activeSource;
  const [identityLoading, setIdentityLoading] = useState(true);
  const [kind, setKind] = useState("users"),
    [identity, setIdentity] = useState<IdentityView | null>(null);
  useEffect(() => {
    let active = true;
    setIdentityLoading(!!source);
    setIdentity(null);
    if (source)
      void ui.run(async () => {
        try {
          const value = await API.Identity(source);
          if (active) setIdentity(value);
        } catch (error) {
          if (active) setIdentity(null);
          throw error;
        } finally {
          if (active) setIdentityLoading(false);
        }
      });
    return () => {
      active = false;
    };
  }, [source, ui.revision]);
  const participation = useResource(
    () =>
      source && identity
        ? API.CommunityParticipation(source)
        : Promise.resolve(null),
    [source, identity?.id, ui.revision],
  );
  const duties = useResource(
    () =>
      source && identity
        ? API.PlatformAccess(source, "")
        : Promise.resolve(null),
    [source, identity?.id, ui.revision],
  );
  useEffect(() => {
    if (
      mode !== "management" ||
      duties.loading ||
      !duties.value ||
      identity?.administrator
    )
      return;
    if (
      (kind === "users" && !duties.value.can_manage_users) ||
      (kind === "audit" && !duties.value.can_read_audit)
    )
      setKind(
        duties.value.can_manage_users
          ? "users"
          : duties.value.can_read_audit
            ? "audit"
            : "namespaces",
      );
  }, [mode, duties.loading, duties.value, identity?.administrator, kind]);
  if (ui.sources.length === 0)
    return (
      <Empty title={t("Connect a team hub")}>
        <button onClick={() => ui.navigate("sources")}>
          {t("Add connection")}
        </button>
      </Empty>
    );
  return (
    <>
      {mode === "management" && (
        <details className="access-explainer">
          <summary>{t("How access works", "权限如何划分")}</summary>
          <p>
            {t(
              "Hub administrators manage accounts, groups and categories. Namespace roles are assigned to people or groups and can be combined.",
              "Hub 管理员管理账号、分组与分类。命名空间角色可分配给用户或分组，也可组合授予。",
            )}
          </p>
          <p>
            {t(
              "Reader: view · Maintainer: publish · Reviewer: review · Namespace admin: team settings and access. Authors cannot approve their own submissions.",
              "读者：查看；维护者：发布；审核者：审核；命名空间管理员：团队设置与授权。发布者不能审核自己的提交。",
            )}
          </p>
          <p>
            {t(
              "Readers must belong to a skill’s audience group. Maintainers, reviewers and namespace administrators retain access for their duties.",
              "普通阅读者需要属于技能的受众群组；维护者、审核者和命名空间管理员保留履职访问权。",
            )}
          </p>
          <p>
            {t(
              "Site name, private access and SSO are configured by the server operator. Edit team names and descriptions from the team page.",
              "站点名称、私有访问和单点登录由部署方在服务器配置中设置；团队名称与介绍在团队主页编辑。",
            )}
          </p>
        </details>
      )}
      {mode === "management" && (
        <div className="tabs">
          {tabs
            .filter(
              ([id]) =>
                !["publish", "submissions"].includes(id) &&
                (id !== "reports" ||
                  (identity?.administrator &&
                    participation.value?.supported)) &&
                (id !== "users" ||
                  identity?.administrator ||
                  duties.value?.can_manage_users) &&
                (id !== "audit" ||
                  identity?.administrator ||
                  duties.value?.can_read_audit),
            )
            .map(([id, label]) => (
              <button
                key={id}
                className={kind === id ? "selected" : ""}
                disabled={ui.busy}
                onClick={() => setKind(id)}
              >
                {id === "reports" ? t("Content reports", "内容举报") : t(label)}
              </button>
            ))}
        </div>
      )}
      {!source ? (
        <Empty title={t("Choose a hub")}>
          {t(
            "Every hub manages its own people, permissions and review process.",
          )}
        </Empty>
      ) : identityLoading ? (
        <p role="status">{t("Loading…", "加载中…")}</p>
      ) : !identity ? (
        <Empty title={t("Sign in to continue")}>
          {isWeb ? (
            <a href={signInURL()}>{t("Sign in")}</a>
          ) : (
            <button onClick={() => ui.navigate("sources")}>
              {t("Go to Connections")}
            </button>
          )}
        </Empty>
      ) : mode === "publish" ? (
        <PublishWizard key={`${source}/${identity.id}`} source={source} />
      ) : mode === "review" || mode === "submissions" ? (
        <ReviewWorkspace
          key={`${source}/${identity.id}`}
          source={source}
          identity={identity}
          mine={mode === "submissions"}
        />
      ) : kind === "reports" ? (
        <ContentReports key={source} source={source} />
      ) : (
        <Records
          key={`${source}/${kind}/${duties.value?.can_manage_users}/${duties.value?.can_read_audit}`}
          source={source}
          kind={kind}
          administrator={identity.administrator}
          canManageUsers={
            identity.administrator || !!duties.value?.can_manage_users
          }
          canReadAudit={
            identity.administrator || !!duties.value?.can_read_audit
          }
          currentUser={identity.id}
        />
      )}
    </>
  );
}
const emptyForm = (source: string, action: string): AdminForm => ({
  source,
  action,
  request_id: "",
  id: "",
  name: "",
  title: "",
  password: "",
  namespace: "",
  subject: "",
  revision: "0",
  enabled: true,
  administrator: false,
  subject_kind: Principal.PrincipalKind_PRINCIPAL_KIND_USER,
  role: Role.NamespaceRole_NAMESPACE_ROLE_READER,
});
function Records({
  source,
  kind,
  administrator,
  currentUser,
  canManageUsers,
  canReadAudit,
}: {
  source: string;
  kind: string;
  administrator: boolean;
  currentUser: string;
  canManageUsers: boolean;
  canReadAudit: boolean;
}) {
  const ui = useUI();
  const [page, setPage] = useState<AdminPage | null>(null),
    [activeFilter, setActiveFilter] = useState(""),
    [listing, setListing] = useState(kind),
    [edit, setEdit] = useState<AdminForm | null>(null);
  const users = useDirectory(
    source,
    kind === "grants" && administrator ? "users" : "",
  );
  const groups = useDirectory(
    source,
    kind === "grants" || kind === "groups" ? "groups" : "",
  );
  const title = tabs.find(([id]) => id === kind)?.[1] ?? kind;
  const loadGeneration = useRef(0);
  const load = (token = "", requestedKind = listing, query = activeFilter) => {
    const generation = ++loadGeneration.current;
    setListing(requestedKind);
    setActiveFilter(query);
    void ui.run(async () => {
      const result = await API.AdminList(source, requestedKind, query, token);
      if (generation === loadGeneration.current) setPage(result);
    });
  };
  useEffect(() => {
    if (
      kind !== "grants" &&
      (kind === "users"
        ? canManageUsers
        : kind === "audit"
          ? canReadAudit
          : true)
    )
      load();
  }, []);
  if (
    (kind === "users" && !canManageUsers) ||
    (kind === "audit" && !canReadAudit)
  )
    return (
      <Empty title={t("Additional access required", "需要相应职责权限")}>
        {t(
          "People administrators manage ordinary accounts; auditors can read audit logs. Ask a Hub administrator to assign the duty.",
          "人员管理员管理普通账号；审计员查看日志。请联系 Hub 管理员分配相应职责。",
        )}
      </Empty>
    );
  const action =
    kind === "namespaces"
      ? "namespace"
      : kind === "categories"
        ? "category"
        : kind === "groups"
          ? "group-add"
          : kind === "users"
            ? "user-add"
            : "grant";
  const canCreate =
    kind === "grants" ||
    (kind === "users" && canManageUsers) ||
    (administrator &&
      ["namespaces", "categories", "groups", "users"].includes(kind));
  const editRow = (row: AdminRow) => {
    const form = emptyForm(source, action);
    if (kind === "namespaces" || kind === "categories") {
      Object.assign(form, {
        name: row.name,
        title: row.title,
        revision: row.revision,
        enabled: row.enabled,
      });
    } else if (kind === "users") {
      Object.assign(form, {
        action: "user-state",
        id: row.id,
        name: row.name,
        enabled: row.enabled,
      });
    } else if (kind === "grants") {
      Object.assign(form, {
        namespace: row.namespace,
        subject: row.id,
        subject_kind: row.subject_kind,
        role: row.role,
        enabled: false,
      });
    }
    setEdit(form);
  };
  return (
    <>
      {edit && (
        <RecordEditor
          administrator={administrator}
          key={`${edit.action}/${edit.id}/${edit.name}/${edit.subject}`}
          initial={edit}
          close={() => setEdit(null)}
          saved={() => {
            setEdit(null);
            if (kind === "groups") groups.retry();
            if (edit.action === "user-admin" && edit.id === currentUser)
              ui.changed(
                t(
                  "Administrator access changed. Sign in again.",
                  "管理员权限已变更，请重新登录。",
                ),
              );
            else load();
          }}
        />
      )}
      <div className="section-heading">
        <h2>
          {listing === "members"
            ? groups.rows.find((r) => r.id === activeFilter)?.name ||
              t("Group members")
            : t(title)}
        </h2>
        <div className="actions">
          {listing === "members" && (
            <button disabled={ui.busy} onClick={() => load("", "groups", "")}>
              {t("Back to groups")}
            </button>
          )}
          {canCreate && listing !== "members" && (
            <button
              className="primary"
              disabled={ui.busy}
              onClick={() => {
                const form = emptyForm(source, action);
                if (kind === "grants") form.namespace = activeFilter;
                setEdit(form);
              }}
            >
              {kind === "grants"
                ? t("Add role grant", "添加角色授权")
                : kind === "namespaces"
                  ? t("Create namespace", "创建命名空间")
                  : kind === "categories"
                    ? t("Create category", "创建分类")
                    : kind === "users"
                      ? t("Create local user", "创建本地用户")
                      : t("Create access group", "创建权限分组")}
            </button>
          )}
          {listing === "members" && administrator && (
            <button
              onClick={() =>
                setEdit({ ...emptyForm(source, "member"), id: activeFilter })
              }
            >
              {t("Add member")}
            </button>
          )}
        </div>
      </div>
      {kind === "users" && (
        <details className="account-guide">
          <summary>{t("About accounts and access", "了解账号与权限")}</summary>
          <p>
            {t(
              "Create local accounts here. By default, verified enterprise sign-ins wait for administrator approval. Approve or reject requests below; enterprise users do not need a Hub password.",
              "可以在这里创建本地账号。默认情况下，已验证的企业身份须等待管理员审批；在下方批准或拒绝申请。企业用户无需 Hub 密码。",
            )}
          </p>
          <p>
            {t(
              "Use access groups and namespace roles to grant publishing or review access. Disabling an account revokes its Hub sessions immediately. Enterprise directory sync is not enabled.",
              "通过权限分组和命名空间角色分配发布、审核权限。禁用账号会立即撤销其 Hub 会话。目前未启用企业目录自动同步。",
            )}
          </p>
        </details>
      )}
      {kind === "grants" && (
        <div className="grant-scope">
          <DirectoryPicker
            source={source}
            kind="namespaces"
            label={t("Namespace", "命名空间")}
            value={activeFilter}
            onChange={(value) => {
              setEdit(null);
              setActiveFilter(value);
              loadGeneration.current++;
              setPage(null);
              if (value) load("", "grants", value);
            }}
          />
        </div>
      )}
      {page && (page.rows ?? []).length === 0 ? (
        <Empty title={t("Nothing here yet")}>
          {t("Create a record or check your filters and permissions.")}
        </Empty>
      ) : (
        <div className="list">
          {page?.rows?.map((row, index) => (
            <article key={`${row.id}/${row.role}/${index}`}>
              <div>
                <h3>
                  {kind === "grants"
                    ? row.subject_kind ===
                      Principal.PrincipalKind_PRINCIPAL_KIND_GROUP
                      ? groups.rows.find((g) => g.id === row.id)?.name ||
                        t("Unavailable group", "不可用的分组")
                      : (() => {
                          const user = users.rows.find((u) => u.id === row.id);
                          return user ? (
                            directoryLabel(user, "users")
                          ) : (
                            <GrantUserName
                              source={source}
                              namespace={row.namespace}
                              id={row.id}
                            />
                          );
                        })()
                    : (kind === "users" || listing === "members") && row.title
                      ? row.title
                      : row.name || row.id}
                </h3>
                {row.title && (
                  <p>
                    {kind === "users" || listing === "members"
                      ? row.name
                      : row.title}
                  </p>
                )}
                {kind === "audit" ? (
                  <>
                    <p className="muted">
                      {row.created_at} {t("· By")} {row.namespace}
                    </p>
                    {row.detail && (
                      <details className="audit-detail">
                        <summary>{t("Event details", "事件详情")}</summary>
                        <pre>{row.detail}</pre>
                      </details>
                    )}
                  </>
                ) : (
                  <>
                    {kind === "grants" &&
                      row.subject_kind ===
                        Principal.PrincipalKind_PRINCIPAL_KIND_USER && (
                        <p className="muted">
                          {users.rows.find((u) => u.id === row.id)?.name}
                        </p>
                      )}
                    {kind === "users" && row.oidc_issuer && (
                      <details className="audit-detail">
                        <summary>
                          {t(
                            "Verified enterprise identity",
                            "已验证的企业身份",
                          )}
                        </summary>
                        <p className="path">{row.oidc_issuer}</p>
                        <p className="path">{row.oidc_subject}</p>
                      </details>
                    )}
                    <div className="chips">
                      {(kind === "users" || listing === "members") && (
                        <>
                          <span>
                            {row.approval_pending
                              ? t("Awaiting approval", "待审批")
                              : row.enabled
                                ? t("Active account")
                                : t("Disabled")}
                          </span>
                          {row.administrator && <span>{t("Hub admin")}</span>}
                          {row.platform_roles?.includes(
                            Duty.PlatformRole_PLATFORM_ROLE_USER_ADMIN,
                          ) && (
                            <span>
                              {t("People administrator", "人员管理员")}
                            </span>
                          )}
                          {row.platform_roles?.includes(
                            Duty.PlatformRole_PLATFORM_ROLE_AUDITOR,
                          ) && <span>{t("Auditor", "审计员")}</span>}
                          <span>
                            {row.detail === "true"
                              ? t("External account", "第三方账号")
                              : t("Local account", "本地账号")}
                          </span>
                        </>
                      )}
                      {kind === "namespaces" && (
                        <span>
                          {row.enabled
                            ? t("Independent review")
                            : t("Direct publication")}
                        </span>
                      )}
                      {kind === "categories" && (
                        <span>
                          {row.enabled
                            ? t("Available for publication")
                            : t("Archived")}
                        </span>
                      )}
                      {kind === "grants" && (
                        <>
                          <span>
                            {row.subject_kind ===
                            Principal.PrincipalKind_PRINCIPAL_KIND_GROUP
                              ? t("Access groups")
                              : t("Users")}
                          </span>
                          <span>{roleLabel(row.role)}</span>
                          <span>{row.namespace}</span>
                        </>
                      )}
                    </div>
                  </>
                )}
              </div>
              <div className="actions wrap">
                {kind === "groups" && listing !== "members" && (
                  <button
                    disabled={ui.busy}
                    onClick={() => load("", "members", row.id)}
                  >
                    {t("View members")}
                  </button>
                )}
                {listing === "members" && administrator && (
                  <button
                    disabled={ui.busy}
                    onClick={() =>
                      setEdit({
                        ...emptyForm(source, "member"),
                        id: activeFilter,
                        subject: row.id,
                        enabled: false,
                      })
                    }
                  >
                    {t("Remove from group")}
                  </button>
                )}
                {kind === "users" &&
                  canManageUsers &&
                  !row.approval_pending && (
                    <>
                      {administrator &&
                        row.detail !== "true" &&
                        row.id !== currentUser &&
                        (administrator || !row.administrator) && (
                          <button
                            disabled={ui.busy}
                            onClick={() =>
                              setEdit({
                                ...emptyForm(source, "user-password"),
                                id: row.id,
                                name: row.name,
                              })
                            }
                          >
                            {t("Reset password", "重置密码")}
                          </button>
                        )}
                      {administrator && (
                        <PlatformRoles
                          source={source}
                          id={row.id}
                          name={row.title || row.name}
                          changed={() => load()}
                        />
                      )}
                      {administrator && (
                        <button
                          disabled={ui.busy || !row.enabled}
                          onClick={() =>
                            setEdit({
                              ...emptyForm(source, "user-admin"),
                              id: row.id,
                              name: row.name,
                              administrator: !row.administrator,
                            })
                          }
                        >
                          {row.administrator
                            ? t("Remove Hub admin", "移除 Hub 管理员")
                            : t("Make Hub admin", "设为 Hub 管理员")}
                        </button>
                      )}
                    </>
                  )}
                {kind === "users" && row.approval_pending && canManageUsers && (
                  <>
                    <button
                      className="primary"
                      disabled={ui.busy}
                      onClick={() =>
                        ui.confirm(
                          `${t("Approve enterprise access", "批准企业账号访问")} · ${row.title || row.name}`,
                          t(
                            "Approve this verified identity as an ordinary member. Namespace roles are assigned separately.",
                            "批准此已验证身份成为普通成员。命名空间角色需要另外分配。",
                          ),
                          async () => {
                            await API.AdminChange({
                              ...emptyForm(source, "user-state"),
                              id: row.id,
                              request_id: await API.NewRequestID(),
                              enabled: true,
                            });
                            load();
                          },
                          "primary",
                        )
                      }
                    >
                      {t("Approve access", "批准访问")}
                    </button>
                    <button
                      disabled={ui.busy}
                      onClick={() =>
                        ui.confirm(
                          `${t("Reject access request", "拒绝访问申请")} · ${row.title || row.name}`,
                          t(
                            "The account will stay disabled. Repeated sign-in cannot reopen this request.",
                            "账号将保持禁用，重复登录不会重新开启申请。",
                          ),
                          async () => {
                            await API.AdminChange({
                              ...emptyForm(source, "user-state"),
                              id: row.id,
                              request_id: await API.NewRequestID(),
                              enabled: false,
                            });
                            load();
                          },
                        )
                      }
                    >
                      {t("Reject", "拒绝")}
                    </button>
                  </>
                )}
                {["namespaces", "categories", "users", "grants"].includes(
                  kind,
                ) &&
                  !row.approval_pending &&
                  (kind !== "users" ||
                    (canManageUsers &&
                      (administrator ||
                        (!row.administrator &&
                          !row.platform_roles?.length &&
                          row.id !== currentUser)))) && (
                    <button disabled={ui.busy} onClick={() => editRow(row)}>
                      {kind === "grants" ? t("Revoke access") : t("Edit")}
                    </button>
                  )}
              </div>
            </article>
          ))}
        </div>
      )}
      {page?.next_page && (
        <button
          className="pager"
          disabled={ui.busy}
          onClick={() => load(page.next_page)}
        >
          {t("Next page")}
        </button>
      )}
    </>
  );
}
function RecordEditor({
  administrator,
  initial,
  close,
  saved,
}: {
  administrator: boolean;
  initial: AdminForm;
  close: () => void;
  saved: () => void;
}) {
  const ui = useUI();
  const mutation = useMutationID();
  const [form, setForm] = useState(initial);
  const set = <K extends keyof AdminForm>(key: K, value: AdminForm[K]) =>
    setForm((old) => ({ ...old, [key]: value }));
  const action = form.action;
  const grantAccess = useResource(async () => {
    if (action !== "grant" || !form.namespace) return null;
    const access = await API.NamespaceAccess(form.source, form.namespace);
    return {
      source: form.source,
      namespace: form.namespace,
      allowed: access.can_manage,
    };
  }, [action, form.source, form.namespace, ui.revision]);
  const grantAllowed =
    grantAccess.value?.source === form.source &&
    grantAccess.value?.namespace === form.namespace &&
    grantAccess.value.allowed;

  const title = (
    {
      namespace: t("Namespace policy"),
      category: t("Category settings"),
      "user-add": t("Create user"),
      "user-state": t("Account status"),
      "user-password": t("Reset password", "重置密码"),
      "user-admin": t("Hub administrator access", "Hub 管理员权限"),
      "group-add": t("Create access group"),
      member: t("Group members"),
      grant: t("Namespace role"),
    } as Record<string, string>
  )[action];
  return (
    <Modal wide title={title} onClose={close}>
      <form
        className="form-grid editor management-editor"
        onSubmit={(e) => {
          e.preventDefault();
          if (
            (action === "member" && (!form.id || !form.subject)) ||
            (action === "grant" &&
              (!form.namespace || !form.subject || !grantAllowed))
          )
            return;
          const save = async () => {
            const request = { ...form, request_id: "" };
            request.request_id = await mutation.get(request);
            await API.AdminChange(request);
            set("password", "");
            mutation.clear();
            saved();
          };
          if (action === "user-admin" || action === "user-password")
            ui.confirm(
              title,
              `${form.name} · ${t("All sessions for this account will be revoked.", "该账号的所有会话都会被撤销。")}`,
              save,
            );
          else void ui.run(save);
        }}
      >
        {["namespace", "category", "user-add", "group-add"].includes(
          action,
        ) && (
          <Field label={action === "user-add" ? t("Username") : t("Name")}>
            <input
              required
              value={form.name}
              readOnly={form.revision !== "0"}
              onChange={(e) => set("name", e.target.value)}
            />
          </Field>
        )}
        {action === "category" && (
          <Field label={t("Display name")}>
            <input
              required
              value={form.title}
              onChange={(e) => set("title", e.target.value)}
            />
          </Field>
        )}
        {(action === "user-add" || action === "user-password") && (
          <>
            <Field
              label={
                action === "user-password"
                  ? t("New password", "新密码")
                  : t("Initial password")
              }
              hint={t("12–72 bytes. Not shown again after creation.")}
            >
              <input
                required
                type="password"
                autoComplete="new-password"
                value={form.password}
                onChange={(e) => set("password", e.target.value)}
              />
            </Field>
            {action === "user-add" && administrator && (
              <label className="check">
                <input
                  type="checkbox"
                  checked={form.administrator}
                  onChange={(e) => set("administrator", e.target.checked)}
                />
                {t("Grant hub administrator access")}
              </label>
            )}
          </>
        )}
        {action === "user-admin" && (
          <p className="full">
            {form.name} ·{" "}
            {form.administrator
              ? t(
                  "Grant full Hub administration. This includes accounts, permissions and all skills.",
                  "授予完整 Hub 管理权限，包括账号、授权和所有技能。",
                )
              : t(
                  "Remove Hub administrator access. The last active administrator cannot be removed.",
                  "移除 Hub 管理权限，不能移除最后一个可用管理员。",
                )}
          </p>
        )}
        {action === "user-state" && (
          <p className="full">
            {t(
              "Disabling this account signs it out of Web, App and CLI immediately. It does not delete its published skills.",
              "禁用账号会立即撤销 Web、App 和 CLI 登录，不会删除已发布技能。",
            )}
            <br />
            {t("Account:")}
            {form.name}
          </p>
        )}
        {action === "member" && (
          <>
            <DirectoryPicker
              source={form.source}
              kind="groups"
              label={t("Access group", "权限分组")}
              value={form.id}
              onChange={(v) => set("id", v)}
              disabled={!!initial.id}
            />
            <DirectoryPicker
              source={form.source}
              kind="users"
              label={t("User", "用户")}
              value={form.subject}
              onChange={(v) => set("subject", v)}
              disabled={!!initial.subject}
            />
          </>
        )}
        {action === "grant" && (
          <>
            {form.namespace && !grantAllowed && (
              <div className="full grant-access-status" role="status">
                {grantAccess.loading ? (
                  t("Checking namespace access…", "正在检查空间权限…")
                ) : grantAccess.error ? (
                  <>
                    {t("Unable to check permissions.", "无法检查操作权限。")}{" "}
                    <button type="button" onClick={grantAccess.reload}>
                      {t("Try again")}
                    </button>
                  </>
                ) : !grantAllowed ? (
                  t(
                    "Only namespace or Hub administrators can manage role grants here. Choose a namespace you administer or ask an administrator.",
                    "只有该空间的管理员或 Hub 管理员可以分配角色。请选择你管理的空间，或联系管理员。",
                  )
                ) : null}
              </div>
            )}
            <DirectoryPicker
              source={form.source}
              kind="namespaces"
              label={t("Namespace", "命名空间")}
              value={form.namespace}
              onChange={(v) =>
                setForm((old) => ({ ...old, namespace: v, subject: "" }))
              }
              disabled={!!initial.subject}
            />
            <Field label={t("Assign to", "授权对象")}>
              <Select
                disabled={!!initial.subject || !grantAllowed}
                value={form.subject_kind}
                onChange={(e) =>
                  setForm((old) => ({
                    ...old,
                    subject_kind: Number(e.target.value),
                    subject: "",
                  }))
                }
              >
                <option value={Principal.PrincipalKind_PRINCIPAL_KIND_USER}>
                  {t("User", "用户")}
                </option>
                <option value={Principal.PrincipalKind_PRINCIPAL_KIND_GROUP}>
                  {t("Access group", "权限分组")}
                </option>
              </Select>
            </Field>
            <DirectoryPicker
              key={`${form.subject_kind}/${form.namespace}`}
              source={form.source}
              namespace={
                form.subject_kind ===
                Principal.PrincipalKind_PRINCIPAL_KIND_USER
                  ? form.namespace
                  : ""
              }
              kind={
                form.subject_kind ===
                Principal.PrincipalKind_PRINCIPAL_KIND_GROUP
                  ? "groups"
                  : "users"
              }
              label={
                form.subject_kind ===
                Principal.PrincipalKind_PRINCIPAL_KIND_GROUP
                  ? t("Access group", "权限分组")
                  : t("User", "用户")
              }
              value={form.subject}
              onChange={(v) => set("subject", v)}
              disabled={!!initial.subject || !grantAllowed}
            />
            <Field label={t("Role")}>
              <Select
                value={form.role}
                disabled={!!initial.subject || !grantAllowed}
                onChange={(e) => set("role", Number(e.target.value))}
              >
                <option value={Role.NamespaceRole_NAMESPACE_ROLE_READER}>
                  {t("Reader")}
                </option>
                <option value={Role.NamespaceRole_NAMESPACE_ROLE_MAINTAINER}>
                  {t("Maintainer (publish, withdraw)")}
                </option>
                <option value={Role.NamespaceRole_NAMESPACE_ROLE_REVIEWER}>
                  {t("Reviewer (approve, return)")}
                </option>
                <option value={Role.NamespaceRole_NAMESPACE_ROLE_ADMIN}>
                  {t("Space admin")}
                </option>
              </Select>
            </Field>
          </>
        )}
        {["namespace", "category", "user-state"].includes(action) && (
          <label className="check full">
            <input
              type="checkbox"
              checked={form.enabled}
              onChange={(e) => set("enabled", e.target.checked)}
            />
            {
              (
                {
                  namespace: t("Require independent review (no self-approval)"),
                  category: t("Available for new submissions"),
                  "user-state": t("Allow sign-in (disabling revokes sessions)"),
                  member: t("Keep group membership"),
                  grant: t("Grant this role (disable to revoke)"),
                } as Record<string, string>
              )[action]
            }
          </label>
        )}
        <div className="actions full">
          <button
            className="primary"
            disabled={
              ui.busy ||
              (action === "member" && (!form.id || !form.subject)) ||
              (action === "grant" &&
                (!form.namespace || !form.subject || !grantAllowed))
            }
          >
            {action === "member"
              ? form.enabled
                ? t("Add member")
                : t("Remove from group")
              : action === "grant"
                ? form.enabled
                  ? t("Grant role", "分配角色")
                  : t("Revoke access")
                : t("Save changes")}
          </button>
        </div>
      </form>
    </Modal>
  );
}

function GrantUserName({
  source,
  namespace,
  id,
}: {
  source: string;
  namespace: string;
  id: string;
}) {
  const directory = useDirectory(source, "users", namespace, id);
  const user = directory.rows.find((row) => row.id === id);
  return (
    <>
      {user
        ? directoryLabel(user, "users")
        : directory.loading
          ? t("Loading…", "加载中…")
          : t("Unavailable user", "用户不可用")}
    </>
  );
}
