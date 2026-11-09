export type WorkspaceAccess = {
  publish: boolean;
  review: boolean;
  manage: boolean;
};
export const noWorkspaceAccess: WorkspaceAccess = {
  publish: false,
  review: false,
  manage: false,
};
type AccessAPI = {
  PlatformAccess(
    source: string,
    user: string,
  ): Promise<{
    can_manage_users: boolean;
    can_read_audit: boolean;
    roles: number[] | null;
  }>;
  AdminList(
    source: string,
    kind: string,
    query: string,
    token: string,
  ): Promise<{
    rows: { name: string; archived: boolean }[] | null;
    next_page: string;
  }>;
  NamespaceAccess(
    source: string,
    namespace: string,
  ): Promise<{
    can_publish: boolean;
    can_review: boolean;
    can_manage: boolean;
  }>;
};
// 导航使用服务端计算的有效权限，包含群组授权，并完整遍历命名空间分页。
export async function loadWorkspaceAccess(
  api: AccessAPI,
  source: string,
  administrator: boolean,
): Promise<WorkspaceAccess> {
  if (administrator) return { publish: true, review: true, manage: true };
  const duties = await api.PlatformAccess(source, "");
  const access = {
    ...noWorkspaceAccess,
    manage: duties.can_manage_users || duties.can_read_audit,
  };
  let token = "";
  const seen = new Set<string>();
  do {
    const page = await api.AdminList(source, "namespaces", "", token);
    const grants = await Promise.all(
      (page.rows ?? [])
        .filter((row) => !row.archived)
        .map((row) => api.NamespaceAccess(source, row.name)),
    );
    for (const grant of grants) {
      access.publish ||= grant.can_publish;
      access.review ||= grant.can_review;
      access.manage ||= grant.can_manage;
    }
    token = page.next_page;
    if (token && seen.has(token)) throw new Error("Repeated namespace cursor");
    seen.add(token);
  } while (token);
  return access;
}
export function canOpenWorkspacePage(access: WorkspaceAccess, page: string) {
  if (page === "publish" || page === "submissions") return access.publish;
  if (page === "review") return access.review;
  if (page === "management") return access.manage;
  return true;
}
