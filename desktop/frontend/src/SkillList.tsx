import { UpstreamAuthor } from "./ProvenancePanel";
import { Icon } from "./Icon";
import { SkillSymbol, skillTitle } from "./SkillIdentity";
import { API } from "./platform";
import { useUI } from "./ui";
import {
  Person,
  AppLink,
  openCard,
  useResource,
  type Contribution,
} from "./community-ui";
import { t } from "./i18n";
import type {
  SkillView,
  AdminRow,
} from "../bindings/github.com/skillhubhq/skillhub/desktop/models";

export function SkillList({
  source,
  skills,
  categories,
  selected,
  onSelect,
  routeFor,
}: {
  source: string;
  skills: SkillView[];
  categories: AdminRow[];
  selected?: string;
  onSelect: (skill: SkillView, tab?: string) => void;
  routeFor: (skill: SkillView, tab?: string) => string;
}) {
  const ui = useUI();
  const authors = useResource(
    () => API.CommunityListContributions(source, "", "", ""),
    [source, ui.revision],
  );
  return (
    <div className="catalog-items">
      {skills.map((skill) => {
        const contribution = authors.value?.items?.find(
          (item) =>
            item.namespace === skill.namespace &&
            item.name === skill.name &&
            item.version === skill.version,
        );
        return (
          <article
            className={`catalog-item ${selected === `${source}/${skill.namespace}/${skill.name}` ? "active" : ""}`}
            key={`${skill.namespace}/${skill.name}`}
            onClick={(event) => openCard(event, () => onSelect(skill))}
          >
            <div className="skill-card-heading">
              <SkillSymbol icon={skill.icon} />
              <div className="skill-card-identity">
                <AppLink className="skill-card-link" to={routeFor(skill)}>
                  {skillTitle(skill)}
                </AppLink>
                <span className="skill-coordinate">
                  {skill.namespace}/{skill.name}
                </span>
              </div>
              <span className="catalog-version" title={skill.version}>
                {skill.provenance
                  ? skill.provenance.revision.slice(0, 7)
                  : `v${skill.version}`}
              </span>
            </div>
            <p>{skill.description}</p>
            <div className="catalog-labels">
              {skill.featured && (
                <span className="featured-label">{t("Featured", "精选")}</span>
              )}
              {skill.category && (
                <span className="skill-category-label">
                  {categories.find((c) => c.name === skill.category)?.title ||
                    skill.category}
                </span>
              )}
              <AppLink
                className="catalog-namespace"
                to={`teams?${new URLSearchParams({ source, team: skill.namespace })}`}
              >
                /{skill.namespace}
              </AppLink>
              {skill.tags?.slice(0, 2).map((tag) => (
                <AppLink
                  key={tag}
                  to={`discover?${new URLSearchParams({ source, tag })}`}
                >
                  {tag}
                </AppLink>
              ))}
            </div>
            {!contribution && skill.provenance && (
              <div className="catalog-social">
                <UpstreamAuthor value={skill.provenance} />
              </div>
            )}
            {contribution && (
              <CatalogSocial
                source={source}
                contribution={contribution}
                revision={ui.revision}
                openDiscussion={() => onSelect(skill, "discussion")}
                openSkill={() => onSelect(skill)}
              />
            )}
          </article>
        );
      })}
    </div>
  );
}
function CatalogSocial({
  source,
  contribution,
  revision,
  openDiscussion,
  openSkill,
}: {
  source: string;
  contribution: Contribution;
  revision: number;
  openDiscussion: () => void;
  openSkill: () => void;
}) {
  const discussion = useResource(
    () => API.CommunityGetDiscussion(source, contribution.release_id),
    [source, contribution.release_id, revision],
  );
  return (
    <div className="catalog-social">
      {contribution.provenance ? (
        <UpstreamAuthor value={contribution.provenance} />
      ) : (
        <Person source={source} person={contribution.author} />
      )}
      {discussion.value && (
        <span className="catalog-counts">
          <button
            className="discussion-shortcut"
            onClick={openSkill}
            aria-label={`${t("Likes", "点赞")} · ${contribution.name} · ${discussion.value.like_count}`}
            title={t("View skill reactions", "查看技能互动")}
          >
            <Icon name="heart" />
            {discussion.value.like_count}
          </button>
          <button
            className="discussion-shortcut"
            onClick={openDiscussion}
            aria-label={`${t("Conversation", "讨论")} · ${contribution.name} · ${discussion.value.comment_count}`}
          >
            <Icon name="message" />
            {discussion.value.comment_count}
          </button>
        </span>
      )}
    </div>
  );
}
