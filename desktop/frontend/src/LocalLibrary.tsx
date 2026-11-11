import { ActionMenu } from "./ActionMenu";
import { Icon } from "./Icon";
import { useEffect, useState } from "react";
import {
  API,
  DirectoryField,
  Empty,
  Field,
  Modal,
  SourceSelect,
  toolLabel,
  useUI,
} from "./ui";
import { ResourceState, useResource } from "./community-ui";
import { Select } from "./Select";
import { SkillDocument } from "./SkillDocument";
import { ProjectSkillPicker } from "./ProjectSkillPicker";
import { t } from "./i18n";
import type {
  LocalSkill,
  LocalPlan,
  UpdateCheck,
} from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
import "./local-library.css";
import { skillTitle } from "./SkillIdentity";
import { ProvenanceDetails } from "./ProvenancePanel";

const stateLabel = (state: string) =>
  ({
    local: t("Local skill", "本地技能"),
    matched: t("Version verified", "版本已核实"),
    modified: t("Local differences", "本地有差异"),
    unavailable: t("Needs attention", "需要处理"),
  })[state] || state;
export function LocalPlanDetails({ plan }: { plan: LocalPlan }) {
  const label =
    {
      associate: t("Associate source", "关联来源"),
      replace: t("Back up and replace", "备份并替换"),
      edit: t("Save changes", "保存修改"),
      detach: t("Disconnect source", "取消来源关联"),
      remove: t("Uninstall", "卸载"),
      restore: t("Restore backup", "恢复备份"),
      copy: t("Create independent copy", "创建独立副本"),
    }[plan.action] || plan.action;
  return (
    <section className="local-plan">
      <h3>{label}</h3>
      <p className="path">{plan.target}</p>
      {plan.association && (
        <p>
          {plan.association.registry_url} · {plan.association.namespace}/
          {plan.association.name}@{plan.association.version}
        </p>
      )}
      {plan.action === "associate" ? (
        <p>
          {plan.differences?.length
            ? t(
                "Keep local files and associate this source. The installed version remains unverified until the contents match.",
                "保留本地文件并关联此来源。内容匹配前，安装版本显示为未核实。",
              )
            : t(
                "The contents match. Record this version without reinstalling files.",
                "内容一致，登记此版本即可，无需重新安装文件。",
              )}
        </p>
      ) : plan.action === "detach" ? (
        <p>
          {t(
            "Keep all files and saved configuration. Stop tracking updates from this source.",
            "保留所有文件与已保存配置，停止跟踪此来源的更新。",
          )}
        </p>
      ) : (
        <p>
          {t(
            "Existing files will be kept in a recovery point. Restore them from My library → Backups.",
            "原文件将保留为恢复点，可从“本地技能 → 备份”恢复。",
          )}
        </p>
      )}
      {(plan.locations?.length ?? 0) > 1 && (
        <div className="notice">
          <strong>{t("Shared folder", "共享目录")}</strong>
          {plan.locations?.map((l) => (
            <p key={l.path}>
              {toolLabel(l.tool)} · {l.scope} · {l.path}
            </p>
          ))}
        </div>
      )}
      {plan.link && (
        <p>
          {t(
            "This operation changes the link entry; its destination is preserved.",
            "此操作处理链接入口，保留链接目标内容。",
          )}
        </p>
      )}
      {(plan.differences ?? []).map((d) => (
        <details key={d.path}>
          <summary>
            {d.path} · {d.change}
          </summary>
          {d.text ? (
            <div className="local-diff">
              <div>
                <strong>{t("Local", "本地")}</strong>
                <pre>{d.before || "—"}</pre>
              </div>
              <div>
                <strong>{t("Selected version", "所选版本")}</strong>
                <pre>{d.after || "—"}</pre>
              </div>
            </div>
          ) : (
            <p>
              {t(
                "Binary, large text or file permissions changed.",
                "二进制文件、大文本或文件权限发生变化。",
              )}
            </p>
          )}
        </details>
      ))}
    </section>
  );
}
export function LocalSourceDialog({
  target,
  close,
  coordinate = "",
}: {
  target: string;
  close: () => void;
  coordinate?: string;
}) {
  const ui = useUI();
  const parts = coordinate.match(/^([^/]+)\/([^/]+)\/([^@]+)@(.+)$/);
  const [source, setSource] = useState(parts?.[1] || ui.activeSource),
    [namespace, setNamespace] = useState(parts?.[2] || ""),
    [name, setName] = useState(parts?.[3] || ""),
    [version, setVersion] = useState(parts?.[4] || ""),
    [replace, setReplace] = useState(false);
  return (
    <Modal
      stable
      wide
      title={t("Connect a local skill to its source", "关联本地技能的来源")}
      onClose={close}
    >
      <p className="path">{target}</p>
      <form
        className="form-grid"
        onSubmit={async (e) => {
          e.preventDefault();
          if (
            await ui.plan(() =>
              API.PlanLocalSource(
                target,
                `${source}/${namespace}/${name}@${version}`,
                replace,
              ),
            )
          )
            close();
        }}
      >
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
        <ProjectSkillPicker
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
        <Field label={t("Local files", "本地文件")}>
          <Select
            value={replace ? "replace" : "keep"}
            onChange={(e) => setReplace(e.target.value === "replace")}
          >
            <option value="keep">
              {t("Keep and associate", "保留并关联")}
            </option>
            <option value="replace">
              {t("Back up and replace", "备份并替换")}
            </option>
          </Select>
        </Field>
        <div className="actions full">
          <button type="button" onClick={close}>
            {t("Cancel")}
          </button>
          <button
            className="primary"
            disabled={ui.busy || !source || !name || !version}
          >
            {t("Compare and review", "比较并预览")}
          </button>
        </div>
      </form>
    </Modal>
  );
}
export function LocalReader({
  item,
  close,
}: {
  item: Pick<LocalSkill, "target" | "name" | "manager">;
  close: () => void;
}) {
  const ui = useUI();
  const [path, setPath] = useState("SKILL.md"),
    [editing, setEditing] = useState(false),
    [text, setText] = useState("");
  const resource = useResource(
    () => API.ReadLocalSkill(item.target, path),
    [item.target, path],
    ui.revision,
  );
  const document = resource.value;
  const changePath = (next: string) => {
    if (editing && text !== document?.file.content) {
      ui.confirm(
        t("Discard unsaved changes?", "放弃未保存的修改？"),
        t("Your saved files will stay unchanged.", "已保存的文件保持不变。"),
        async () => {
          setEditing(false);
          setPath(next);
        },
      );
      return;
    }
    setEditing(false);
    setPath(next);
  };
  const dismiss = () => {
    if (editing && text !== document?.file.content) {
      ui.confirm(
        t("Discard unsaved changes?", "放弃未保存的修改？"),
        t("Your saved files will stay unchanged.", "已保存的文件保持不变。"),
        async () => close(),
      );
      return;
    }
    close();
  };
  return (
    <Modal
      wide
      stable
      title={item.name}
      onClose={dismiss}
      footer={
        document && (
          <div className="actions">
            <button onClick={dismiss}>{t("Close")}</button>
            {document.file.text &&
              (item.manager === "local" || item.manager === "skillhub") &&
              (editing ? (
                <button
                  className="primary"
                  disabled={ui.busy || text === document.file.content}
                  onClick={async () => {
                    if (
                      await ui.plan(() =>
                        API.PlanLocalEdit(
                          item.target,
                          path,
                          document.fingerprint,
                          text,
                        ),
                      )
                    )
                      close();
                  }}
                >
                  {t("Review changes", "预览修改")}
                </button>
              ) : (
                <button
                  onClick={() => {
                    setText(document.file.content);
                    setEditing(true);
                  }}
                >
                  {t("Edit", "编辑")}
                </button>
              ))}
          </div>
        )
      }
    >
      <ResourceState {...resource} retry={resource.reload}>
        {document && (
          <>
            <Field label={t("File", "文件")}>
              <Select value={path} onChange={(e) => changePath(e.target.value)}>
                {document.files?.map((f) => (
                  <option key={f.path} value={f.path}>
                    {f.path}
                  </option>
                ))}
              </Select>
            </Field>
            {editing ? (
              <textarea
                className="local-editor"
                aria-label={t("File contents", "文件内容")}
                value={text}
                onChange={(e) => setText(e.target.value)}
              />
            ) : document.file.text ? (
              <SkillDocument
                text={document.file.content}
                path={path}
                onNavigate={(p) => changePath(p)}
              />
            ) : (
              <p>
                {t(
                  "Binary file. Open the folder to view it.",
                  "二进制文件，请打开目录查看。",
                )}
              </p>
            )}
          </>
        )}
      </ResourceState>
    </Modal>
  );
}
export function LocalLibrary() {
  const ui = useUI();
  const inventory = useResource(() => API.LocalInventory(), [], ui.revision);
  const suites = useResource(() => API.Suites(), [], ui.revision);
  const [filter, setFilter] = useState(""),
    [hidden, setHidden] = useState(false),
    [kind, setKind] = useState("all"),
    [folders, setFolders] = useState(false),
    [folder, setFolder] = useState(""),
    [backups, setBackups] = useState(false),
    [reader, setReader] = useState<LocalSkill | null>(null),
    [sourceItem, setSourceItem] = useState<LocalSkill | null>(null),
    [copy, setCopy] = useState<LocalSkill | null>(null),
    [destination, setDestination] = useState("");
  const [checks, setChecks] = useState<UpdateCheck[]>([]);
  const records = useResource(() => API.LocalBackups(), [], ui.revision);
  useEffect(() => {
    const refresh = () => inventory.reload();
    window.addEventListener("focus", refresh);
    return () => window.removeEventListener("focus", refresh);
  }, []);
  useEffect(() => {
    setChecks([]);
  }, [ui.revision]);
  const items = inventory.value?.skills ?? [];
  const visible = items.filter(
    (i) =>
      i.hidden === hidden &&
      (kind === "all" ||
        (kind === "connected" ? !!i.association : !i.association)) &&
      `${i.name} ${i.association ? skillTitle(i.association) : ""} ${i.description} ${i.target} ${i.association?.registry_url ?? ""} ${(i.locations ?? []).map((l) => `${toolLabel(l.tool)} ${l.project || ""}`).join(" ")}`
        .toLowerCase()
        .includes(filter.toLowerCase()),
  );
  const checkUpdates = (target = "") =>
    void ui.run(async () =>
      setChecks((await API.CheckUpdatesFrom(target, "")) ?? []),
    );
  return (
    <ResourceState {...inventory} retry={inventory.reload}>
      <div className="toolbar library-toolbar">
        <label className="search-field">
          <Icon name="explore" />
          <input
            aria-label={t("Filter installed skills")}
            placeholder={t("Filter by name, tool or project…")}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
        </label>
        <Select
          aria-label={t("Skill source", "技能来源")}
          value={kind}
          onChange={(e) => setKind(e.target.value)}
        >
          <option value="all">{t("All skills", "全部技能")}</option>
          <option value="connected">
            {t("Connected to a Hub", "已关联 Hub")}
          </option>
          <option value="local">{t("Local only", "仅本地")}</option>
        </Select>
        <span className="library-count">
          {visible.length} {t("skills", "个技能")}
        </span>
        <button
          className="icon-button"
          title={t("Refresh")}
          aria-label={t("Refresh")}
          onClick={() => inventory.reload()}
        >
          <Icon name="refresh" />
        </button>
        <ActionMenu label={t("Library options", "技能库选项")}>
          <button
            disabled={ui.busy || !items.some((i) => i.installation)}
            onClick={() => checkUpdates()}
          >
            {t("Check for updates")}
          </button>
          <button onClick={() => setFolders(true)}>
            {t("Folders", "扫描目录")}
          </button>
          <button onClick={() => setBackups(true)}>
            {t("Backups", "备份")}
          </button>
          <hr />
          <button onClick={() => setHidden(!hidden)}>
            {hidden
              ? t("Show visible skills", "查看可见技能")
              : t("Hidden skills", "已隐藏技能")}
          </button>
        </ActionMenu>
      </div>
      {hidden && (
        <div className="library-filter-note">
          <span>{t("Showing hidden skills", "正在查看已隐藏技能")}</span>
          <button className="text-button" onClick={() => setHidden(false)}>
            {t("Back to library", "返回技能库")}
          </button>
        </div>
      )}
      {(inventory.value?.warnings ?? []).map((w) => (
        <p className="notice" key={w}>
          {w}
        </p>
      ))}
      {!visible.length ? (
        <Empty
          title={
            filter || kind !== "all"
              ? t("No matching installations")
              : hidden
                ? t("No hidden skills", "没有已隐藏的技能")
                : t("No local skills found", "尚未发现本地技能")
          }
        >
          <button
            onClick={() => {
              if (filter || kind !== "all") {
                setFilter("");
                setKind("all");
              } else if (hidden) setHidden(false);
              else setFolders(true);
            }}
          >
            {filter || kind !== "all"
              ? t("Clear filters", "清除筛选")
              : hidden
                ? t("Back to library", "返回技能库")
                : t("Add a folder", "添加目录")}
          </button>
        </Empty>
      ) : (
        <div className="list library-list">
          {visible.map((item) => {
            const a = item.association;
            const check = checks.find(
              (c) => c.installation.target === item.target,
            );
            const parents = (suites.value ?? []).filter(
              (s) =>
                s.target === item.target ||
                s.members?.some((m) => m.target === item.target),
            );
            const own = item.manager === "local" || item.manager === "skillhub";
            return (
              <article className="installed-row" key={item.id}>
                <div className="installation-content">
                  <div className="installation-heading">
                    <h2>
                      <button
                        className="local-title"
                        onClick={() => setReader(item)}
                      >
                        {a ? skillTitle(a) : item.name}
                      </button>
                    </h2>
                    <span className="badge">{stateLabel(item.state)}</span>
                    {a && (
                      <span className="version">
                        {item.state === "matched"
                          ? `v${a.version}`
                          : t("Version unverified", "版本未核实")}
                      </span>
                    )}
                  </div>
                  <p className="local-description">{item.description}</p>
                  <div className="installation-meta">
                    {item.locations?.map((l) => (
                      <span key={`${l.path}/${l.tool}`} title={l.path}>
                        {toolLabel(l.tool) || t("Folder", "目录")} ·{" "}
                        {l.scope === "global"
                          ? t("Global")
                          : l.project?.split(/[\\/]/).pop() ||
                            t("Local", "本地")}
                        {l.linked ? ` · ${t("Link", "链接")}` : ""}
                      </span>
                    ))}
                  </div>
                  {a && (
                    <p className="skill-coordinate">
                      {ui.sources.find((s) => s.registry_id === a.registry_id)
                        ?.display_name || a.registry_url}{" "}
                      · {a.namespace}/{a.name}
                    </p>
                  )}
                  {!own && (
                    <p className="muted">
                      {t("Maintained by", "维护工具")}: {item.manager}
                    </p>
                  )}
                  {item.problem && <p className="notice">{item.problem}</p>}
                  {check?.error && (
                    <p className="notice">
                      {t(
                        "Update check unavailable. Check the source connection.",
                        "无法检查更新，请检查来源连接。",
                      )}
                    </p>
                  )}
                  {check?.version_unavailable && (
                    <p className="notice">
                      {t(
                        "This version is no longer available from its Hub. Local files are kept.",
                        "Hub 已无法提供此版本，本地文件仍保留。",
                      )}
                    </p>
                  )}
                  {check &&
                    !check.error &&
                    !check.available_version &&
                    !check.version_unavailable && (
                      <p className="installation-current">
                        {t("Up to date", "已是最新")}
                      </p>
                    )}
                  <details className="installation-details">
                    <summary>{t("Location and source", "位置与来源")}</summary>
                    <p className="path">{item.target}</p>
                    {a && (
                      <ProvenanceDetails
                        value={a.provenance}
                        version={a.version}
                        digest={a.digest}
                      />
                    )}{" "}
                    {item.locations
                      ?.filter((l) => l.linked)
                      .map((l) => (
                        <div className="actions" key={l.path}>
                          <span className="path">{l.path}</span>
                          <button
                            onClick={() =>
                              ui.plan(() =>
                                API.PlanLocalAction(l.path, "remove", ""),
                              )
                            }
                          >
                            {t("Remove this link", "移除此链接")}
                          </button>
                        </div>
                      ))}
                  </details>
                </div>
                <div className="actions installation-actions">
                  {check?.available_version && own && !parents.length && (
                    <button
                      className="primary"
                      disabled={ui.busy}
                      onClick={() => setSourceItem(item)}
                    >
                      {t("Review update", "预览更新")} v
                      {check.available_version}
                    </button>
                  )}
                  <button onClick={() => setReader(item)}>
                    {t("View files", "查看文件")}
                  </button>
                  <ActionMenu
                    label={`${t("Actions", "操作")}: ${a ? skillTitle(a) : item.name}`}
                    iconOnly
                  >
                    <button
                      onClick={() =>
                        void ui.run(() => API.RevealLocalSkill(item.target))
                      }
                    >
                      {t("Open folder", "打开目录")}
                    </button>
                    <button
                      onClick={() =>
                        ui.navigate(
                          `configuration?${new URLSearchParams({ target: item.target })}`,
                        )
                      }
                    >
                      {t("Configure", "配置")}
                    </button>
                    {own && !parents.length && (
                      <button
                        disabled={ui.busy}
                        onClick={() => setSourceItem(item)}
                      >
                        {a
                          ? t("Source and updates", "来源与更新")
                          : t("Associate source", "关联来源")}
                      </button>
                    )}
                    <hr />
                    <button
                      onClick={() => {
                        setCopy(item);
                        setDestination("");
                      }}
                    >
                      {t("Make a copy", "创建副本")}
                    </button>
                    <button
                      onClick={() =>
                        void ui.run(async () => {
                          await API.SetLocalHidden(item.target, !item.hidden);
                          ui.refresh();
                        })
                      }
                    >
                      {item.hidden
                        ? t("Show skill", "显示技能")
                        : t("Hide skill", "隐藏技能")}
                    </button>
                    {a && own && !parents.length && (
                      <button
                        onClick={() =>
                          ui.plan(() =>
                            API.PlanLocalAction(item.target, "detach", ""),
                          )
                        }
                      >
                        {t("Disconnect source", "取消来源关联")}
                      </button>
                    )}
                    {own && !parents.length && (
                      <button
                        className="text-danger"
                        onClick={() =>
                          ui.plan(() =>
                            API.PlanLocalAction(item.target, "remove", ""),
                          )
                        }
                      >
                        {t("Uninstall")}
                      </button>
                    )}
                    {parents.map((s) => (
                      <div key={s.target}>
                        <span className="menu-label">
                          {t("Suite", "套装")}: {s.coordinate.Name}
                        </span>
                        <button
                          onClick={() =>
                            ui.plan(() => API.PlanUpdateFrom(s.target, "", ""))
                          }
                        >
                          {t("Update suite", "更新套装")}
                        </button>
                        <button
                          onClick={() =>
                            ui.plan(() => API.PlanRemove(s.target))
                          }
                        >
                          {t("Uninstall suite", "卸载套装")}
                        </button>
                      </div>
                    ))}
                  </ActionMenu>
                </div>
              </article>
            );
          })}
        </div>
      )}
      {reader && <LocalReader item={reader} close={() => setReader(null)} />}
      {sourceItem && (
        <LocalSourceDialog
          target={sourceItem.target}
          coordinate={
            sourceItem.association
              ? `${sourceItem.association.source}/${sourceItem.association.namespace}/${sourceItem.association.name}@${checks.find((c) => c.installation.target === sourceItem.target)?.available_version || sourceItem.association.version}`
              : ""
          }
          close={() => setSourceItem(null)}
        />
      )}
      {folders && (
        <Modal
          title={t("Skill folders", "技能目录")}
          onClose={() => setFolders(false)}
        >
          <p>
            {t(
              "Standard tool folders and saved projects are discovered automatically. Add a skill folder or a folder containing skills.",
              "自动发现工具的标准目录与已添加项目。还可以添加单个技能目录或包含技能的目录。",
            )}
          </p>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              void ui.run(async () => {
                await API.SetLocalRoot(folder, true);
                setFolder("");
                ui.refresh();
              });
            }}
          >
            <DirectoryField value={folder} onChange={setFolder} required />
            <div className="actions">
              <button className="primary" disabled={ui.busy || !folder}>
                {t("Add folder", "添加目录")}
              </button>
            </div>
          </form>
          {inventory.value?.roots?.map((root) => (
            <div className="local-folder" key={root}>
              <span className="path">{root}</span>
              <button
                onClick={() =>
                  void ui.run(async () => {
                    await API.SetLocalRoot(root, false);
                    ui.refresh();
                  })
                }
              >
                {t("Stop scanning", "停止扫描")}
              </button>
            </div>
          ))}
        </Modal>
      )}
      {backups && (
        <Modal
          wide
          stable
          title={t("Recovery points", "恢复点")}
          onClose={() => setBackups(false)}
        >
          {records.value?.length ? (
            records.value.map((b) => (
              <article className="local-backup" key={b.id}>
                <div>
                  <strong>{b.target.split(/[\\/]/).pop()}</strong>
                  <p>
                    {new Date(b.created_at).toLocaleString()} · {b.action}
                  </p>
                  <p className="path">{b.target}</p>
                </div>
                <button
                  disabled={b.suite}
                  title={
                    b.suite
                      ? t(
                          "Use the suite workflow to restore its members together.",
                          "请通过套装流程一起恢复其成员。",
                        )
                      : undefined
                  }
                  onClick={async () => {
                    if (
                      await ui.plan(() =>
                        API.PlanLocalAction(b.id, "restore", ""),
                      )
                    )
                      setBackups(false);
                  }}
                >
                  {t("Review restore", "预览恢复")}
                </button>
                <button
                  className="text-danger"
                  onClick={() =>
                    ui.confirm(
                      t("Delete this recovery point?", "删除此恢复点？"),
                      b.target,
                      async () => {
                        await API.DeleteLocalBackup(b.id);
                        ui.refresh();
                      },
                      "danger",
                    )
                  }
                >
                  {t("Delete backup", "删除备份")}
                </button>
              </article>
            ))
          ) : (
            <Empty title={t("No recovery points yet", "暂无恢复点")}>
              {t(
                "File replacements, edits and uninstalls create recovery points.",
                "文件替换、编辑和卸载会创建恢复点。",
              )}
            </Empty>
          )}
        </Modal>
      )}
      {copy && (
        <Modal
          title={t("Create independent copy", "创建独立副本")}
          onClose={() => setCopy(null)}
        >
          <form
            onSubmit={async (e) => {
              e.preventDefault();
              if (
                await ui.plan(() =>
                  API.PlanLocalAction(
                    copy.target,
                    "copy",
                    `${destination.replace(/[\\/]$/, "")}/${copy.name}`,
                  ),
                )
              )
                setCopy(null);
            }}
          >
            <DirectoryField
              value={destination}
              onChange={setDestination}
              required
            />
            <p>
              {t(
                "The selected folder will contain a new skill subfolder.",
                "将在所选目录下创建一个新的技能子目录。",
              )}
            </p>
            <div className="actions">
              <button className="primary" disabled={ui.busy || !destination}>
                {t("Review copy", "预览副本")}
              </button>
            </div>
          </form>
        </Modal>
      )}
    </ResourceState>
  );
}
