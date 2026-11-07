import { communityWeb } from "./community-web";
import { t } from "./i18n";
import { createConnectTransport } from "@connectrpc/connect-web";
import {
  createHubClients,
  type PrincipalKind,
  type NamespaceRole,
  type Release,
  type PlatformRole,
  type ScanReport,
} from "../../../sdk/typescript/src/index";
import { parseDocument } from "yaml";
import { unzipSync } from "fflate";
import type { PlatformAPI } from "./platform";
import type {
  PublishForm,
  AdminRow,
} from "../bindings/github.com/skillhubhq/skillhub/desktop/models";
import type {
  ReleaseInfo,
  PreviewTarget,
  Source,
} from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
const clients = createHubClients(
  createConnectTransport({
    baseUrl: window.location.origin,
    interceptors: [
      (next) => async (request) => {
        request.header.set("X-SkillHub-Web", "1");
        return next(request);
      },
    ],
    fetch: (input, init) =>
      fetch(input, { ...init, credentials: "same-origin", redirect: "error" }),
  }),
);
const page = (token: string) => ({ pageSize: 30, pageToken: token });
const info = (r?: Release): ReleaseInfo => {
  if (!r) throw new Error(t("Version information unavailable"));
  return {
    id: r.releaseId,
    namespace: r.namespace,
    name: r.name,
    display_name_en: r.displayNameEn,
    display_name_zh: r.displayNameZh,
    icon: Number(r.icon),
    version: r.version,
    digest: r.digest,
    status: Number(r.status),
    visibility: Number(r.visibility),
    groups: r.groupIds,
    submitted_by: r.submittedBy,
    created_at: r.createdAt
      ? new Date(Number(r.createdAt.seconds) * 1000).toISOString()
      : "",
    reason: r.reviewReason,
    markdown: r.skillMarkdown,
  };
};
const governanceSupported = async () =>
  (await clients.registry.getRegistry({})).capabilities.includes(
    "governance.v1",
  );
