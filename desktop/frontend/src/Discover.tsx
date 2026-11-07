import { skillWebURL } from "./official";
import { ProjectLink } from "./Downloads";
import { EvaluationPanel } from "./EvaluationPanel";
import { CatalogOrder as Order } from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";
import { FeaturedControl } from "./Governance";
import { Select } from "./Select";
import { SkillSymbol, skillTitle } from "./SkillIdentity";
import { signInURL } from "./auth";
import { AccessState } from "./AccessState";
import { Discussion } from "./Discussion";
import { SkillActions, WebInstallGuide } from "./SkillActions";
import { SkillPeople, CommunityAside } from "./SkillPeople";
import { SkillList } from "./SkillList";
import { Icon } from "./Icon";
import { useResource } from "./community-ui";
import { t } from "./i18n";
import { useEffect, useRef, useState } from "react";
import type {
  AdminRow,
  ReleasePage,
  SearchPage,
  SkillView,
} from "../bindings/github.com/skillhubhq/skillhub/desktop/models";
import type { ReleasePreview } from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
import {
  API,
  DirectoryField,
  Empty,
  Field,
  Modal,
  PreviewBody,
  ToolSelect,
  useUI,
  visibilityLabel,
} from "./ui";
import { isWeb } from "./platform";
export function Discover() {
  const ui = useUI();
  const initialParams = new URLSearchParams(location.hash.split("?")[1] ?? "");
  const [query, setQuery] = useState(initialParams.get("q") ?? ""),
    [category] = useState(initialParams.get("category") ?? ""),
    [categories, setCategories] = useState<AdminRow[]>([]),
    [pages, setPages] = useState<SearchPage[]>([]),
    [loaded, setLoaded] = useState(false),
    [selected, setSelected] = useState<{
      coordinate: string;
      skill: SkillView;
      releases: ReleasePage;
    } | null>(null),
    [preview, setPreview] = useState<ReleasePreview | null>(null),
    [version, setVersion] = useState(""),
    [install, setInstall] = useState(false),
    [tool, setTool] = useState("codex"),
    [scope, setScope] = useState("project"),
    [project, setProject] = useState("");
  const [detailTab, setDetailTab] = useState("readme");
  const [detailError, setDetailError] = useState("");
  const [detailAttempt, setDetailAttempt] = useState(0);
  const [previewError, setPreviewError] = useState("");
  const [previewAttempt, setPreviewAttempt] = useState(0);
  const catalogOrder =
    initialParams.get("order") === "recent"
      ? Order.CatalogOrder_CATALOG_ORDER_RECENT
      : Order.CatalogOrder_CATALOG_ORDER_UNSPECIFIED;
  const featuredOnly = initialParams.get("featured") === "true";
  const tag = initialParams.get("tag") ?? "";
  const inventory = useResource(
    () => (isWeb ? Promise.resolve([]) : API.ListInstalled()),
    [ui.revision],
  );
  const installed = (inventory.value ?? []).filter(
    (item) =>
      preview &&
      selected &&
      item.registry_id ===
        ui.sources.find((s) => s.alias === selected.coordinate.split("/")[0])
          ?.registry_id &&
      item.namespace === selected.skill.namespace &&
      item.name === selected.skill.name &&
      item.version === version &&
      item.digest === preview.release.digest,
  );
  useEffect(() => {
    if (!selected || !version) return;
    const params = new URLSearchParams(location.hash.split("?")[1] ?? "");
    if (params.get("skill")) {
      params.set("tab", detailTab);
      params.set("version", version);
      history.replaceState(null, "", `#discover?${params}`);
    }
  }, [selected?.coordinate, detailTab, version]);
  const detailRef = useRef<HTMLElement>(null);
  const discussionRef = useRef<HTMLDivElement>(null);
  const [focusComment, setFocusComment] = useState(0);
  useEffect(() => {
    if (focusComment && detailTab === "discussion") {
      discussionRef.current?.scrollIntoView({ block: "start" });
      discussionRef.current
        ?.querySelector("textarea")
        ?.focus({ preventScroll: true });
    }
  }, [focusComment, detailTab]);
  useEffect(() => {
    if (selected) detailRef.current?.scrollIntoView({ block: "start" });
  }, [selected?.coordinate]);
  const source = ui.activeSource;
  const allSources = !isWeb && initialParams.get("scope") === "all";
  const originSource = initialParams.get("origin") || source;
  const sourceLabel = (alias: string) => {
    const hub = ui.sources.find((s) => s.alias === alias);
    return hub
      ? `${hub.display_name || hub.alias}${isWeb ? "" : ` · ${hub.alias}`}`
      : alias;
  };
  const sourceKey = ui.sources.map((s) => s.alias).join("|");
  const detailSummary = useResource(
    () =>
      preview
        ? API.CommunityGetDiscussion(
            selected!.coordinate.split("/")[0],
            preview.release.id,
          )
        : Promise.resolve(null),
    [preview?.release.id, ui.revision],
  );
  const categoryOptions = categories.length
    ? categories.map((c) => ({ name: c.name, title: c.title || c.name }))
    : Array.from(
        new Set(
          pages
            .flatMap((p) => p.skills ?? [])
            .map((skill) => skill.category)
            .filter(Boolean),
        ),
      )
        .sort()
        .map((name) => ({
          name,
          title: name.replace(/-/g, " ").replace(/^./, (c) => c.toUpperCase()),
        }));

  const generation = useRef(0),
    selection = useRef(0);
  const last = useRef({ query: "", category: "" });
  function catalogRoute(
    q = query,
    c = category,
    all = allSources,
    selectedTag = tag,
  ) {
    const params = new URLSearchParams({
      source: originSource,
      order:
        catalogOrder === Order.CatalogOrder_CATALOG_ORDER_RECENT
          ? "recent"
          : "name",
      featured: String(featuredOnly),
      q,
      category: all ? "" : c,
      tag: selectedTag,
      scope: all ? "all" : "current",
    });
    return `discover?${params}`;
  }
  function skillRoute(alias: string, skill: SkillView, tab = "readme") {
    const params = new URLSearchParams({
      source: alias,
      order:
        catalogOrder === Order.CatalogOrder_CATALOG_ORDER_RECENT
          ? "recent"
          : "name",
      featured: String(featuredOnly),
      origin: originSource,
      scope: allSources ? "all" : "current",
      q: last.current.query,
      category: last.current.category,
      tag,
      skill: `${skill.namespace}/${skill.name}`,
      version: skill.version,
      tab,
    });
    return `discover?${params}`;
  }
  async function search(q = query, c = category, preserveSelection = false) {
    const n = ++generation.current;
    setLoaded(false);
    if (!preserveSelection) {
      setSelected(null);
      setPreview(null);
      selection.current++;
    }
    await ui.run(async () => {
      const result = await API.Search(
        allSources ? "" : source,
        q,
        allSources ? "" : c,
        tag ? [tag] : [],
        "",
        catalogOrder,
        featuredOnly,
      );
      if (n === generation.current) {
        setPages(result ?? []);
        setLoaded(true);
        last.current = { query: q, category: c };
      }
    });
  }
  useEffect(() => {
    let active = true;
    setDetailError("");
    if (sourceKey)
      void (async () => {
        const params = new URLSearchParams(location.hash.split("?")[1] ?? "");
        const path = params.get("skill");
        if (!path) await search(query, category);
        else last.current = { query, category };
        if (path && source) {
          const [namespace, name] = path.split("/");
          if (namespace && name) {
            const skill = await API.GetSkill(source, namespace, name);
            if (!active) return;
            if (skill)
              await select(
                source,
                skill,
                params.get("version") ?? undefined,
                params.get("tab") ?? "readme",
              );
            else
              throw new Error(
                t(
                  "This skill is unavailable or you no longer have access.",
                  "该技能不可用，或你已无权访问。",
                ),
              );
          } else throw new Error(t("Invalid skill link.", "技能链接无效。"));
        }
      })().catch((error) => {
        if (active) setDetailError(String(error.message ?? error));
      });
    return () => {
      active = false;
      generation.current++;
      selection.current++;
    };
  }, [sourceKey, source, detailAttempt]);
  useEffect(() => {
    if (sourceKey && ui.revision > 0 && !initialParams.get("skill"))
      void search(last.current.query, last.current.category, true);
  }, [ui.revision]);
  useEffect(() => {
    let active = true;
    setCategories([]);
    if (source)
      void (async () => {
        try {
          let token = "";
          const rows: AdminRow[] = [];
          do {
            const p = await API.AdminList(source, "categories", "", token);
            rows.push(...(p.rows ?? []));
            token = p.next_page;
          } while (token);
          if (active) setCategories(rows.filter((c) => c.enabled));
        } catch {}
      })();
    return () => {
      active = false;
    };
  }, [source]);
  async function select(
    alias: string,
    skill: SkillView,
    requestedVersion?: string,
    tab = "readme",
  ) {
    const n = ++selection.current;
    setPreview(null);
    setSelected(null);
    setDetailTab(tab);
    const coordinate = `${alias}/${skill.namespace}/${skill.name}`;
    const releases = await API.Releases(coordinate, "");
    if (n !== selection.current) return;
    if (!releases.releases?.length)
      throw new Error(t("No accessible versions.", "没有可访问的版本。"));
    setSelected({ coordinate, skill, releases });
    setVersion(requestedVersion || releases.releases[0].version);
  }
  useEffect(() => {
    let active = true;
    setPreview(null);
    setPreviewError("");
    if (selected && version)
      void API.Preview(`${selected.coordinate}@${version}`)
        .then((p) => {
          if (active) setPreview(p);
        })
        .catch((error) => {
          if (active) setPreviewError(String(error.message ?? error));
        });
    return () => {
      active = false;
    };
  }, [selected?.coordinate, version, ui.revision, previewAttempt]);
  const browseControl = (
    <Select
      aria-label={t("Browse skills", "浏览技能")}
      value={
        featuredOnly
          ? "featured"
          : catalogOrder === Order.CatalogOrder_CATALOG_ORDER_RECENT
            ? "recent"
            : "name"
      }
      onChange={(e) => {
        const params = new URLSearchParams(catalogRoute().split("?")[1]);
        params.set("order", e.target.value === "name" ? "name" : "recent");
        params.set("featured", String(e.target.value === "featured"));
        ui.navigate(`discover?${params}`);
      }}
    >
      <option value="name">{t("Name", "名称")}</option>
      <option value="recent">{t("Recently published", "最近发布")}</option>
      <option value="featured">{t("Featured", "精选")}</option>
    </Select>
  );
  if (!ui.sources.length)
    return (
      <Empty
        title={
          isWeb ? t("Connecting to this hub") : t("Connect your first hub")
        }
      >
        {!isWeb && (
          <button className="primary" onClick={() => ui.navigate("sources")}>
            {t("Add hub")}
          </button>
        )}
      </Empty>
    );
  if (initialParams.get("skill") && !selected)
    return (
      <section className="detail-load-state" aria-busy={!detailError}>
        <button onClick={() => ui.navigate(catalogRoute())}>
          {t("Back to skills")}
        </button>
        {detailError ? (
          <div role="alert">
            <h2>{t("Could not open this skill", "无法打开这个技能")}</h2>
            <AccessState
              error={detailError}
              retry={() => setDetailAttempt((n) => n + 1)}
            />
          </div>
        ) : (
          <p role="status">{t("Loading this version…")}</p>
        )}
      </section>
    );
  return (
    <div className={`discovery ${selected ? "has-detail" : "browse-mode"}`}>
      <div className="discovery-catalog">
        <div className="discovery-intro">
          <h1>
            {t("Good work is worth sharing.", "让好方法，被更多人用到。")}
          </h1>
          <p>
            {t(
              "Discover skills for the work you do. Learn from the people who made them.",
              "找到适合你工作的技能，也认识创造它们的人。",
            )}
          </p>
        </div>
        <form
          className="discovery-search"
          onSubmit={(e) => {
            e.preventDefault();
            ui.navigate(catalogRoute());
          }}
        >
          <div className="search-row">
            <Icon name="explore" />
            {!isWeb && (
              <Select
                className="search-scope"
                aria-label={t("Search scope", "搜索范围")}
                value={allSources ? "all" : "current"}
                disabled={ui.busy}
                onChange={(e) =>
                  ui.navigate(catalogRoute(query, "", e.target.value === "all"))
                }
              >
                <option value="current">{t("This hub", "当前 Hub")}</option>
                <option value="all">{t("All connections", "所有连接")}</option>
              </Select>
            )}
            <input
              aria-label={t("Search skills")}
              value={query}
              maxLength={200}
              placeholder={t("Search by name or what you want to do…")}
              onChange={(e) => setQuery(e.target.value)}
            />
            <button className="primary" disabled={ui.busy}>
              {t("Search")}
            </button>
          </div>
          {!isWeb && (
            <p className="search-context">
              {allSources
                ? t(
                    "Searching all connected hubs. Results keep each connection’s identity and permissions.",
                    "搜索所有已连接 Hub，结果保留各连接的身份与权限。",
                  )
                : t(
                    "Searching this community. Choose All connections to explore beyond it.",
                    "正在搜索当前社区。选择「所有连接」可跨 Hub 发现技能。",
                  )}
            </p>
          )}
          {(categoryOptions.length > 0 || category || query || tag) && (
            <div className="catalog-filters">
              <div
                className="topic-buttons"
                aria-label={t("Filter by category")}
              >
                <button
                  type="button"
                  aria-pressed={!category}
                  onClick={() => {
                    ui.navigate(catalogRoute(query, ""));
                  }}
                >
                  {t("All skills", "全部技能")}
                </button>
                {(!allSources ? categoryOptions : []).map((c) => (
                  <button
                    type="button"
                    key={c.name}
                    disabled={!source}
                    aria-pressed={category === c.name}
                    onClick={() => {
                      ui.navigate(catalogRoute(query, c.name));
                    }}
                  >
                    {c.title || c.name}
                  </button>
                ))}
              </div>
              {(category || query || tag) && (
                <button
                  type="button"
                  className="text-button"
                  onClick={() => {
                    ui.navigate(catalogRoute("", "", allSources, ""));
                  }}
                >
                  {t("Clear filters", "清除筛选")}
                </button>
              )}
            </div>
          )}
        </form>
        {tag && (
          <p className="active-tag">
            {t("Tag", "标签")}: {tag}
          </p>
        )}
        {!allSources &&
          !tag &&
          !last.current.query &&
          !last.current.category &&
          source && <CommunityAside source={source} />}
        {pages.map((p, index) => (
          <section className="source-results" key={p.source}>
            <div className="section-heading">
              <h2>
                {allSources
                  ? sourceLabel(p.source)
                  : last.current.query || last.current.category || tag
                    ? t("Search results", "搜索结果")
                    : t("Explore the community", "探索社区技能")}
              </h2>
              <div className="catalog-browse-controls">
                {!p.error && (
                  <span className="muted">
                    {p.skills?.length ?? 0}{" "}
                    {(p.skills?.length ?? 0) === 1
                      ? t("skill", "个技能")
                      : t("skills")}
                  </span>
                )}
                {(!allSources || index === 0) && browseControl}
              </div>
            </div>
            {p.error ? (
              <AccessState error={p.error} retry={() => void search()} />
            ) : (p.skills ?? []).length === 0 ? (
              <Empty title={t("No matching skills")}>
                {t("Try a different search, or share a skill with your team.")}
              </Empty>
            ) : (
              <SkillList
                source={p.source}
                skills={p.skills ?? []}
                categories={allSources ? [] : categories}
                selected={selected?.coordinate}
                routeFor={(skill, tab) => skillRoute(p.source, skill, tab)}
                onSelect={(skill, tab) =>
                  ui.navigate(skillRoute(p.source, skill, tab))
                }
              />
            )}
            {p.next_page && (
              <button
                className="pager"
                disabled={ui.busy}
                onClick={() =>
                  void ui.run(async () => {
                    const result = await API.Search(
                      p.source,
                      last.current.query,
                      last.current.category,
                      tag ? [tag] : [],
                      p.next_page,
                      catalogOrder,
                      featuredOnly,
                    );
                    const next = result?.[0];
                    if (next)
                      setPages((old) =>
                        old.map((v) =>
                          v.source === p.source
                            ? {
                                ...next,
                                skills: [
                                  ...(v.skills ?? []),
                                  ...(next.skills ?? []),
                                ],
                              }
                            : v,
                        ),
                      );
                  })
                }
              >
                {t("Load more")}
              </button>
            )}
          </section>
        ))}
        {!loaded && !pages.length && (
          <p className="muted">{t("Loading skills…")}</p>
        )}
      </div>
      {selected ? (
        <section ref={detailRef} className="skill-detail community-detail">
          <div className="section-heading">
            <span className="muted">
              {sourceLabel(selected.coordinate.split("/")[0])} /{" "}
              {selected.skill.namespace}
            </span>
            <button
              onClick={() => {
                ui.navigate(catalogRoute());
              }}
            >
              {t("Back to skills")}
            </button>
          </div>
          <div className="detail-hero">
            <div className="detail-symbol">
              <SkillSymbol
                icon={preview?.release.icon ?? selected.skill.icon}
              />
            </div>
            <div>
              <span className="eyebrow">
                {categories.find((c) => c.name === selected.skill.category)
                  ?.title ||
                  selected.skill.category ||
                  t("Skill", "技能")}
              </span>
              <h1>{skillTitle(preview?.release || selected.skill)}</h1>
              <span className="skill-coordinate">
                {selected.skill.namespace}/{selected.skill.name}
              </span>
              <p>{selected.skill.description}</p>
            </div>
          </div>
          <FeaturedControl
            source={selected.coordinate.split("/")[0]}
            skill={selected.skill}
            changed={(v) =>
              setSelected((old) =>
                old ? { ...old, skill: { ...old.skill, featured: v } } : old,
              )
            }
          />
          <div className="detail-action-bar">
            {!isWeb && (
              <ProjectLink
                className="source-web-link"
                href={skillWebURL(
                  ui.sources.find(
                    (s) => s.alias === selected.coordinate.split("/")[0],
                  )?.url ?? "",
                  selected.skill.namespace,
                  selected.skill.name,
                  version,
                )}
              >
                {t("View on source Hub", "在来源网站查看")} ↗
              </ProjectLink>
            )}
            <button
              className="primary install-primary"
              disabled={!preview || ui.busy}
              onClick={() => setInstall(true)}
            >
              <Icon name="installed" />
              {installed.length
                ? t("Install elsewhere", "安装到其他位置")
                : t("Install skill")}
              <span>v{version}</span>
            </button>
            {preview && (
              <SkillActions
                key={preview.release.id}
                source={selected.coordinate.split("/")[0]}
                id={preview.release.id}
                summary={detailSummary.value}
                changed={detailSummary.setValue}
                comment={() => {
                  setDetailTab("discussion");
                  setFocusComment((n) => n + 1);
                }}
              />
            )}
            {detailSummary.error && (
              <button onClick={detailSummary.reload}>
                {t("Retry community actions", "重新加载社区操作")}
              </button>
            )}
          </div>
          {previewError && (
            <div className="version-unavailable" role="status">
              <strong>
                {t("This version could not be loaded", "当前版本无法加载")}
              </strong>
              <p>
                {t(
                  "It may have been withdrawn, access may have changed, or the Hub may be unavailable. Installed files are unchanged.",
                  "该版本可能已撤下、访问权限已变更，或 Hub 暂不可用。已安装文件不会改变。",
                )}
              </p>
              <div className="actions">
                <button onClick={() => setPreviewAttempt((n) => n + 1)}>
                  {t("Try again")}
                </button>
                {selected.skill.version !== version && (
                  <button onClick={() => setVersion(selected.skill.version)}>
                    {t("View available version", "查看可获取版本")}{" "}
                    {selected.skill.version}
                  </button>
                )}
              </div>
            </div>
          )}
          {installed.length > 0 && (
            <div className="installed-confirmation">
              <Icon name="review" />
              <span>
                {t(
                  "This version is installed on this device.",
                  "当前版本已安装到本机。",
                )}
              </span>
              <button onClick={() => ui.navigate("installed")}>
                {t("Open my library", "查看本地技能")}
              </button>
            </div>
          )}
          <div className="skill-detail-layout">
            <aside className="skill-install-aside">
              <h3>{t("About this version", "关于当前版本")}</h3>
              <p className="version-context">
                {t(
                  "Likes, saves and comments belong to this version.",
                  "点赞、收藏和评论属于当前版本。",
                )}
              </p>
              <div className="version-selector">
                <Field label={t("Version")}>
                  <Select
                    value={version}
                    onChange={(e) => setVersion(e.target.value)}
                  >
                    {!selected.releases.releases?.some(
                      (r) => r.version === version,
                    ) && <option value={version}>{version}</option>}
                    {selected.releases.releases?.map((r) => (
                      <option key={r.id} value={r.version}>
                        {r.version}
                      </option>
                    ))}
                  </Select>
                </Field>
              </div>
              {preview && (
                <>
                  <div className="detail-facts">
                    <span>{visibilityLabel(preview.release.visibility)}</span>
                    <span>
                      {preview.files?.length ?? 0}{" "}
                      {preview.files?.length === 1
                        ? t("file", "个文件")
                        : t("files", "个文件")}
                    </span>
                  </div>
                  <SkillPeople
                    source={selected.coordinate.split("/")[0]}
                    userId={preview.release.submitted_by}
                    namespace={preview.release.namespace}
                  />
                </>
              )}
              {selected.releases.next_page && (
                <button
                  disabled={ui.busy}
                  onClick={() =>
                    void ui.run(async () => {
                      const p = await API.Releases(
                        selected.coordinate,
                        selected.releases.next_page,
                      );
                      setSelected({
                        ...selected,
                        releases: {
                          ...p,
                          releases: [
                            ...(selected.releases.releases ?? []),
                            ...(p.releases ?? []),
                          ],
                        },
                      });
                    })
                  }
                >
                  {t("More versions")}
                </button>
              )}
            </aside>
            <div className="detail-reading">
              <div
                className="content-tabs detail-tabs"
                aria-label={t("Skill content", "技能内容")}
              >
                <button
                  aria-pressed={detailTab === "readme"}
                  onClick={() => setDetailTab("readme")}
                >
                  {t("Overview", "介绍")}
                </button>
                <button
                  aria-pressed={detailTab === "discussion"}
                  onClick={() => setDetailTab("discussion")}
                >
                  {t("Discussion", "讨论")}{" "}
                  {detailSummary.value && (
                    <span>{detailSummary.value.comment_count}</span>
                  )}
                </button>
                <button
                  aria-pressed={detailTab === "evaluation"}
                  onClick={() => setDetailTab("evaluation")}
                >
                  {t("Evaluations", "评测")}
                </button>
                <button
                  aria-pressed={detailTab === "files"}
                  onClick={() => setDetailTab("files")}
                >
                  {t("Files", "文件")}{" "}
                  {preview && <span>{preview.files?.length ?? 0}</span>}
                </button>
              </div>
              <div className="detail-content">
                {preview ? (
                  <>
                    <div
                      hidden={
                        detailTab === "discussion" || detailTab === "evaluation"
                      }
                    >
                      <PreviewBody
                        preview={preview}
                        view={detailTab === "files" ? "files" : "readme"}
                      />
                    </div>
                    {detailTab === "evaluation" && (
                      <EvaluationPanel
                        key={preview.release.id}
                        source={selected.coordinate.split("/")[0]}
                        release={preview.release.id}
                        digest={preview.release.digest}
                      />
                    )}
                    <div
                      ref={discussionRef}
                      className="discussion-anchor"
                      hidden={detailTab !== "discussion"}
                    >
                      <Discussion
                        key={preview.release.id}
                        source={selected.coordinate.split("/")[0]}
                        id={preview.release.id}
                        summary={detailSummary.value}
                        onCountChange={detailSummary.reload}
                      />
                    </div>
                  </>
                ) : previewError ? (
                  <details>
                    <summary>{t("Error details", "错误详情")}</summary>
                    <p>{previewError}</p>
                  </details>
                ) : (
                  <p className="muted" role="status">
                    {t("Loading this version…")}
                  </p>
                )}
              </div>
            </div>
          </div>
        </section>
      ) : null}
      {install && selected && (
        <Modal
          stable
          title={`${t("Install skill")} · ${skillTitle(preview?.release || selected.skill)}`}
          onClose={() => setInstall(false)}
        >
          <p className="install-coordinate">
            {selected.skill.namespace}/{selected.skill.name}@{version}
          </p>
          {isWeb ? (
            <WebInstallGuide
              url={
                ui.sources.find(
                  (s) => s.alias === selected.coordinate.split("/")[0],
                )?.url ?? ""
              }
              coordinate={`${selected.skill.namespace}/${selected.skill.name}@${version}`}
            />
          ) : (
            <form
              className="form-grid"
              onSubmit={(e) => {
                e.preventDefault();
                setInstall(false);
                ui.plan(() =>
                  API.PlanInstall(
                    `${selected.coordinate}@${version}`,
                    tool,
                    scope,
                    scope === "project" ? project : "",
                  ),
                );
              }}
            >
              <Field label={t("Tool")}>
                <ToolSelect value={tool} onChange={setTool} />
              </Field>
              <Field label={t("Install for", "安装范围")}>
                <Select
                  value={scope}
                  onChange={(e) => setScope(e.target.value)}
                >
                  <option value="project">
                    {t("This project", "当前项目")}
                  </option>
                  <option value="global">
                    {t("All your projects", "你的所有项目")}
                  </option>
                </Select>
              </Field>
              <p className="scope-hint full">
                {scope === "project"
                  ? t(
                      "Only available in the project folder you choose.",
                      "仅在你选择的项目目录中可用。",
                    )
                  : t(
                      "Available to this tool across your projects for the current OS user.",
                      "当前系统用户的所有项目都可在此工具中使用。",
                    )}
              </p>
              {scope === "project" && (
                <DirectoryField
                  required
                  value={project}
                  onChange={setProject}
                />
              )}
              <p className="muted full">
                {t(
                  "Review the destination and file changes before installing.",
                )}
              </p>
              <div className="actions full">
                <button
                  className="primary"
                  disabled={ui.busy || (scope === "project" && !project.trim())}
                >
                  {t("Review installation")}
                </button>
              </div>
            </form>
          )}
        </Modal>
      )}
    </div>
  );
}
