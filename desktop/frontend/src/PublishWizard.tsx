import { SuiteComposer } from "./SuiteComposer";
import { registerDraftSave } from "./pendingDraft";
import { scrollPageToTop } from "./pageScroll";
import { validProvenance, type Provenance } from "./provenance";
import { ProvenanceDetails, ProvenanceEditor } from "./ProvenancePanel";
import { SkillEditor } from "./SkillEditor";
import { repackDraft } from "./draft-archive";
import { Select } from "./Select";
import { SkillIconPicker, SkillSymbol, skillTitle } from "./SkillIdentity";
import { t } from "./i18n";
import { useResource } from "./community-ui";
import { Icon } from "./Icon";
import { SkillDocument } from "./SkillDocument";
import { useEffect, useMemo, useRef, useState } from "react";
import {
  API,
  DirectoryField,
  Field,
  Modal,
  Empty,
  splitList,
  statusLabel,
  useUI,
  visibilityLabel,
} from "./ui";
import { isWeb } from "./platform";
import {
  SkillIcon,
  Visibility,
  ReleaseStatus,
} from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";
import type {
  AdminRow,
  DraftInfo,
  PublishForm,
} from "../bindings/github.com/skillhubhq/skillhub/desktop/models";
import type { ReleaseInfo } from "../bindings/github.com/skillhubhq/skillhub/internal/client/models";
export function PublishWizard({ source }: { source: string }) {
  const ui = useUI();
  const participation = useResource(
    () => API.CommunityParticipation(source),
    [source],
  );
  const previousID =
    new URLSearchParams(location.hash.split("?")[1] ?? "").get("from") || "";
  const previous = useResource(
    () =>
      previousID
        ? API.PreviewSubmission(source, previousID)
        : Promise.resolve(null),
    [source, previousID],
  );
  const [editing, setEditing] = useState(false);
  const [savedDrafts, setSavedDrafts] = useState<Awaited<
    ReturnType<typeof API.ListDrafts>
  > | null>(null);
  const resumeDraft = async (id: string) => {
    const work = await API.OpenDraft(id);
    setWorkspace({ ...work, files: work.files ?? [] });
    setDirectory(work.directory);
    setDraft(null);
    try {
      setDraft(await API.InspectDirectory(work.directory));
    } catch {
      /* 未完成的内容仍可继续编辑。 */
    }
    setSavedDrafts(null);
    setEditing(true);
  };

  const [workspace, setWorkspace] = useState<{
    id: string;
    directory: string;
    files: string[];
  } | null>(null);
  const startEditor = async (blank = false, previousVersion = false) => {
    if (isWeb) {
      setEditing(true);
      return;
    }
    if (!blank && !previousVersion && workspace?.directory === directory) {
      setEditing(true);
      return;
    }
    const target =
      previousVersion && previous.value
        ? previous.value.target
        : {
            source: "",
            namespace: "",
            name: "",
            version: "",
            submission_id: "",
            digest: "",
          };
    const work = await API.CreateDraft(
      blank || previousVersion ? "" : directory,
      target,
    );
    setWorkspace({ ...work, files: work.files ?? [] });
    setDirectory(work.directory);
    setDraft(await API.InspectDirectory(work.directory));
    setEditing(true);
  };
  const refreshDraft = async () => {
    if (!isWeb && workspace)
      setDraft(await API.InspectDirectory(workspace.directory));
  };
  const [step, setStep] = useState(0),
    [directory, setDirectory] = useState(""),
    [draft, setDraft] = useState<DraftInfo | null>(null),
    [archive, setArchive] = useState<Uint8Array | null>(null),
    [namespace, setNamespace] = useState(""),
    [category, setCategory] = useState(""),
    [origin, setOrigin] = useState<Provenance | null>(null),
    [displayNameEN, setDisplayNameEN] = useState(""),
    [displayNameZH, setDisplayNameZH] = useState(""),
    [icon, setIcon] = useState<SkillIcon>(
      SkillIcon.SkillIcon_SKILL_ICON_UNSPECIFIED,
    ),
    [version, setVersion] = useState("1.0.0"),
    [tags, setTags] = useState(""),
    [visibility, setVisibility] = useState<Visibility>(
      Visibility.Visibility_VISIBILITY_MEMBERS,
    ),
    [groups, setGroups] = useState<string[]>([]),
    [options, setOptions] = useState<{
      namespaces: AdminRow[];
      categories: AdminRow[];
      groups: AdminRow[];
    } | null>(null),
    [reload, setReload] = useState(0),
    [result, setResult] = useState<ReleaseInfo | null>(null);
  const draftIdentity = useResource(() => API.Identity(source), [source]);
  const [restored, setRestored] = useState("");
  const [draftSave, setDraftSave] = useState("");
  const savedPayload = useRef("");
  const saveQueue = useRef(Promise.resolve());
  const account = draftIdentity.value?.id || "";
  const draftScope = `${source}/${account}/${previousID}`;
  useEffect(() => {
    if (!options || !account || restored === draftScope) return;
    let active = true;
    void ui.run(async () => {
      const raw = await API.LoadPublicationDraft(source, account);
      if (!active) return;
      if (raw) {
        const saved = JSON.parse(raw);
        if (saved.previousID === previousID) {
          setNamespace(
            options.namespaces.some((n) => n.name === saved.namespace)
              ? saved.namespace
              : "",
          );
          setCategory(
            options.categories.some((c) => c.name === saved.category)
              ? saved.category
              : "",
          );
          setGroups(
            (saved.groups || []).filter((g: string) =>
              options.groups.some((item) => item.id === g),
            ),
          );
          setOrigin(saved.origin || null);
          setDisplayNameEN(saved.displayNameEN || "");
          setDisplayNameZH(saved.displayNameZH || "");
          setIcon(saved.icon || 0);
          setVersion(saved.version || "1.0.0");
          setTags(saved.tags || "");
          setVisibility(saved.visibility || 2);
          if (isWeb && saved.archive) {
            const bytes = Uint8Array.from(atob(saved.archive), (c) =>
              c.charCodeAt(0),
            );
            const bundle = await (
              await import("./web-api")
            ).inspectArchive(bytes);
            if (!active) return;
            setArchive(bytes);
            setDraft({
              name: bundle.name,
              description: bundle.description,
              markdown: bundle.markdown,
              digest: bundle.digest,
              files: Object.keys(bundle.files),
            });
          } else if (!isWeb && saved.directory) {
            setDirectory(saved.directory);
            if (saved.workspace?.id) {
              const work = await API.OpenDraft(saved.workspace.id);
              setWorkspace({ ...work, files: work.files ?? [] });
            }
            try {
              setDraft(await API.InspectDirectory(saved.directory));
            } catch {
              setDraftSave(
                t(
                  "Open the editor to repair this draft.",
                  "请打开编辑器修复此草稿。",
                ),
              );
            }
          }
        }
      }
      if (active) setRestored(draftScope);
    });
    return () => {
      active = false;
    };
  }, [options, account, draftScope]);
  const encodedArchive = useMemo(
    () =>
      archive
        ? btoa(
            Array.from({ length: Math.ceil(archive.length / 32768) }, (_, i) =>
              String.fromCharCode(
                ...archive.subarray(i * 32768, (i + 1) * 32768),
              ),
            ).join(""),
          )
        : "",
    [archive],
  );
  const snapshot = useMemo(
    () =>
      JSON.stringify({
        previousID,
        directory,
        workspace,
        namespace,
        category,
        origin,
        displayNameEN,
        displayNameZH,
        icon,
        version,
        tags,
        visibility,
        groups,
        archive: encodedArchive,
      }),
    [
      previousID,
      directory,
      workspace,
      namespace,
      category,
      origin,
      displayNameEN,
      displayNameZH,
      icon,
      version,
      tags,
      visibility,
      groups,
      encodedArchive,
    ],
  );
  useEffect(() => {
    if (restored !== draftScope || result) return;
    const payload = snapshot;
    const save = async () => {
      if (savedPayload.current === payload) return;
      saveQueue.current = saveQueue.current
        .catch(() => {})
        .then(async () => {
          await API.SavePublicationDraft(source, account, payload);
          savedPayload.current = payload;
        });
      await saveQueue.current;
    };
    const unregister = registerDraftSave(save);
    const timer = setTimeout(() => {
      void save()
        .then(() =>
          setDraftSave(t("Draft saved on this device", "草稿已保存在此设备")),
        )
        .catch(() =>
          setDraftSave(
            t(
              "Draft could not be saved. Retry before leaving.",
              "草稿保存失败，请重试后再离开。",
            ),
          ),
        );
    }, 500);
    const beforeUnload = (event: BeforeUnloadEvent) => {
      if (savedPayload.current !== payload) {
        event.preventDefault();
        event.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", beforeUnload);
    return () => {
      clearTimeout(timer);
      unregister();
      window.removeEventListener("beforeunload", beforeUnload);
    };
  }, [snapshot, restored, draftScope, result]);
  useEffect(() => {
    scrollPageToTop();
  }, [step]);
  useEffect(() => {
    if (!options || restored === draftScope) return;
    if (previous.value) {
      const r = previous.value.release;
      setNamespace(
        options.namespaces.some((n) => n.name === r.namespace)
          ? r.namespace
          : "",
      );
      setVersion(
        r.status === ReleaseStatus.ReleaseStatus_RELEASE_STATUS_PUBLISHED ||
          r.status === ReleaseStatus.ReleaseStatus_RELEASE_STATUS_YANKED
          ? ""
          : r.version,
      );
      setOrigin(r.provenance || null);
      setDisplayNameEN(r.display_name_en || "");
      setDisplayNameZH(r.display_name_zh || "");
      setIcon(r.icon || 0);
      setVisibility(r.visibility);
      setGroups(
        (r.groups ?? []).filter((id) =>
          options.groups.some((g) => g.id === id),
        ),
      );
    } else if (!previousID && options.namespaces.length === 1)
      setNamespace(options.namespaces[0].name);
  }, [options, previous.value]);
  const mutation = useRef<{ key: string; id: string } | null>(null);
  const fileGeneration = useRef(0);
  useEffect(() => {
    let active = true;
    setOptions(null);
    setNamespace("");
    setCategory("");
    setGroups([]);
    setStep(0);
    void ui.run(async () => {
      const all = async (kind: string) => {
        let token = "";
        const rows: AdminRow[] = [];
        do {
          const p = await API.AdminList(source, kind, "", token);
          rows.push(...(p.rows ?? []));
          token = p.next_page;
        } while (token);
        return rows;
      };
      const [namespaces, categories] = await Promise.all([
        all("submission-namespaces"),
        all("categories"),
      ]);
      // 群组目录并非所有发布者都有列举权；未获权时不提供猜测 ID 的入口。
      let groupRows: AdminRow[] = [];
      try {
        groupRows = await all("groups");
      } catch {}
      if (active)
        setOptions({
          namespaces,
          categories: categories.filter((c) => c.enabled),
          groups: groupRows,
        });
    });
    return () => {
      active = false;
      fileGeneration.current++;
    };
  }, [source, ui.revision, reload]);
  const loadFile = async (file?: File) => {
    const generation = ++fileGeneration.current;
    setDraft(null);
    setArchive(null);
    if (!file) return;
    await ui.run(async () => {
      if (file.size > 20 * 1024 * 1024)
        throw new Error(t("Package must be smaller than 20 MiB."));
      const bytes = new Uint8Array(await file.arrayBuffer());
      const { inspectArchive } = await import("./web-api");
      const bundle = await inspectArchive(bytes);
      if (generation !== fileGeneration.current) return;
      setArchive(bytes);
      setDraft({
        name: bundle.name,
        description: bundle.description,
        markdown: bundle.markdown,
        digest: bundle.digest,
        files: Object.keys(bundle.files),
      });
    });
  };
  const identity = {
    name: draft?.name || "",
    display_name_en: displayNameEN.trim(),
    display_name_zh: displayNameZH.trim(),
    icon,
  };
  const validName = (value: string) =>
    [...value.trim()].length <= 80 &&
    !/[\u0000-\u001f\u007f-\u009f\u202a-\u202e\u2066-\u2069]/.test(value);
  const valid =
    (!origin || validProvenance(origin, draft?.files ?? [])) &&
    validName(displayNameEN) &&
    validName(displayNameZH) &&
    !!draft &&
    /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$/.test(
      version,
    ) &&
    (!previousID ||
      (!!previous.value &&
        (version !== previous.value.release.version ||
          previous.value.release.status ===
            ReleaseStatus.ReleaseStatus_RELEASE_STATUS_PENDING ||
          previous.value.release.status ===
            ReleaseStatus.ReleaseStatus_RELEASE_STATUS_REJECTED))) &&
    !!options?.namespaces.some((n) => n.name === namespace) &&
    (!category || !!options.categories.some((c) => c.name === category)) &&
    (visibility !== Visibility.Visibility_VISIBILITY_GROUPS ||
      groups.length > 0);
  async function submit() {
    if (!valid || !draft) return;
    await saveQueue.current.catch(() => {});
    const form: PublishForm = {
      provenance: origin,
      display_name_en: identity.display_name_en,
      display_name_zh: identity.display_name_zh,
      icon,
      source,
      directory,
      namespace,
      version,
      visibility,
      groups,
      category,
      tags: splitList(tags),
      request_id: "",
    };
    const key = JSON.stringify([form, draft.digest]);
    if (mutation.current?.key !== key)
      mutation.current = { key, id: await API.NewRequestID() };
    form.request_id = mutation.current.id;
    const release = isWeb
      ? await (await import("./web-api")).publishPackage(form, archive!)
      : await API.PublishChecked(form, draft.digest);
    await saveQueue.current.catch(() => {});
    await API.SavePublicationDraft(source, account, "");
    setResult(release);
    mutation.current = null;
  }
  if (result)
    return (
      <div className="panel publish-result">
        <Icon name="review" />
        <h2>
          {result.status === ReleaseStatus.ReleaseStatus_RELEASE_STATUS_PENDING
            ? t("Submitted for review", "已提交审核")
            : t("Your skill is published", "技能已发布")}
        </h2>
        <span className="badge">{statusLabel(result.status)}</span>
        <h3>{skillTitle(result)}</h3>
        <span className="skill-coordinate">
          {result.namespace}/{result.name} · {result.version}
        </span>
        <p>
          {result.status === ReleaseStatus.ReleaseStatus_RELEASE_STATUS_PENDING
            ? t("Submitted. An independent reviewer will take a look.")
            : t("This version is available according to your space policy.")}
        </p>
        <p className="muted">
          {result.status === ReleaseStatus.ReleaseStatus_RELEASE_STATUS_PENDING
            ? t(
                "Track this version in My releases. Review feedback will appear there; it is not available to install yet.",
                "在「我的发布」中跟踪当前版本并查看审核反馈。审核通过前无法安装。",
              )
            : t(
                "People with access can now discover and install this exact version.",
                "有访问权限的用户现在可以发现并安装当前版本。",
              )}
        </p>
        <div className="actions">
          <button
            className="primary"
            onClick={() =>
              ui.navigate(
                `submissions?source=${encodeURIComponent(source)}&release=${encodeURIComponent(result.id)}`,
              )
            }
          >
            {t("View this submission", "查看本次提交")}
          </button>
          {result.status ===
            ReleaseStatus.ReleaseStatus_RELEASE_STATUS_PUBLISHED && (
            <button
              onClick={() =>
                ui.navigate(
                  `discover?${new URLSearchParams({ source, skill: `${result.namespace}/${result.name}`, version: result.version })}`,
                )
              }
            >
              {t("View published skill", "查看已发布技能")}
            </button>
          )}
        </div>
      </div>
    );
  return (
    <div className="publishing-journey">
      <SuiteComposer source={source} />
      <p className="muted" role="status">
        {draftSave}
      </p>
      {previousID && (
        <section className="revision-context">
          <h2>{t("Submit an updated version", "提交修改后的版本")}</h2>
          {previous.loading ? (
            <p>{t("Loading previous submission…", "正在加载上次提交…")}</p>
          ) : previous.error ? (
            <>
              <p role="alert">{previous.error}</p>
              <button onClick={previous.reload}>{t("Try again")}</button>
            </>
          ) : (
            previous.value && (
              <>
                <p>
                  {previous.value.release.namespace}/
                  {previous.value.release.name} · v
                  {previous.value.release.version}
                </p>
                {previous.value.release.reason && (
                  <blockquote>{previous.value.release.reason}</blockquote>
                )}
                <p>
                  {t(
                    "Update your skill folder or ZIP, then choose a new version number. The previous submission and its feedback stay available.",
                    "先修改技能目录或 ZIP，再填写新的版本号。上次提交及其反馈会保留。",
                  )}
                </p>
              </>
            )
          )}
        </section>
      )}
      {savedDrafts !== null && (
        <Modal
          stable
          title={t("Saved drafts", "已保存草稿")}
          onClose={() => setSavedDrafts(null)}
        >
          <p className="muted">
            {t(
              "Saved skill files stay on this device. Choose publication details when you are ready to submit.",
              "已保存的技能文件保留在此设备上；提交前再选择发布信息。",
            )}
          </p>
          {savedDrafts.length === 0 ? (
            <Empty title={t("No saved drafts", "暂无已保存草稿")} />
          ) : (
            <div className="saved-draft-list">
              {savedDrafts.map((item) => (
                <button
                  key={item.id}
                  disabled={ui.busy}
                  onClick={() => void ui.run(() => resumeDraft(item.id))}
                >
                  <span>
                    <strong>
                      {item.name || t("Untitled skill", "未命名技能")}
                    </strong>
                    <small>
                      {item.id.slice(0, 8)} ·{" "}
                      {new Date(item.updated_at).toLocaleString()}
                    </small>
                  </span>
                  <span>{t("Continue editing", "继续编辑")} →</span>
                </button>
              ))}
            </div>
          )}
        </Modal>
      )}
      <div className="publish-layout">
        <div className="panel publish-main">
          {participation.value?.community_mode && (
            <p className="notice" role="status">
              {t(
                "Every submission is reviewed before publication.",
                "每次提交都需审核后发布。",
              )}{" "}
              {t("Submitted in the last 24 hours", "过去 24 小时已提交")}:{" "}
              {participation.value.submissions_today} /{" "}
              {participation.value.submission_limit}
            </p>
          )}
          <ol className="steps">
            {[t("Preview"), t("Details"), t("Confirm")].map((label, i) => (
              <li
                key={label}
                aria-current={step === i ? "step" : undefined}
                className={step === i ? "active" : ""}
              >
                <button
                  disabled={i > step || ui.busy}
                  onClick={() => setStep(i)}
                >
                  {i < step ? <Icon name="review" /> : <span>{i + 1}</span>}
                  {label}
                </button>
              </li>
            ))}
          </ol>
          {!options && !ui.busy && (
            <div className="notice">
              {t("Publication options are not ready.")}
              <button
                onClick={() => setReload((n) => n + 1)}
                disabled={ui.busy}
              >
                {t("Reload options")}
              </button>
            </div>
          )}
          {options?.namespaces.length === 0 && (
            <Empty title={t("No publishing spaces available")}>
              {t(
                "Switch accounts or ask your hub administrator for publishing access.",
              )}
            </Empty>
          )}
          {step === 0 && (
            <>
              <h2>
                {isWeb
                  ? t("Choose a skill package")
                  : t("Choose a skill folder")}
              </h2>
              <p className="muted">
                {t("Preview SKILL.md and its files before sharing.")}
              </p>
              {isWeb ? (
                <Field label={t("Skill package (ZIP, up to 20 MiB)")}>
                  <input
                    type="file"
                    accept=".zip,application/zip"
                    onChange={(e) => void loadFile(e.target.files?.[0])}
                  />
                </Field>
              ) : (
                <>
                  <DirectoryField
                    label={t("Skill folder")}
                    value={directory}
                    onChange={(v) => {
                      fileGeneration.current++;
                      setDirectory(v);
                      setDraft(null);
                    }}
                  />
                  <button
                    disabled={!directory || ui.busy}
                    onClick={() =>
                      void ui.run(async () => {
                        const generation = ++fileGeneration.current;
                        const value = await API.InspectDirectory(directory);
                        if (generation === fileGeneration.current)
                          setDraft(value);
                      })
                    }
                  >
                    {t("Preview folder")}
                  </button>
                </>
              )}
              {!isWeb && (
                <div className="actions wrap">
                  <button
                    disabled={ui.busy}
                    onClick={() => void ui.run(() => startEditor(true))}
                  >
                    {t("Create a skill", "创建技能")}
                  </button>
                  <button
                    disabled={ui.busy}
                    onClick={() =>
                      void ui.run(async () =>
                        setSavedDrafts((await API.ListDrafts()) ?? []),
                      )
                    }
                  >
                    {t("Open saved draft", "打开已保存草稿")}
                  </button>
                  {previous.value && (
                    <button
                      disabled={ui.busy}
                      onClick={() =>
                        void ui.run(() => startEditor(false, true))
                      }
                    >
                      {t("Edit a copy of this version", "编辑此版本的副本")}
                    </button>
                  )}
                </div>
              )}
              {(draft || workspace) && (
                <button
                  disabled={ui.busy}
                  onClick={() => void ui.run(() => startEditor())}
                >
                  {t("Edit skill files", "编辑技能文件")}
                </button>
              )}
              {draft && (
                <>
                  <h3>{draft.name}</h3>
                  <p>{draft.description}</p>
                  <details>
                    <summary>
                      {`${t("Contains")} ${draft.files?.length} ${t("files")}`}
                    </summary>
                    <ul>
                      {draft.files?.map((f) => (
                        <li key={f}>
                          <code>{f}</code>
                        </li>
                      ))}
                    </ul>
                  </details>
                  <SkillDocument text={draft.markdown} />
                </>
              )}
              <div className="actions">
                <button
                  className="primary"
                  disabled={!draft || !options?.namespaces.length || ui.busy}
                  onClick={() => setStep(1)}
                >
                  {t("Continue to details")}
                </button>
              </div>
            </>
          )}
          {step === 1 && (
            <form
              className="form-grid"
              onSubmit={(e) => {
                e.preventDefault();
                if (valid) setStep(2);
              }}
            >
              <h2 className="full">
                {t("Make your skill easy to recognize", "让技能更容易被认出")}
              </h2>
              <p className="full muted">
                {t(
                  "Optional names for readers. The skill ID and installed folder stay unchanged.",
                  "为读者填写展示名称（选填），技能标识和安装目录保持不变。",
                )}
              </p>
              <Field label={t("English display name", "英文展示名称")}>
                <input
                  lang="en"
                  maxLength={80}
                  value={displayNameEN}
                  placeholder="Experiment Readout"
                  onChange={(e) => setDisplayNameEN(e.target.value)}
                />
              </Field>
              <Field label={t("Chinese display name", "中文展示名称")}>
                <input
                  lang="zh-CN"
                  maxLength={80}
                  value={displayNameZH}
                  placeholder="实验结果解读"
                  onChange={(e) => setDisplayNameZH(e.target.value)}
                />
              </Field>
              <SkillIconPicker value={icon} onChange={setIcon} />
              <ProvenanceEditor value={origin} onChange={setOrigin} />
              <p className="full skill-coordinate">
                {t("Skill ID", "技能标识")}: {namespace ? namespace + "/" : ""}
                {draft?.name}
              </p>
              <h3 className="full">
                {t("Publishing and access", "发布与访问")}
              </h3>
              <Field
                label={t("Publishing space")}
                hint={t("Only spaces you can publish to are listed.")}
              >
                <Select
                  required
                  value={namespace}
                  onChange={(e) => setNamespace(e.target.value)}
                >
                  <option value="">{t("Choose a space")}</option>
                  {options?.namespaces.map((n) => (
                    <option key={n.name} value={n.name}>
                      {n.name === participation.value?.personal_namespace
                        ? t("My skills", "我的技能")
                        : n.name}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field
                label={t("Exact version")}
                hint={
                  previous.value
                    ? `${t("Previous version", "上次版本")}: ${previous.value.release.version}. ${t("Published versions require a new version number.", "已发布版本需使用新版本号。")}`
                    : t(
                        "Use a version such as 1.0.0. Each version is a separate submission.",
                        "例如 1.0.0。每个版本都是独立提交。",
                      )
                }
              >
                <input
                  required
                  value={version}
                  pattern="(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?"
                  onChange={(e) => setVersion(e.target.value)}
                />
              </Field>
              <Field label={t("Category")}>
                <Select
                  value={category}
                  onChange={(e) => setCategory(e.target.value)}
                >
                  <option value="">{t("Uncategorized")}</option>
                  {options?.categories.map((c) => (
                    <option key={c.name} value={c.name}>
                      {c.title || c.name}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field label={t("Audience")}>
                <Select
                  value={visibility}
                  onChange={(e) => setVisibility(Number(e.target.value))}
                >
                  <option value={Visibility.Visibility_VISIBILITY_MEMBERS}>
                    {t("Signed-in hub members")}
                  </option>
                  <option value={Visibility.Visibility_VISIBILITY_PUBLIC}>
                    {t("Public (subject to hub policy)")}
                  </option>
                  {!!options?.groups.length && (
                    <option value={Visibility.Visibility_VISIBILITY_GROUPS}>
                      {t("Specific access groups")}
                    </option>
                  )}
                </Select>
              </Field>
              <p className="audience-explanation muted full">
                {visibility === Visibility.Visibility_VISIBILITY_GROUPS
                  ? t(
                      "Visible to the selected groups, namespace maintainers, reviewers and administrators, and Hub administrators. Readers must belong to a selected group.",
                      "所选分组、命名空间维护者、审核者和管理员，以及 Hub 管理员可访问。普通 Reader 必须属于所选分组。",
                    )
                  : visibility === Visibility.Visibility_VISIBILITY_PUBLIC
                    ? t(
                        "Anonymous access is possible only when this Hub allows public browsing. Your display profile accompanies the publication.",
                        "仅当 Hub 开放公开浏览时，访客才能访问；你的展示资料会随发布内容展示。",
                      )
                    : t(
                        "Visible to signed-in members of this Hub. Signing in does not grant publishing or review permissions.",
                        "此 Hub 的已登录成员可访问；登录不会自动获得发布或审核权限。",
                      )}
              </p>
              {visibility === Visibility.Visibility_VISIBILITY_GROUPS && (
                <Field label={t("Audience groups")}>
                  <Select
                    multiple
                    required
                    value={groups}
                    onChange={(e) =>
                      setGroups(
                        Array.from(e.target.selectedOptions).map(
                          (o) => o.value,
                        ),
                      )
                    }
                  >
                    {options?.groups.map((g) => (
                      <option key={g.id} value={g.id}>
                        {g.name}
                      </option>
                    ))}
                  </Select>
                </Field>
              )}
              <Field
                label={t("Tags (optional)")}
                hint={t("Separate tags with commas.")}
              >
                <input value={tags} onChange={(e) => setTags(e.target.value)} />
              </Field>
              <div className="actions full">
                <button type="button" onClick={() => setStep(0)}>
                  {t("Back")}
                </button>
                <button className="primary" disabled={!valid || ui.busy}>
                  {t("Review submission")}
                </button>
              </div>
            </form>
          )}
          {step === 2 && (
            <>
              <h2>{t("Ready to share?")}</h2>
              <ProvenanceDetails
                value={origin}
                version={version}
                digest={draft?.digest}
              />
              <div className="skill-card-heading">
                <SkillSymbol icon={icon} />
                <div>
                  <h3>{skillTitle(identity)}</h3>
                  <span className="skill-coordinate">
                    {namespace}/{draft?.name} · v{version}
                  </span>
                </div>
              </div>
              <dl className="properties">
                <dt>{t("English display name", "英文展示名称")}</dt>
                <dd>{identity.display_name_en || t("Not set", "未设置")}</dd>
                <dt>{t("Chinese display name", "中文展示名称")}</dt>
                <dd>{identity.display_name_zh || t("Not set", "未设置")}</dd>
                <dt>{t("Category")}</dt>
                <dd>{category || t("Uncategorized")}</dd>
                <dt>{t("Audience")}</dt>
                <dd>
                  {visibilityLabel(visibility)}
                  {visibility === Visibility.Visibility_VISIBILITY_PUBLIC && (
                    <p className="notice">
                      {t(
                        "Publishing makes this skill, its published history and discussion public.",
                        "发布后，此技能、已发布历史和讨论将公开。",
                      )}
                    </p>
                  )}
                </dd>
                <dt>{t("Files")}</dt>
                <dd>{draft?.files?.length}</dd>
                <dt>{t("Review policy")}</dt>
                <dd>
                  {!!origin ||
                  participation.value?.community_mode ||
                  options?.namespaces.find((n) => n.name === namespace)?.enabled
                    ? t("Independent review required")
                    : t("Maintainers can publish directly")}
                </dd>
              </dl>
              <details>
                <summary>
                  {t("Preview the exact package", "预览本次提交的内容")}
                </summary>
                <SkillDocument text={draft?.markdown ?? ""} />
                <p className="path">{draft?.digest}</p>
              </details>
              <div className="actions">
                <button disabled={ui.busy} onClick={() => setStep(1)}>
                  {t("Back")}
                </button>
                <button
                  className="primary"
                  disabled={!valid || ui.busy}
                  onClick={() => void ui.run(submit)}
                >
                  {!!origin ||
                  participation.value?.community_mode ||
                  options?.namespaces.find((n) => n.name === namespace)?.enabled
                    ? t("Submit for review")
                    : t("Publish version")}
                </button>
              </div>
            </>
          )}
        </div>
        <div className="panel publish-summary">
          <h2>{t("Your submission")}</h2>
          <div className="skill-card-heading">
            <SkillSymbol icon={icon} />
            <h3>{draft ? skillTitle(identity) : t("No skill selected")}</h3>
          </div>
          {draft && (
            <span className="skill-coordinate">
              {namespace ? namespace + "/" : ""}
              {draft.name}
            </span>
          )}
          <p>
            {draft?.description ||
              t("Choose your skill package to get started.")}
          </p>
          <dl className="properties">
            <dt>Hub</dt>
            <dd>{ui.sources.find((s) => s.alias === source)?.display_name}</dd>
            <dt>{t("Space")}</dt>
            <dd>{namespace || t("Not selected")}</dd>
            <dt>{t("Version")}</dt>
            <dd>{version || t("Not selected")}</dd>
            <dt>{t("Audience")}</dt>
            <dd>{visibilityLabel(visibility)}</dd>
          </dl>
          <p className="muted">
            {t(
              "Published versions are immutable. Changes require a new submission.",
            )}
          </p>
        </div>
      </div>
      {editing && (
        <SkillEditor
          files={workspace?.files || draft?.files || []}
          read={async (path) => {
            if (!isWeb) return API.ReadDraftFile(workspace!.id, path);
            const { inspectArchive } = await import("./web-api");
            const b = await inspectArchive(archive!);
            const data = b.files[path];
            if (!data || data.length > 1024 * 1024 || data.includes(0))
              throw new Error("File too large");
            return {
              text: new TextDecoder("utf-8", { fatal: true }).decode(data),
              digest: b.digest,
            };
          }}
          save={async (path, digest, text) => {
            if (!isWeb) {
              const result = await API.SaveDraftFile(
                workspace!.id,
                path,
                digest,
                text,
              );
              setDraft(null);
              return result;
            }
            if (
              new TextEncoder().encode(text).length > 1024 * 1024 ||
              text.includes("\0")
            )
              throw new Error("Only UTF-8 text up to 1 MiB can be edited");
            const { inspectArchive } = await import("./web-api");
            const b = await inspectArchive(archive!);
            if (b.digest !== digest)
              throw new Error("Draft changed; reopen the editor");
            b.files[path] = new TextEncoder().encode(text);
            const bytes = repackDraft(archive!, b.files);
            const next = await inspectArchive(bytes);
            setArchive(bytes);
            setDraft({
              name: next.name,
              description: next.description,
              markdown: next.markdown,
              digest: next.digest,
              files: Object.keys(next.files),
            });
            return { text, digest: next.digest };
          }}
          onClose={() => {
            setEditing(false);
            void ui.run(refreshDraft);
          }}
        />
      )}
    </div>
  );
}
