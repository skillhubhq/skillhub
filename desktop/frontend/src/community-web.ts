import type { NamespaceRole } from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";
import type {
  createHubClients,
  Profile,
  Team,
  Comment,
  Contribution,
  Discussion,
} from "../../../sdk/typescript/src/index";
import type { PlatformAPI } from "./platform";
const profile = (v?: Profile) => {
  if (!v) throw new Error("Profile unavailable");
  return {
    user_id: v.userId,
    username: v.username,
    display_name: v.displayName,
    bio: v.bio,
    avatar: v.avatar,
    revision: Number(v.revision),
    can_edit: v.canEdit,
  };
};
const team = (v?: Team) => {
  if (!v) throw new Error("Team unavailable");
  return {
    namespace: v.namespace,
    display_name: v.displayName,
    bio: v.bio,
    revision: Number(v.revision),
    can_edit: v.canEdit,
    can_view_members: v.canViewMembers,
  };
};
const contribution = (v: Contribution) => ({
  release_id: v.releaseId,
  namespace: v.namespace,
  name: v.name,
  display_name_en: v.displayNameEn,
  display_name_zh: v.displayNameZh,
  icon: Number(v.icon),
  version: v.version,
  description: v.description,
  author: profile(v.author),
});
const comment = (v?: Comment) => {
  if (!v) throw new Error("Comment unavailable");
  return {
    comment_id: v.commentId,
    release_id: v.releaseId,
    parent_id: v.parentId,
    author: profile(v.author),
    body: v.body,
    created_at: new Date(
      Number(v.createdAt?.seconds ?? 0) * 1000,
    ).toISOString(),
    deleted: v.deleted,
    can_delete: v.canDelete,
  };
};
const discussion = (v?: Discussion) => {
  if (!v) throw new Error("Discussion unavailable");
  return {
    like_count: Number(v.likeCount),
    comment_count: Number(v.commentCount),
    liked: v.liked,
    bookmarked: v.bookmarked,
    can_interact: v.canInteract,
  };
};
export function communityWeb(
  clients: ReturnType<typeof createHubClients>,
): Partial<PlatformAPI> {
  const api = clients.community;
  const page = (pageToken: string) => ({ pageToken, pageSize: 30 });
  return {
    CommunitySite: async () => {
      const v = await clients.registry.getRegistry({});
      return {
        privacy_url: v.privacyUrl,
        terms_url: v.termsUrl,
        contact_url: v.contactUrl,
        filing_text: v.filingText,
        filing_url: v.filingUrl,
      };
    },
    CommunityParticipation: async () => {
      const supported = (
        await clients.registry.getRegistry({})
      ).capabilities.includes("community.moderation.v1");
      const v = supported ? await api.getParticipation({}) : null;
      return {
        supported,
        community_mode: v?.communityMode ?? false,
        personal_namespace: v?.personalNamespace ?? "",
        versions: Number(v?.versions ?? 0),
        version_limit: Number(v?.versionLimit ?? 0),
        submissions_today: Number(v?.submissionsToday ?? 0),
        submission_limit: Number(v?.submissionLimit ?? 0),
        storage_bytes: Number(v?.storageBytes ?? 0),
        storage_limit: Number(v?.storageLimit ?? 0),
      };
    },
    CommunityReport: async (_s, releaseId, commentId, reason, key) =>
      (
        await api.reportContent({
          releaseId,
          commentId,
          reason,
          mutation: { idempotencyKey: key },
        })
      ).id,
    CommunityReports: async (_s, resolved, token) => {
      const v = await api.listReports({ resolved, page: page(token) });
      return {
        items: v.reports.map((r) => ({
          id: r.id,
          reporter: r.reporter,
          release_id: r.releaseId,
          comment_id: r.commentId,
          reason: r.reason,
          resolved: r.resolved,
          resolution: r.resolution,
          coordinate: r.coordinate,
          content: r.content,
          created_at: new Date(
            Number(r.createdAt?.seconds ?? 0) * 1000,
          ).toISOString(),
        })),
        next_page: v.page?.nextPageToken ?? "",
      };
    },
    CommunityResolveReport: async (_s, id, action, note, key) => {
      await api.resolveReport({
        id,
        action: Number(action),
        note,
        mutation: { idempotencyKey: key },
      });
      return true;
    },
    CommunityUpdateAvatar: async (_s, image, revision, key) =>
      profile(
        (
          await api.updateAvatar({
            image: Uint8Array.from(atob(image ?? ""), (c) => c.charCodeAt(0)),
            expectedRevision: BigInt(revision),
            mutation: { idempotencyKey: key },
          })
        ).profile,
      ),
    CommunityGetProfile: async (_s, id) =>
      profile((await api.getProfile({ userId: id })).profile),
    CommunityUpdateProfile: async (_s, displayName, bio, revision, key) =>
      profile(
        (
          await api.updateProfile({
            displayName,
            bio,
            expectedRevision: BigInt(revision),
            mutation: { idempotencyKey: key },
          })
        ).profile,
      ),
    CommunityGetTeam: async (_s, namespace) =>
      team((await api.getTeam({ namespace })).team),
    CommunityUpdateTeam: async (
      _s,
      namespace,
      displayName,
      bio,
      revision,
      key,
    ) =>
      team(
        (
          await api.updateTeam({
            namespace,
            displayName,
            bio,
            expectedRevision: BigInt(revision),
            mutation: { idempotencyKey: key },
          })
        ).team,
      ),
    CommunityListTeams: async (_s, token) => {
      const r = await api.listTeams({ page: page(token) });
      return {
        items: r.teams.map(team),
        next_page: r.page?.nextPageToken ?? "",
      };
    },
    CommunityListMembers: async (_s, namespace, token) => {
      const r = await api.listMembers({ namespace, page: page(token) });
      return {
        items: r.members.map((v) => ({
          profile: profile(v.profile),
          roles: v.roles.map((r) => Number(r) as NamespaceRole),
        })),
        next_page: r.page?.nextPageToken ?? "",
      };
    },
    CommunityListContributions: async (_s, userId, namespace, token) => {
      const r = await api.listContributions({
        userId,
        namespace,
        page: page(token),
      });
      return {
        items: r.contributions.map(contribution),
        next_page: r.page?.nextPageToken ?? "",
      };
    },
    CommunityListBookmarks: async (_s, token) => {
      const r = await api.listBookmarks({ page: page(token) });
      return {
        items: r.contributions.map(contribution),
        next_page: r.page?.nextPageToken ?? "",
      };
    },
    CommunityGetDiscussion: async (_s, releaseId) =>
      discussion((await api.getDiscussion({ releaseId })).discussion),
    CommunityListComments: async (_s, releaseId, parentId, token) => {
      const r = await api.listComments({
        releaseId,
        parentId,
        page: page(token),
      });
      return {
        items: r.comments.map(comment),
        next_page: r.page?.nextPageToken ?? "",
      };
    },
    CommunityCreateComment: async (_s, releaseId, parentId, body, key) =>
      comment(
        (
          await api.createComment({
            releaseId,
            parentId,
            body,
            mutation: { idempotencyKey: key },
          })
        ).comment,
      ),
    CommunityDeleteComment: async (_s, commentId, key) => {
      await api.deleteComment({ commentId, mutation: { idempotencyKey: key } });
      return true;
    },
    CommunitySetReaction: async (_s, releaseId, bookmark, active, key) =>
      discussion(
        (
          await api.setReaction({
            releaseId,
            bookmark,
            active,
            mutation: { idempotencyKey: key },
          })
        ).discussion,
      ),
  };
}
