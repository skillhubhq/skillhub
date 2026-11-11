import { ResourceState, useResource } from "./community-ui";
import { ProjectSkillPicker } from "./ProjectSkillPicker";
import { Select } from "./Select";
import { t } from "./i18n";
import { useEffect, useRef, useState } from "react";
import type { ProjectState } from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
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
export { LocalLibrary as Installed } from "./LocalLibrary";
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
