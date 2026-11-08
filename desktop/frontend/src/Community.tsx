import { useState } from "react";
import { API, Field, Modal, roleLabel, useUI } from "./ui";
import { t } from "./i18n";
import {
  Avatar,
  Contributions,
  Person,
  ResourceState,
  useMutationKey,
  useResource,
  type Contribution,
  type Member,
  type Page,
  type Profile,
  type Team,
} from "./community-ui";
import { TeamContributors } from "./SkillPeople";
export function Community({ mode }: { mode: string }) {
  const ui = useUI(),
    params = new URLSearchParams(location.hash.split("?")[1] ?? "");
  const selectedSource = ui.activeSource;
  if (!selectedSource) return <p>{t("Connect your first hub")}</p>;
  return (
    <div className="community-page">
      <CommunityContent
        key={`${selectedSource}/${mode}/${params}`}
        source={selectedSource}
        mode={mode}
        id={params.get("id") ?? ""}
        namespace={params.get("team") ?? ""}
      />
    </div>
  );
}
function CommunityContent({
  source,
  mode,
  id,
  namespace,
}: {
  source: string;
  mode: string;
  id: string;
  namespace: string;
}) {
  const ui = useUI(),
    [editing, setEditing] = useState(false),
    [teamFilter, setTeamFilter] = useState("");
  const resource = useResource(async () => {
    if (mode === "bookmarks")
      return {
        kind: "saved" as const,
        page: await API.CommunityListBookmarks(source, ""),
      };
    if (mode === "profile")
      return {
        kind: "profile" as const,
        profile: await API.CommunityGetProfile(source, id),
      };
    if (namespace)
      return {
        kind: "team" as const,
        team: await API.CommunityGetTeam(source, namespace),
      };
    return {
      kind: "teams" as const,
      page: await API.CommunityListTeams(source, ""),
    };
  }, [source, mode, id, namespace, ui.revision]);
  const value = resource.value;
  const card =
    value?.kind === "profile"
      ? value.profile
      : value?.kind === "team"
        ? value.team
        : null;
  return (
    <ResourceState {...resource} retry={resource.reload}>
      {value?.kind === "teams" && (
        <>
          <div className="section-intro">
            <h2>
              {t(
                "Good work comes from good company.",
                "好方法，来自一起做事的人。",
              )}
            </h2>
            <p>
              {t(
                "Meet the teams behind the skills in this hub.",
                "认识这个 Hub 中分享技能的团队。",
              )}
            </p>
          </div>
          <input
            className="team-filter"
            aria-label={t("Find a team", "查找团队")}
            placeholder={t(
              "Filter teams by name or interest…",
              "按名称或方向筛选团队…",
            )}
            value={teamFilter}
            onChange={(e) => setTeamFilter(e.target.value)}
          />
          <div className="team-grid">
            {value.page.items
              ?.filter((team) =>
                `${team.display_name} ${team.namespace} ${team.bio}`
                  .toLowerCase()
                  .includes(teamFilter.toLowerCase()),
              )
              .map((team) => (
                <button
                  className="team-card"
                  key={team.namespace}
                  onClick={() =>
                    ui.navigate(
                      `teams?source=${encodeURIComponent(source)}&team=${encodeURIComponent(team.namespace)}`,
                    )
                  }
                >
                  <Avatar name={team.display_name} size="medium" />
                  <h3>{team.display_name}</h3>
                  <span className="handle">/{team.namespace}</span>
                  <p>
                    {team.bio ||
                      t(
                        "A shared space for useful skills.",
                        "分享实用技能的团队空间。",
                      )}
                  </p>
                  <span className="text-link">
                    {t("Visit team", "查看团队")}
                  </span>
                </button>
              ))}
          </div>
          {teamFilter &&
            !value.page.items?.some((team) =>
              `${team.display_name} ${team.namespace} ${team.bio}`
                .toLowerCase()
                .includes(teamFilter.toLowerCase()),
            ) && (
              <div className="quiet-empty">
                <p>{t("No teams match this filter.", "没有匹配的团队。")}</p>
                <button onClick={() => setTeamFilter("")}>
                  {t("Clear filters", "清除筛选")}
                </button>
              </div>
            )}
          {!value.page.items?.length && (
            <div className="quiet-empty">
              {t(
                "Teams appear here when you have access to their space or published skills.",
                "当你有权访问团队空间或已发布技能时，团队会显示在这里。",
              )}
            </div>
          )}
          {value.page.next_page && (
            <button
              onClick={() =>
                void ui.run(async () => {
                  const p = await API.CommunityListTeams(
                    source,
                    value.page.next_page,
                  );
                  resource.setValue({
                    ...value,
                    page: {
                      items: [...(value.page.items ?? []), ...(p.items ?? [])],
                      next_page: p.next_page,
                    },
                  });
                })
              }
            >
              {t("Load more")}
            </button>
          )}
        </>
      )}
      {value?.kind === "saved" && (
        <>
          <div className="section-intro">
            <h2>{t("Saved skills", "已收藏的技能")}</h2>
            <p>
              {t(
                "A private collection of exact versions. Only you can see your saves.",
                "收藏精确版本，只有你自己能看到。",
              )}
            </p>
          </div>
          <Contributions source={source} items={value.page.items ?? []} />
          {!value.page.items?.length && (
            <div className="quiet-empty">
              <h3>{t("No saved skills yet.", "还没有收藏技能。")}</h3>
              <p>
                {t(
                  "Save a skill from its detail page to find it here later.",
                  "在技能详情中收藏，即可在这里再次找到它。",
                )}
              </p>
              <button onClick={() => ui.navigate("discover")}>
                {t("Explore skills", "浏览技能")}
              </button>
            </div>
          )}
          {value.page.next_page && (
            <button
              onClick={() =>
                void ui.run(async () => {
                  const p = await API.CommunityListBookmarks(
                    source,
                    value.page.next_page,
                  );
                  resource.setValue({
                    ...value,
                    page: {
                      items: [...(value.page.items ?? []), ...(p.items ?? [])],
                      next_page: p.next_page,
                    },
                  });
                })
              }
            >
              {t("Load more")}
            </button>
          )}
        </>
      )}
      {card && (
        <>
          <div
            className={`profile-heading ${value?.kind === "profile" ? "person-heading" : "team-heading"}`}
          >
            <Avatar
              name={card.display_name}
              image={
                value?.kind === "profile" ? value.profile.avatar : undefined
              }
              size="large"
            />
            <div>
              <h2>{card.display_name}</h2>
              <p className="handle">
                {value?.kind === "profile"
                  ? t("Community member", "社区成员")
                  : "/" + (card as Team).namespace}
              </p>
              <p className="profile-bio">
                {card.bio ||
                  (value?.kind === "profile"
                    ? t("No introduction yet.", "还没有个人介绍。")
                    : t(
                        "A shared space for useful skills.",
                        "分享实用技能的团队空间。",
                      ))}
              </p>
            </div>
            {card.can_edit && (
              <button
                className="outline-button"
                onClick={() => setEditing(true)}
              >
                {t("Edit profile", "编辑主页")}
              </button>
            )}
          </div>
          <div className="profile-columns">
            <aside className="profile-about">
              {value?.kind === "team" && value.team.can_view_members ? (
                <TeamContributors
                  source={source}
                  namespace={value.team.namespace}
                />
              ) : (
                <>
                  <h3>{t("Community", "社区")}</h3>
                  <p>
                    {t(
                      "Skills and conversations shared in this community.",
                      "在这个社区分享的技能与交流。",
                    )}
                  </p>
                </>
              )}
              <span className="small-label">
                {ui.sources.find((s) => s.alias === source)?.display_name}
              </span>
              <button
                className="text-button"
                onClick={() => ui.navigate("discover")}
              >
                {t("Explore more skills", "探索更多技能")} →
              </button>
            </aside>
            <div>
              <ProfileTabs
                source={source}
                profile={value?.kind === "profile" ? value.profile : undefined}
                team={value?.kind === "team" ? value.team : undefined}
              />
            </div>
          </div>
          {editing && (
            <ProfileEditor
              source={source}
              profile={value?.kind === "profile" ? value.profile : undefined}
              team={value?.kind === "team" ? value.team : undefined}
              close={() => {
                setEditing(false);
                ui.changed("");
              }}
              saved={() => {
                setEditing(false);
                ui.changed(t("Profile updated", "资料已更新"));
              }}
            />
          )}
        </>
      )}
    </ResourceState>
  );
}
function ProfileTabs({
  source,
  profile,
  team,
}: {
  source: string;
  profile?: Profile;
  team?: Team;
}) {
  const ui = useUI(),
    [tab, setTab] = useState("skills");
  const skills = useResource(
    () =>
      API.CommunityListContributions(
        source,
        profile?.user_id ?? "",
        team?.namespace ?? "",
        "",
      ),
    [source, profile?.user_id, team?.namespace],
  );
  const members = useResource(
    async () =>
      team?.can_view_members
        ? API.CommunityListMembers(source, team.namespace, "")
        : ({ items: [], next_page: "" } as Page<Member>),
    [source, team?.namespace, team?.can_view_members],
  );
  return (
    <>
      <div className="content-tabs">
        <button
          aria-selected={tab === "skills"}
          onClick={() => setTab("skills")}
        >
          {t("Published skills", "已发布技能")}{" "}
          {skills.value && (
            <span>
              {skills.value.items?.length ?? 0}
              {skills.value.next_page ? "+" : ""}
            </span>
          )}
        </button>
        {team?.can_view_members && (
          <button
            aria-selected={tab === "members"}
            onClick={() => setTab("members")}
          >
            {t("Team members", "团队成员")}{" "}
            {members.value && (
              <span>
                {members.value.items?.length ?? 0}
                {members.value.next_page ? "+" : ""}
              </span>
            )}
          </button>
        )}
      </div>
      {tab === "skills" ? (
        <ResourceState {...skills} retry={skills.reload}>
          <Contributions source={source} items={skills.value?.items ?? []} />
          {!skills.value?.items?.length && (
            <p className="quiet-empty">
              {t(
                "No published versions available to you yet.",
                "暂时没有你可访问的已发布版本。",
              )}
            </p>
          )}
          {skills.value?.next_page && (
            <button
              onClick={() =>
                void ui.run(async () => {
                  const p = await API.CommunityListContributions(
                    source,
                    profile?.user_id ?? "",
                    team?.namespace ?? "",
                    skills.value!.next_page,
                  );
                  skills.setValue({
                    items: [...(skills.value?.items ?? []), ...(p.items ?? [])],
                    next_page: p.next_page,
                  });
                })
              }
            >
              {t("Load more")}
            </button>
          )}
        </ResourceState>
      ) : (
        <ResourceState {...members} retry={members.reload}>
          <div className="member-list">
            {members.value?.items?.map((member) => (
              <div className="member-row" key={member.profile.user_id}>
                <Person source={source} person={member.profile} />
                {member.profile.bio && (
                  <p className="member-bio">{member.profile.bio}</p>
                )}
                <div className="chips">
                  {member.roles?.map((role) => (
                    <span key={role}>{roleLabel(role)}</span>
                  ))}
                </div>
              </div>
            ))}
          </div>
          {!members.value?.items?.length && (
            <p className="quiet-empty">
              {t("No explicitly assigned members yet.", "尚未分配团队成员。")}
            </p>
          )}
          {members.value?.next_page && (
            <button
              onClick={() =>
                void ui.run(async () => {
                  const p = await API.CommunityListMembers(
                    source,
                    team!.namespace,
                    members.value!.next_page,
                  );
                  members.setValue({
                    items: [
                      ...(members.value?.items ?? []),
                      ...(p.items ?? []),
                    ],
                    next_page: p.next_page,
                  });
                })
              }
            >
              {t("Load more")}
            </button>
          )}
        </ResourceState>
      )}
    </>
  );
}
function ProfileEditor({
  source,
  profile,
  team,
  close,
  saved,
}: {
  source: string;
  profile?: Profile;
  team?: Team;
  close: () => void;
  saved: () => void;
}) {
  const ui = useUI(),
    initial = (profile ?? team)!,
    key = useMutationKey();
  const [currentProfile, setCurrentProfile] = useState(profile);
  const [avatarError, setAvatarError] = useState("");
  const [avatarBusy, setAvatarBusy] = useState(false);
  const avatarKey = useMutationKey();
  async function saveAvatar(file?: File) {
    setAvatarError("");
    if (
      file &&
      (file.size > 524288 || !["image/png", "image/jpeg"].includes(file.type))
    ) {
      setAvatarError(
        t(
          "Choose a PNG or JPEG up to 512 KB.",
          "请选择不超过 512 KB 的 PNG 或 JPEG。",
        ),
      );
      return;
    }
    setAvatarBusy(true);
    const uploaded = await ui.run(async () => {
      const bytes = file
        ? new Uint8Array(await file.arrayBuffer())
        : new Uint8Array();
      let binary = "";
      for (const byte of bytes) binary += String.fromCharCode(byte);
      const image = btoa(binary);
      const id = await avatarKey([source, image, currentProfile!.revision]);
      const updated = await API.CommunityUpdateAvatar(
        source,
        image,
        currentProfile!.revision,
        id,
      );
      setCurrentProfile(updated);
      return true;
    });
    if (!uploaded)
      setAvatarError(
        t(
          "Couldn’t save the photo. Check the image size, connection and profile revision, then reopen the editor to retry.",
          "头像保存失败。请检查图片尺寸和连接，重新打开编辑窗口后重试。",
        ),
      );
    setAvatarBusy(false);
  }
  const [name, setName] = useState(initial.display_name),
    [bio, setBio] = useState(initial.bio);
  return (
    <Modal title={t("Edit profile", "编辑主页")} onClose={close}>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void ui.run(async () => {
            const id = await key([
              source,
              name,
              bio,
              currentProfile?.revision ?? initial.revision,
            ]);
            if (team)
              await API.CommunityUpdateTeam(
                source,
                team.namespace,
                name,
                bio,
                team.revision,
                id,
              );
            else
              await API.CommunityUpdateProfile(
                source,
                name,
                bio,
                currentProfile!.revision,
                id,
              );
            saved();
          });
        }}
      >
        {currentProfile && (
          <div className="avatar-editor">
            <Avatar name={name} image={currentProfile.avatar} size="large" />
            <div>
              <label className="upload-avatar">
                {t("Upload photo", "上传头像")}
                <input
                  aria-label={t("Upload photo", "上传头像")}
                  type="file"
                  accept="image/png,image/jpeg"
                  disabled={avatarBusy || ui.busy}
                  onChange={(e) => {
                    const file = e.target.files?.[0];
                    if (file) void saveAvatar(file);
                    e.target.value = "";
                  }}
                />
              </label>
              <p>
                {t(
                  "PNG or JPEG · up to 512 KB, 2048 × 2048. Photos are cropped to a square and saved immediately.",
                  "PNG 或 JPEG，最多 512 KB、2048 × 2048。照片会裁为方形并立即保存。",
                )}
              </p>
              {currentProfile.avatar && (
                <button
                  type="button"
                  disabled={avatarBusy || ui.busy}
                  onClick={() => void saveAvatar()}
                >
                  {t("Remove photo", "移除头像")}
                </button>
              )}
              {avatarError && <p role="alert">{avatarError}</p>}
            </div>
          </div>
        )}
        <p className="muted">
          {team
            ? t(
                "The team name and introduction appear on its community page. Editing them does not change namespace permissions.",
                "团队名称和简介展示在团队主页，修改它们不会改变命名空间权限。",
              )
            : t(
                "Your display name, bio and avatar appear alongside contributions people can access. Your sign-in username is only shown to you and Hub administrators.",
                "显示名称、简介和头像会随他人有权查看的贡献展示；登录用户名仅对你和 Hub 管理员可见。",
              )}
        </p>
        <Field label={t("Display name")}>
          <input
            required
            maxLength={80}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
        </Field>
        <Field label={t("Bio", "简介")}>
          <textarea
            maxLength={600}
            value={bio}
            onChange={(e) => setBio(e.target.value)}
            placeholder={t(
              "What do you work on? What do you like to share?",
              "你在做什么？希望和大家分享什么？",
            )}
          />
        </Field>
        <div className="actions">
          <button type="button" onClick={close}>
            {t("Cancel")}
          </button>
          <button className="primary" disabled={ui.busy || !name.trim()}>
            {t("Save changes")}
          </button>
        </div>
      </form>
    </Modal>
  );
}
