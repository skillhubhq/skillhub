import { API, useUI } from "./ui";
import { Avatar, Person, useResource } from "./community-ui";
import { t } from "./i18n";
export function SkillPeople({
  source,
  userId,
  namespace,
  curated = false,
}: {
  source: string;
  userId: string;
  namespace: string;
  curated?: boolean;
}) {
  const ui = useUI();
  const profile = useResource(
    () => API.CommunityGetProfile(source, userId),
    [source, userId],
  );
  const team = useResource(
    () => API.CommunityGetTeam(source, namespace),
    [source, namespace],
  );
  return (
    <div className="skill-people">
      <div>
        <span className="small-label">
          {curated ? t("Submitted by", "提交账号") : t("Published by")}
        </span>
        {profile.value ? (
          <Person source={source} person={profile.value} />
        ) : (
          <span className="muted">
            {profile.loading
              ? t("Loading", "正在加载")
              : t("Profile unavailable", "主页不可用")}
          </span>
        )}
      </div>
      <div>
        <span className="small-label">
          {curated ? t("Curating maintainer", "收录维护者") : t("Team", "团队")}
        </span>
        <button
          className="person-link"
          onClick={() =>
            ui.navigate(
              `teams?source=${encodeURIComponent(source)}&team=${encodeURIComponent(namespace)}`,
            )
          }
        >
          <Avatar name={team.value?.display_name ?? namespace} />
          <span>
            {team.value?.display_name ?? namespace}
            <small>/{namespace}</small>
          </span>
        </button>
      </div>
    </div>
  );
}
export function CommunityAside({ source }: { source: string }) {
  const ui = useUI();
  const teams = useResource(
    () =>
      source
        ? API.CommunityListTeams(source, "")
        : Promise.resolve({ items: [], next_page: "" }),
    [source],
  );
  if (teams.loading || teams.error || !teams.value?.items?.length) return null;
  return (
    <section className="discovery-community team-collections">
      <div className="community-team-list">
        <div className="section-heading">
          <div>
            <h2>{t("Explore teams", "发现团队")}</h2>
          </div>
          <button className="text-button" onClick={() => ui.navigate("teams")}>
            {t("View all", "查看全部")}
          </button>
        </div>
        <div className="collection-grid">
          {teams.value?.items?.slice(0, 4).map((team, index) => (
            <button
              key={team.namespace}
              className={`collection-card collection-${index % 4}`}
              onClick={() =>
                ui.navigate(
                  `teams?source=${encodeURIComponent(source)}&team=${encodeURIComponent(team.namespace)}`,
                )
              }
            >
              <Avatar name={team.display_name} />
              <strong>{team.display_name}</strong>
              <span className="collection-arrow" aria-hidden="true">
                ›
              </span>
            </button>
          ))}
        </div>
        {!teams.loading && !teams.value?.items?.length && (
          <p className="muted">
            {t("Your teams will appear here.", "你可访问的团队会显示在这里。")}
          </p>
        )}
      </div>
    </section>
  );
}

export function TeamContributors({
  source,
  namespace,
}: {
  source: string;
  namespace: string;
}) {
  const members = useResource(
    () => API.CommunityListMembers(source, namespace, ""),
    [source, namespace],
  );
  if (!members.value?.items?.length) return null;
  return (
    <section className="team-contributors">
      <h3>{t("Meet the people", "认识团队成员")}</h3>
      <p>
        {t("The people building and sharing here.", "在这里创造与分享的人。")}
      </p>
      {members.value.items.slice(0, 5).map((member) => (
        <div key={member.profile.user_id}>
          <Person source={source} person={member.profile} />
          {member.profile.bio && <p>{member.profile.bio}</p>}
        </div>
      ))}
    </section>
  );
}