const scanInfo = (r?: ScanReport) => ({
  supported: true,
  engine: r?.engine ?? "",
  digest: r?.digest ?? "",
  complete: r?.complete ?? false,
  blocking: r?.blocking ?? false,
  findings: (r?.findings ?? []).map((f) => ({
    rule: f.rule,
    path: f.path,
    line: f.line,
    blocking: f.blocking,
    message: f.message,
  })),
  scanned_at: r?.scannedAt
    ? new Date(Number(r.scannedAt.seconds) * 1000).toISOString()
    : "",
});
const coordinate = (value: string) => {
  const [path, version = ""] = value.split("@");
  const [source, namespace, name] = path.split("/");
  if (source !== "hub" || !namespace || !name)
    throw new Error(t("Invalid skill coordinate"));
  return { namespace, name, version };
};
export async function inspectArchive(archive: Uint8Array) {
  if (!archive.length || archive.length > 20 * 1024 * 1024)
    throw new Error(t("Package must be smaller than 20 MiB."));
  let total = 0,
    count = 0;
  const files = unzipSync(archive, {
    filter: (file) => {
      total += file.originalSize;
      count++;
      if (
        count > 2048 ||
        total > 80 * 1024 * 1024 ||
        file.originalSize > 8 * 1024 * 1024
      )
        throw new Error(t("Expanded package exceeds the size limit"));
      if (
        file.name.startsWith("/") ||
        file.name.includes("\\") ||
        file.name.split("/").includes("..")
      )
        throw new Error(t("Invalid package path"));
      return !file.name.endsWith("/");
    },
  });
  if (!files["SKILL.md"])
    throw new Error(t("SKILL.md must be at the package root"));
  const markdown = new TextDecoder("utf-8", { fatal: true }).decode(
    files["SKILL.md"],
  );
  const digest =
    "sha256:" +
    Array.from(
      new Uint8Array(
        await crypto.subtle.digest("SHA-256", new Uint8Array(archive)),
      ),
    )
      .map((v) => v.toString(16).padStart(2, "0"))
      .join("");
  const front = markdown.match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/);
  if (!front)
    throw new Error(
      t("SKILL.md requires YAML frontmatter.", "SKILL.md 缺少 YAML 元数据。"),
    );
  const doc = parseDocument(front[1], { uniqueKeys: true });
  if (doc.errors.length)
    throw new Error(
      t("Invalid SKILL.md metadata.", "SKILL.md 元数据格式错误。"),
    );
  const meta = doc.toJS({ maxAliasCount: 50 });
  if (
    !meta ||
    typeof meta.name !== "string" ||
    !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(meta.name) ||
    meta.name.length > 64 ||
    typeof meta.description !== "string" ||
    !meta.description.trim() ||
    [...meta.description].length > 1024
  )
    throw new Error(
      t(
        "SKILL.md needs a valid name and description.",
        "SKILL.md 需要有效的名称和描述。",
      ),
    );
  return {
    files,
    markdown,
    digest,
    name: meta.name as string,
    description: meta.description as string,
  };
}
async function preview(target: PreviewTarget) {
  const response = target.submission_id
    ? await clients.publishing.getSubmission({
        releaseId: target.submission_id,
      })
    : await clients.catalog.downloadRelease({
        namespace: target.namespace,
        name: target.name,
        version: target.version,
      });
  const release = info(response.release);
  const bundle = await inspectArchive(response.archive);
  if (
    bundle.digest !== release.digest ||
    (target.digest && target.digest !== release.digest)
  )
    throw new Error(t("Content changed. Reload the preview."));
  return {
    release: { ...release, markdown: bundle.markdown },
    target: { ...target, digest: release.digest },
    files: Object.entries(bundle.files).map(([path, data]) => ({
      path,
      size: data.length,
      executable: false,
    })),
    bundle,
  };
}
export async function publishPackage(form: PublishForm, archive: Uint8Array) {
  if (form.display_name_en || form.display_name_zh || form.icon) {
    const registry = await clients.registry.getRegistry({});
    if (!registry.capabilities.includes("catalog.presentation.v1"))
      throw new Error(
        t(
          "This Hub needs an upgrade to accept display names and icons.",
          "此 Hub 需要升级后才能接收展示名称和图标。",
        ),
      );
  }
  return info(
    (
      await clients.publishing.submitRelease({
        mutation: { idempotencyKey: form.request_id },
        namespace: form.namespace,
        version: form.version,
        archive,
        visibility: Number(form.visibility),
        groupIds: form.groups ?? [],
        category: form.category,
        tags: form.tags ?? [],
        displayNameEn: form.display_name_en,
        displayNameZh: form.display_name_zh,
        icon: Number(form.icon),
      })
    ).release,
  );
}
const supported: Partial<PlatformAPI> = {
  ...communityWeb(clients),
  ReviewTasks: async (_source, token) => {
    if (
      !(await clients.registry.getRegistry({})).capabilities.includes(
        "governance.tasks.v1",
      )
    )
      return {
        supported: false,
        tasks: [],
        total: 0,
        oldest_at: "",
        next_page: "",
      };
    const r = await clients.governance.listReviewTasks({ page: page(token) });
    return {
      supported: true,
      total: r.total,
      oldest_at: r.oldestAt
        ? new Date(Number(r.oldestAt.seconds) * 1000).toISOString()
        : "",
      next_page: r.page?.nextPageToken ?? "",
      tasks: r.tasks.map((n) => ({
        release_id: n.releaseId,
        coordinate: n.coordinate,
        created_at: n.createdAt
          ? new Date(Number(n.createdAt.seconds) * 1000).toISOString()
          : "",
      })),
    };
  },
  SetFeaturedSkill: async (_source, namespace, name, featured, key) => {
    await clients.governance.setFeaturedSkill({
      namespace,
      name,
      featured,
      mutation: { idempotencyKey: key },
    });
  },
  NewRequestID: async () => crypto.randomUUID(),
  PlatformAccess: async (_source, id) => {
    if (!(await governanceSupported()))
      return {
        supported: false,
        user_id: "",
        roles: [],
        can_curate: false,
        can_manage_users: false,
        can_read_audit: false,
        administrator: false,
      };
    const r = await clients.governance.getPlatformAccess({ userId: id });
    return {
      supported: true,
      user_id: r.userId,
      roles:
        r.roles as unknown as import("../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models").PlatformRole[],
      can_curate:
        r.administrator &&
        (await clients.registry.getRegistry({})).capabilities.includes(
          "catalog.discovery.v1",
        ),
      can_manage_users: r.canManageUsers,
      can_read_audit: r.canReadAudit,
      administrator: r.administrator,
    };
  },
  SetPlatformRole: async (_source, id, role, granted, key) => {
    await clients.governance.setPlatformRole({
      userId: id,
      role: role as unknown as PlatformRole,
      granted,
      mutation: { idempotencyKey: key },
    });
  },
  ScanReport: async (_source, id) => {
    if (!(await governanceSupported()))
      return { ...scanInfo(), supported: false };
    return scanInfo(
      (await clients.governance.getScanReport({ releaseId: id })).report,
    );
  },
  ScanRelease: async (_source, id, key) =>
    scanInfo(
      (
        await clients.governance.scanRelease({
          releaseId: id,
          mutation: { idempotencyKey: key },
        })
      ).report,
    ),
  Notifications: async (_source, token) => {
    if (!(await governanceSupported()))
      return { supported: false, items: [], next_page: "" };
    const r = await clients.governance.listNotifications({ page: page(token) });
    return {
      supported: true,
      next_page: r.page?.nextPageToken ?? "",
      items: r.notifications.map((n) => ({
        id: n.notificationId,
        release_id: n.releaseId,
        coordinate: n.coordinate,
        approved: n.approved,
        read: n.read,
        created_at: n.createdAt
          ? new Date(Number(n.createdAt.seconds) * 1000).toISOString()
          : "",
      })),
    };
  },
  MarkNotificationRead: async (_source, id) => {
    await clients.governance.markNotificationRead({ notificationId: id });
  },
  ListSources: async () => {
    const r = await clients.registry.getRegistry({});
    return [
      {
        alias: "hub",
        url: location.origin,
        registry_id: r.registryId,
        display_name: r.displayName,
        private: r.privateMode,
        credential_ref: "",
      } as unknown as Source,
    ];
  },
  Identity: async () => {
    const i = (await clients.auth.getIdentity({})).identity;
    if (!i) throw new Error("authentication required");
    return {
      id: i.userId,
      username: i.username,
      administrator: i.administrator,
      federated: i.federated,
    };
  },
  Search: async (_source, query, category, tags, token, order, featured) => {
    try {
      if (
        (order || featured) &&
        !(await clients.registry.getRegistry({})).capabilities.includes(
          "catalog.discovery.v1",
        )
      )
        throw new Error(
          "This Hub does not support this catalog order or featured filter. Choose default name order.",
        );
      const r = await clients.catalog.searchSkills({
        order: Number(order),
        featuredOnly: featured,
        query,
        category,
        tags: tags ?? [],
        page: page(token),
      });
      return [
        {
          source: "hub",
          skills: r.skills.map((s) => ({
            featured: s.featured,
            namespace: s.namespace,
            name: s.name,
            display_name_en: s.displayNameEn,
            display_name_zh: s.displayNameZh,
            icon: Number(s.icon),
            description: s.description,
            category: s.category,
            tags: s.tags,
            version: s.latestVersion,
          })),
          next_page: r.page?.nextPageToken ?? "",
          error: "",
        },
      ];
    } catch (e) {
      return [
        {
          source: "hub",
          skills: [],
          next_page: "",
          error: e instanceof Error ? e.message : String(e),
        },
      ];
    }
  },
  GetSkill: async (_source, namespace, name) => {
    const s = (await clients.catalog.getSkill({ namespace, name })).skill;
    if (!s) throw new Error("Skill unavailable");
    return {
      featured: s.featured,
      namespace: s.namespace,
      name: s.name,
      display_name_en: s.displayNameEn,
      display_name_zh: s.displayNameZh,
      icon: Number(s.icon),
      description: s.description,
      category: s.category,
      tags: s.tags,
      version: s.latestVersion,
    };
  },
  NamespaceAccess: async (_source, namespace) => {
    const r = await clients.admin.getNamespaceAccess({ namespace });
    return {
      can_manage: r.canManage,
      can_publish: r.canPublish,
      can_review: r.canReview,
    };
  },
  LookupUsers: async (_source, namespace, query, token) => {
    const r = await clients.admin.lookupNamespaceUsers({
      namespace,
      query,
      page: page(token),
    });
    return {
      rows: r.users.map((u) => ({
        id: u.userId,
        name: u.username,
        title: u.displayName,
        enabled: !u.disabled,
        administrator: false,
        approval_pending: false,
        platform_roles: [],
        oidc_issuer: "",
        oidc_subject: "",
        revision: "0",
        namespace: "",
        subject_kind: 0,
        role: 0,
        detail: "",
        created_at: "",
      })),
      next_page: r.page?.nextPageToken ?? "",
    };
  },
  ChangePassword: async (_source, current, password) => {
    await clients.auth.changePassword({
      currentPassword: current,
      newPassword: password,
    });
  },
  FilterSubmissions: async (_source, query, status, mine, token) => {
    const r = await clients.publishing.listSubmissions({
      query,
      status: Number(status),
      mine,
      page: page(token),
    });
    return {
      releases: r.releases.map(info),
      next_page: r.page?.nextPageToken ?? "",
    };
  },
  Releases: async (value, token) => {
    const r = await clients.catalog.listReleases({
      ...coordinate(value),
      page: page(token),
    });
    return {
      releases: r.releases.map(info),
      next_page: r.page?.nextPageToken ?? "",
    };
  },
  Preview: async (value) =>
    preview({
      source: "hub",
      ...coordinate(value),
      submission_id: "",
      digest: "",
    }),
  PreviewSubmission: async (_source, id) =>
    preview({
      source: "hub",
      namespace: "",
      name: "",
      version: "",
      submission_id: id,
      digest: "",
    }),
  ReadPreviewFile: async (target, path) => {
    const p = await preview(target);
    const bytes = p.bundle.files[path];
    if (!bytes) throw new Error(t("File unavailable"));
    try {
      return {
        path,
        text: true,
        content: new TextDecoder("utf-8", { fatal: true }).decode(bytes),
      };
    } catch {
      return { path, text: false, content: "" };
    }
  },
  Submissions: async (_source, namespace, token) => {
    const r = await clients.publishing.listSubmissions({
      namespace,
      page: page(token),
    });
    return {
      releases: r.releases.map(info),
      next_page: r.page?.nextPageToken ?? "",
    };
  },
  Review: async (_s, id, reason, key, approve) =>
    info(
      (
        await clients.publishing.reviewRelease({
          releaseId: id,
          reason,
          approve,
          mutation: { idempotencyKey: key },
        })
      ).release,
    ),
  Yank: async (_s, id, reason, key) =>
    info(
      (
        await clients.publishing.yankRelease({
          releaseId: id,
          reason,
          mutation: { idempotencyKey: key },
        })
      ).release,
    ),
  AdminList: async (_source, kind, _filter, token) => {
    const blank: AdminRow = {
      approval_pending: false,
      platform_roles: [],
      oidc_issuer: "",
      oidc_subject: "",
      id: "",
      name: "",
      title: "",
      revision: "0",
      enabled: false,
      administrator: false,
      namespace: "",
      subject_kind: 0,
      role: 0,
      detail: "",
      created_at: "",
    };
    if (kind === "categories") {
      const r = await clients.admin.listCategories({ page: page(token) });
      return {
        rows: r.categories.map((c) => ({
          ...blank,
          id: c.name,
          name: c.name,
          title: c.title,
          enabled: !c.archived,
          revision: String(c.revision),
        })),
        next_page: r.page?.nextPageToken ?? "",
      };
    }
    if (kind === "submission-namespaces" || kind === "namespaces") {
      const r = await clients.admin.listNamespaces({
        forSubmission: kind === "submission-namespaces",
        page: page(token),
      });
      return {
        rows: r.namespaces.map((n) => ({
          ...blank,
          id: n.name,
          name: n.name,
          enabled: n.reviewRequired,
          revision: String(n.revision),
        })),
        next_page: r.page?.nextPageToken ?? "",
      };
    }
    if (kind === "groups") {
      const r = await clients.admin.listGroups({ page: page(token) });
      return {
        rows: r.groups.map((g) => ({ ...blank, id: g.groupId, name: g.name })),
        next_page: r.page?.nextPageToken ?? "",
      };
    }
    if (kind === "users" || kind === "members") {
      const r =
        kind === "users"
          ? await clients.admin.listUsers({ page: page(token) })
          : await clients.admin.listGroupMembers({
              page: page(token),
              groupId: _filter,
            });
      return {
        rows: r.users.map((u) => ({
          ...blank,
          id: u.userId,
          name: u.username,
          title: u.displayName,
          detail: String(u.federated),
          enabled: !u.disabled,
          administrator: u.administrator,
          approval_pending: u.approvalPending,
          platform_roles: u.platformRoles.map(Number),
          oidc_issuer: u.oidcIssuer,
          oidc_subject: u.oidcSubject,
        })),
        next_page: r.page?.nextPageToken ?? "",
      };
    }
    if (kind === "grants") {
      const r = await clients.admin.listNamespaceGrants({
        page: page(token),
        namespace: _filter,
      });
      return {
        rows: r.grants.map((g) => ({
          ...blank,
          id: g.subjectId,
          namespace: g.namespace,
          subject_kind: Number(g.subjectKind),
          role: Number(g.role),
        })),
        next_page: r.page?.nextPageToken ?? "",
      };
    }
    if (kind === "audit") {
      const r = await clients.admin.listAudit({ page: page(token) });
      return {
        rows: r.events.map((e) => ({
          ...blank,
          id: String(e.sequence),
          name: e.action,
          title: e.resource,
          detail: e.detail,
          namespace: e.actorId,
          created_at: e.createdAt
            ? new Date(Number(e.createdAt.seconds) * 1000).toISOString()
            : "",
        })),
        next_page: r.page?.nextPageToken ?? "",
      };
    }
    throw new Error(t("Unknown management view", "未知的管理页面"));
  },
  AdminChange: async (f) => {
    const mutation = { idempotencyKey: f.request_id };
    switch (f.action) {
      case "user-add":
        await clients.admin.createUser({
          mutation,
          username: f.name,
          password: f.password,
          administrator: f.administrator,
        });
        break;
      case "user-password":
        await clients.admin.resetUserPassword({
          mutation,
          userId: f.id,
          password: f.password,
        });
        break;
      case "user-admin":
        await clients.admin.setUserAdministrator({
          mutation,
          userId: f.id,
          administrator: f.administrator,
        });
        break;
      case "user-state":
        await clients.admin.setUserDisabled({
          mutation,
          userId: f.id,
          disabled: !f.enabled,
        });
        break;
      case "group-add":
        await clients.admin.createGroup({ mutation, name: f.name });
        break;
      case "member":
        await clients.admin.setGroupMember({
          mutation,
          groupId: f.id,
          userId: f.subject,
          member: f.enabled,
        });
        break;
      case "grant":
        await clients.admin.setNamespaceGrant({
          mutation,
          namespace: f.namespace,
          subjectId: f.subject,
          subjectKind: Number(f.subject_kind) as PrincipalKind,
          role: Number(f.role) as NamespaceRole,
          granted: f.enabled,
        });
        break;
      case "namespace":
        await clients.admin.putNamespace({
          mutation,
          name: f.name,
          reviewRequired: f.enabled,
          expectedRevision: BigInt(f.revision),
        });
        break;
      case "category":
        await clients.admin.putCategory({
          mutation,
          name: f.name,
          title: f.title,
          archived: !f.enabled,
          expectedRevision: BigInt(f.revision),
        });
        break;
      default:
        throw new Error(t("Unknown management action", "未知的管理操作"));
    }
  },
};
export const webAPI = new Proxy(supported, {
  get(target, key) {
    return (
      target[key as keyof PlatformAPI] ??
      (async () => {
        throw new Error(t("This action requires the desktop app"));
      })
    );
  },
}) as PlatformAPI;

// 下载仍走同源鉴权 RPC，并验证摘要；不会通过公共链接暴露私有包。
export async function downloadSkillZIP(coordinateText: string) {
  const target = coordinate(`hub/${coordinateText}`);
  const response = await clients.catalog.downloadRelease(target);
  const bundle = await inspectArchive(response.archive);
  if (
    !response.release ||
    bundle.digest !== response.digest ||
    response.release.digest !== bundle.digest
  )
    throw new Error(t("Package verification failed.", "技能包校验失败。"));
  const blob = new Blob([new Uint8Array(response.archive)], {
    type: "application/zip",
  });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${target.namespace}-${target.name}-${target.version}.zip`;
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 60000);
}
