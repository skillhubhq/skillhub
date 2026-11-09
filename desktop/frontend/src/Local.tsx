import { ResourceState, useResource } from "./community-ui";
import { ProvenanceDetails, UpstreamAuthor } from "./ProvenancePanel";
import { ProjectSkillPicker } from "./ProjectSkillPicker";
import { Select } from "./Select";
import { SkillSymbol, skillTitle } from "./SkillIdentity";
import { t } from "./i18n";
import { useEffect, useRef, useState } from "react";
import type {
  Installation,
  ProjectState,
  UpdateCheck,
} from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
import {
  API,
  DirectoryField,
  Empty,
  Field,
  Modal,
  SourceSelect,
  ToolSelect,
  toolLabel,
  useUI,
} from "./ui";
export function Installed() {
  const ui = useUI();
  const inventory = useResource(() => API.ListInstalled(), [], ui.revision);
  const suites = useResource(() => API.Suites(), [], ui.revision);
  const pinnedBy = (target: string) =>
    (suites.value ?? []).filter((s) =>
      (s.members ?? []).some((m) => m.target === target),
    );
  const items = inventory.value ?? [];
  const setItems = inventory.setValue;
  const [filter, setFilter] = useState("");
  const [checks, setChecks] = useState<UpdateCheck[]>([]);
  const [repairSources, setRepairSources] = useState<Record<string, string>>(
    {},
  );
  useEffect(() => {
    setChecks([]);
  }, [ui.revision]);
  const origin = (item: Installation) =>
    JSON.stringify([item.registry_id, item.registry_url]);
  const connections = (item: Installation) =>
    ui.sources.filter(
      (s) => s.registry_id === item.registry_id && s.url === item.registry_url,
    );
  const hubName = (item: Installation) =>
    connections(item)[0]?.display_name || item.registry_url;
  const checkUpdates = (targets: Installation[], source = "") =>
    void ui.run(async () => {
      const results: UpdateCheck[] = [];
      for (const item of targets.filter(
        (i) => pinnedBy(i.target).length === 0,
      )) {
        results.push(
          ...((await API.CheckUpdatesFrom(item.target, source)) ?? []),
        );
      }
      setChecks((old) => [
        ...old.filter(
          (c) => !targets.some((i) => i.target === c.installation.target),
        ),
        ...results,
      ]);
      setItems((await API.ListInstalled()) ?? []);
    });
  const repairs = [
    ...new Map(
      checks
        .filter((c) => c.connection_required)
        .map((c) => [origin(c.installation), c.installation]),
    ).entries(),
  ];
  const visible = items.filter((i) =>
    `${i.name} ${i.display_name_en} ${i.display_name_zh} ${i.namespace} ${i.tool} ${i.project} ${hubName(i)}`
      .toLowerCase()
      .includes(filter.toLowerCase()),
  );
  return (
    <ResourceState {...inventory} retry={inventory.reload}>
      <div className="toolbar library-toolbar">
        <input
          aria-label={t("Filter installed skills")}
          placeholder={t("Filter by name, tool or project…")}
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <span>
          {visible.length} {t("installations")}
        </span>
        <button
          disabled={ui.busy || items.length === 0}
          onClick={() => checkUpdates(items)}
        >
          {t("Check for updates")}
        </button>
      </div>
      {repairs.map(([key, item]) => {
        const candidates = connections(item);
        const affected = checks
          .filter(
            (c) => c.connection_required && origin(c.installation) === key,
          )
          .map((c) => c.installation);
        const selected =
          repairSources[key] ||
          (candidates.length === 1 ? candidates[0].alias : "");
        return (
          <section
            className="library-repair"
            key={key}
            aria-label={t("Reconnect for updates", "恢复更新连接")}
          >
            <div>
              <strong>{hubName(item)}</strong>
              <p>
                {candidates.length
                  ? t(
                      "Choose an account once to resume updates for these skills. Your installed files stay available.",
                      "为这些技能选择一次账号以恢复更新，已安装的文件仍可正常使用。",
                    )
                  : t(
                      "Add this hub in Connections to check for updates. Your installed files stay available.",
                      "在“来源与账号”中添加此 Hub 后即可检查更新，已安装的文件仍可正常使用。",
                    )}
              </p>
              <small>{affected.map((i) => i.name).join(", ")}</small>
            </div>
            {candidates.length > 0 ? (
              <form
                className="library-repair-actions"
                onSubmit={(e) => {
                  e.preventDefault();
                  checkUpdates(affected, selected);
                }}
              >
                <Select
                  aria-label={`${t("Account connection", "账号连接")} · ${hubName(item)}`}
                  value={selected}
                  onChange={(e) =>
                    setRepairSources((old) => ({
                      ...old,
                      [key]: e.target.value,
                    }))
                  }
                >
                  <option value="">
                    {t("Choose an account…", "选择账号…")}
                  </option>
                  {candidates.map((source) => (
                    <option key={source.alias} value={source.alias}>
                      {source.alias}
                    </option>
                  ))}
                </Select>
                <button className="primary" disabled={ui.busy || !selected}>
                  {t("Connect and check", "连接并检查")}
                </button>
              </form>
            ) : (
              <button onClick={() => ui.navigate("sources")}>
                {t("Connections")}
              </button>
            )}
          </section>
        );
      })}
      {visible.length === 0 ? (
        <Empty
          title={
            filter
              ? t("No matching installations")
              : t("Your library starts here")
          }
        >
          <button
            onClick={() => (filter ? setFilter("") : ui.navigate("discover"))}
          >
            {filter ? t("Clear filters", "清除筛选") : t("Explore")}
          </button>
        </Empty>
      ) : (
        <div className="list library-list">
          {visible.map((item) => {
            const check = checks.find(
              (c) => c.installation.target === item.target,
            );
            const current =
              check &&
              !check.error &&
              !check.version_unavailable &&
              !check.available_version;
            const suite = (suites.value ?? []).find(
              (s) => s.target === item.target,
            );
            const parents = pinnedBy(item.target);
            const project = item.project?.split(/[\\/]/).pop();
            return (
              <article className="installed-row" key={item.id}>
                <div className="installation-content">
                  <div className="installation-heading">
                    <SkillSymbol icon={item.icon} />
                    <h2>{skillTitle(item)}</h2>
                    {suite && (
                      <span className="badge">
                        {t("Suite", "套装")} · {suite.members?.length ?? 0}
                      </span>
                    )}
                    <span className="version">v{item.version}</span>
                  </div>
                  <span className="skill-coordinate">
                    {item.namespace}/{item.name}
                  </span>
                  <div className="installation-meta">
                    <span>{toolLabel(item.tool)}</span>
                    <span title={item.project}>
                      {item.scope === "global"
                        ? t("Global")
                        : project || t("Projects")}
                    </span>
                    <span title={item.registry_url}>
                      {hubName(item)} / {item.namespace}
                    </span>
                  </div>
                  {parents.length > 0 && (
                    <p className="muted">
                      {t("Updated with suite", "随套装更新")}:{" "}
                      {parents
                        .map((s) => s.target.split(/[\\/]/).pop())
                        .join(", ")}
                    </p>
                  )}
                  {item.provenance && (
                    <UpstreamAuthor value={item.provenance} />
                  )}
                  <details className="installation-details">
                    <summary>{t("Installation details", "安装详情")}</summary>
                    <ProvenanceDetails
                      value={item.provenance}
                      version={item.version}
                      digest={item.digest}
                    />
                    <dl>
                      <dt>{t("Location", "安装位置")}</dt>
                      <dd>{item.target}</dd>
                      <dt>{t("Source", "来源")}</dt>
                      <dd>{item.registry_url}</dd>
                      {item.source && (
                        <>
                          <dt>{t("Account connection", "账号连接")}</dt>
                          <dd>{item.source}</dd>
                        </>
                      )}
                    </dl>
                  </details>
                  {check?.version_unavailable && (
                    <div className="installation-error" role="status">
                      {t(
                        "This installed version is no longer available from its Hub. Your local files are kept, but reinstalling or restoring this version may not be possible.",
                        "此安装版本已无法从原 Hub 获取。本地文件仍保留，但可能无法重新安装或恢复此版本。",
                      )}
                      <div className="actions">
                        <button onClick={() => checkUpdates([item])}>
                          {t("Check again", "重新检查")}
                        </button>
                        {check.source && (
                          <button
                            onClick={() =>
                              ui.navigate(
                                `discover?${new URLSearchParams({ source: check.source!, skill: `${item.namespace}/${item.name}` })}`,
                              )
                            }
                          >
                            {t("Browse available versions", "浏览可获取版本")}
                          </button>
                        )}
                      </div>
                    </div>
                  )}
                  {check?.error && !check.connection_required && (
                    <div className="installation-error" role="status">
                      <p>
                        {t(
                          "Could not check for updates. Check your connection or sign in again.",
                          "暂时无法检查更新，请检查连接或重新登录。",
                        )}
                      </p>
                      <button onClick={() => ui.navigate("sources")}>
                        {t("Connections")}
                      </button>
                      <details>
                        <summary>{t("Error details", "错误详情")}</summary>
                        <p>{check.error}</p>
                      </details>
                    </div>
                  )}
                </div>
                <div className="actions installation-actions">
                  <button
                    disabled={ui.busy}
                    onClick={() =>
                      ui.navigate(
                        `configuration?${new URLSearchParams({ target: item.target })}`,
                      )
                    }
                  >
                    {t("Configure", "配置")}
                  </button>
                  {parents.length > 0 ? (
                    <span className="muted">
                      {t("Version pinned by suite", "版本由套装固定")}
                    </span>
                  ) : current ? (
                    <span className="installation-current" role="status">
                      {t("Up to date", "已是最新")}
                    </span>
                  ) : check?.connection_required ? (
                    <span className="muted">
                      {t("Connection needed", "待恢复连接")}
                    </span>
                  ) : (
                    <button
                      className={check?.available_version ? "primary" : ""}
                      disabled={ui.busy}
                      onClick={() =>
                        check?.available_version
                          ? ui.plan(() =>
                              API.PlanUpdateFrom(
                                item.target,
                                check.available_version ?? "",
                                "",
                              ),
                            )
                          : checkUpdates([item])
                      }
                    >
                      {check?.available_version
                        ? `${t("Update to", "更新至")} v${check.available_version}`
                        : t("Check update", "检查更新")}
                    </button>
                  )}
                  <button
                    className="text-danger"
                    disabled={ui.busy || parents.length > 0}
                    onClick={() => ui.plan(() => API.PlanRemove(item.target))}
                  >
                    {t("Uninstall")}
                  </button>
                </div>
              </article>
            );
          })}
        </div>
      )}
    </ResourceState>
  );
}
export function Projects() {
  const ui = useUI();
  const [opening, setOpening] = useState(false),
    [options, setOptions] = useState(false),
    [editing, setEditing] = useState(false),
    [editingExisting, setEditingExisting] = useState(false);
  const [roots, setRoots] = useState<string[]>([]),
    [path, setPath] = useState(""),
    [project, setProject] = useState<ProjectState | null>(null);
  const [source, setSource] = useState(""),
    [namespace, setNamespace] = useState(""),
    [name, setName] = useState(""),
    [version, setVersion] = useState(""),
    [tool, setTool] = useState("codex");
  useEffect(() => {
    void ui.run(async () => {
      const recent = (await API.ListProjects()) ?? [];
      setRoots(recent);
      if (project) setProject(await API.OpenProject(project.root, false));
      else if (recent.length)
        setProject(await API.OpenProject(recent[0], false));
    });
  }, [ui.revision]);
  const editor = useRef<HTMLFormElement>(null);
  const open = (p: string, create: boolean) =>
    void ui.run(async () => {
      setProject(await API.OpenProject(p, create));
      setOpening(false);
      setRoots((await API.ListProjects()) ?? []);
    });
  return (
    <>
      <div className="project-toolbar">
        {roots.length > 0 && (
          <Field label={t("Project", "项目")}>
            <Select
              aria-label={t("Current project", "当前项目")}
              value={project?.root ?? ""}
              disabled={ui.busy}
              onChange={(e) => open(e.target.value, false)}
            >
              <option value="" disabled>
                {t("Choose a project", "选择项目")}
              </option>
              {roots.map((root) => (
                <option key={root} value={root}>
                  {roots.filter(
                    (r) => r.split(/[\\/]/).pop() === root.split(/[\\/]/).pop(),
                  ).length > 1
                    ? root
                    : root.split(/[\\/]/).pop()}
                </option>
              ))}
            </Select>
          </Field>
        )}
        <div className="actions">
          <button
            className={roots.length ? "" : "primary"}
            onClick={() => setOpening(true)}
          >
            {t("Open or add project", "打开或添加项目")}
          </button>
          {project && (
            <button
              aria-label={t("Project options", "项目选项")}
              onClick={() => setOptions(true)}
            >
              {t("Project options", "项目选项")}
            </button>
          )}
        </div>
      </div>
      {options && project && (
        <Modal
          title={t("Project options", "项目选项")}
          onClose={() => setOptions(false)}
        >
          <h3>{project.root.split(/[\\/]/).pop()}</h3>
          <p className="muted">{t("Project folder", "项目文件夹")}</p>
          <p className="path project-location">{project.root}</p>
          <p className="muted">
            {t("Your manifest, lockfile and installed skills will remain.")}
          </p>
          <div className="actions">
            <button onClick={() => setOptions(false)}>
              {t("Close", "关闭")}
            </button>
            <button
              className="text-danger"
              disabled={ui.busy}
              onClick={() => {
                const root = project.root;
                ui.confirm(
                  t("Remove project shortcut"),
                  t(
                    "Your manifest, lockfile and installed skills will remain.",
                  ),
                  async () => {
                    await API.ForgetProject(root);
                    setOptions(false);
                    const recent = (await API.ListProjects()) ?? [];
                    setRoots(recent);
                    setProject(
                      recent.length
                        ? await API.OpenProject(recent[0], false)
                        : null,
                    );
                  },
                );
              }}
            >
              {t("Remove from recent projects", "从最近项目中移除")}
            </button>
          </div>
        </Modal>
      )}
      {opening && (
        <Modal
          title={t("Open or add project", "打开或添加项目")}
          onClose={() => setOpening(false)}
        >
          <DirectoryField value={path} onChange={setPath} />
          <p className="muted">
            {t(
              "Open an existing skill manifest, or start one in the selected folder.",
              "打开已有技能清单，或在所选文件夹中创建一份。",
            )}
          </p>
          <div className="actions">
            <button
              disabled={ui.busy || !path}
              onClick={() => open(path, false)}
            >
              {t("Open project")}
            </button>
            <button
              className="primary"
              disabled={ui.busy || !path}
              onClick={() => open(path, true)}
            >
              {t("Create project manifest")}
            </button>
          </div>
        </Modal>
      )}
      {!project ? (
        roots.length ? (
          <p className="project-prompt">
            {t(
              "Choose a project to see its skills and installation status.",
              "选择项目，查看技能清单与安装状态。",
            )}
          </p>
        ) : (
          <Empty title={t("Keep your team in sync")}>
            {t(
              "Open a project with skillhub.json, or create a manifest for a folder.",
            )}
          </Empty>
        )
      ) : (
        <section className="project-detail">
          <div className="section-heading">
            <div>
              <h2>{t("Project skills", "项目技能")}</h2>
            </div>
            {((project.manifest.skills ?? []).length > 0 ||
              (project.lock?.skills ?? []).length > 0) && (
              <span className={`badge ${project.needs_lock ? "warning" : ""}`}>
                {project.needs_lock ? t("Needs locking") : t("Versions locked")}
              </span>
            )}
          </div>
          {((project.manifest.skills ?? []).length > 0 ||
            (project.lock?.skills ?? []).length > 0) && (
            <>
              <p className="muted">
                {t(
                  "Commit skillhub.json and skillhub.lock.json with your project. Restoring still requires access to each hub.",
                )}
              </p>
              <div className="actions wrap">
                <button
                  className="primary"
                  disabled={ui.busy}
                  onClick={() => ui.plan(() => API.PlanRestore(project.root))}
                >
                  {t("Review and apply changes", "预览并应用变更")}
                </button>
              </div>
            </>
          )}
          <div className="list">
            {(project.manifest.skills ?? []).length === 0 ? (
              <p className="muted">
                {t(
                  "Add a skill, then review and apply the project changes.",
                  "添加技能后，预览并应用项目变更。",
                )}
              </p>
            ) : (
              (project.manifest.skills ?? []).map((skill) => (
                <article key={`${skill.tool}/${skill.name}`}>
                  <div>
                    <h3>
                      {skill.source}/{skill.namespace}/{skill.name}
                    </h3>
                    <p className="muted">
                      v{skill.version} · {toolLabel(skill.tool)}
                    </p>
                  </div>
                  <div className="actions">
                    <button
                      disabled={ui.busy}
                      onClick={() => {
                        setEditing(true);
                        setEditingExisting(true);
                        setSource(skill.source);
                        setNamespace(skill.namespace);
                        setName(skill.name);
                        setVersion(skill.version);
                        setTool(skill.tool);
                      }}
                    >
                      {t("Edit version")}
                    </button>
                    <button
                      className="text-danger"
                      disabled={ui.busy}
                      onClick={() =>
                        ui.confirm(
                          `${t("Remove from manifest")} ${skill.name}`,
                          t(
                            "This changes the manifest only. To remove files, uninstall from Installed.",
                          ),
                          async () =>
                            setProject(
                              await API.RemoveProjectSkill(
                                project.root,
                                skill.tool,
                                skill.name,
                              ),
                            ),
                        )
                      }
                    >
                      {t("Remove from manifest")}
                    </button>
                  </div>
                </article>
              ))
            )}
          </div>
          <button
            className={
              (project.manifest.skills ?? []).length === 0 ? "primary" : ""
            }
            onClick={() => {
              setEditingExisting(false);
              setSource(ui.activeSource || "");
              setNamespace("");
              setName("");
              setVersion("");
              setEditing(true);
            }}
          >
            {t("Add a skill", "添加技能")}
          </button>
          {editing && (
            <Modal
              title={
                editingExisting
                  ? t("Edit version")
                  : t("Add a skill", "添加技能")
              }
              onClose={() => setEditing(false)}
            >
              <form
                ref={editor}
                className="form-grid"
                onSubmit={(e) => {
                  e.preventDefault();
                  if (!source || !namespace || !name || !version) return;
                  void ui.run(async () => {
                    setProject(
                      await API.SetProjectSkill(project.root, {
                        source,
                        namespace,
                        name,
                        version,
                        tool,
                      }),
                    );
                    setEditing(false);
                    ui.changed(
                      t("Manifest updated. Lock versions again to continue."),
                    );
                  });
                }}
              >
                <div className="full">
                  <Field label={t("Hub")}>
                    <SourceSelect
                      value={source}
                      onChange={(v) => {
                        setSource(v);
                        setNamespace("");
                        setName("");
                        setVersion("");
                      }}
                    />
                  </Field>
                </div>
                <ProjectSkillPicker
                  key={source}
                  source={source}
                  namespace={namespace}
                  name={name}
                  version={version}
                  onChange={(ns, n, v) => {
                    setNamespace(ns);
                    setName(n);
                    setVersion(v);
                  }}
                />
                <Field label={t("Tool")}>
                  <ToolSelect value={tool} onChange={setTool} />
                </Field>
                <div className="actions project-save">
                  <button
                    className="primary"
                    disabled={
                      ui.busy || !source || !namespace || !name || !version
                    }
                  >
                    {t("Save to manifest")}
                  </button>
                </div>
              </form>
            </Modal>
          )}
        </section>
      )}
    </>
  );
}
