import { useState } from "react";
import { API, Field, Modal, useUI } from "./ui";
import { useResource, ResourceState } from "./community-ui";
import { Select } from "./Select";
import { t } from "./i18n";
import "./distribution.css";
import {
  Visibility,
  CatalogOrder,
} from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";
import type { Member } from "../bindings/github.com/skillhubhq/skillhub/internal/skillsuite/models";

export function SuiteComposer({ source }: { source: string }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button onClick={() => setOpen(true)}>
        {t("Create a skill suite", "创建技能套装")}
      </button>
      {open && <Composer source={source} close={() => setOpen(false)} />}
    </>
  );
}
function Composer({ source, close }: { source: string; close: () => void }) {
  const ui = useUI();
  const [members, setMembers] = useState<Member[]>([]),
    [entry, setEntry] = useState(""),
    [namespace, setNamespace] = useState(""),
    [name, setName] = useState(""),
    [version, setVersion] = useState("1.0.0"),
    [description, setDescription] = useState(""),
    [query, setQuery] = useState(""),
    [search, setSearch] = useState(""),
    [visibility, setVisibility] = useState(
      Visibility.Visibility_VISIBILITY_MEMBERS,
    ),
    [selected, setSelected] = useState(""),
    [release, setRelease] = useState("");
  const spaces = useResource(async () => {
    let token = "";
    const rows = [];
    do {
      const p = await API.AdminList(source, "submission-namespaces", "", token);
      rows.push(...(p.rows ?? []));
      token = p.next_page;
    } while (token);
    return rows;
  }, [source]);
  const found = useResource(
    () =>
      search
        ? API.Search(
            source,
            search,
            "",
            [],
            "",
            CatalogOrder.CatalogOrder_CATALOG_ORDER_NAME,
            false,
          )
        : Promise.resolve(null),
    [source, search],
  );
  const versions = useResource(async () => {
    if (!selected) return null;
    let p = await API.Releases(`${source}/${selected}`, "");
    const releases = [...(p.releases ?? [])];
    while (p.next_page) {
      p = await API.Releases(`${source}/${selected}`, p.next_page);
      releases.push(...(p.releases ?? []));
    }
    return { ...p, releases };
  }, [source, selected]);
  return (
    <Modal
      title={t("Create a skill suite", "创建技能套装")}
      onClose={close}
      stable
    >
      <form
        className="stack"
        onSubmit={(e) => {
          e.preventDefault();
          void ui.run(async () => {
            const v = await API.PublishSuite({
              source,
              namespace,
              name,
              version,
              description,
              visibility,
              members,
              entry,
              request_id: await API.NewRequestID(),
            });
            close();
            ui.navigate(
              `submissions?${new URLSearchParams({ source, release: v.id })}`,
            );
          });
        }}
      >
        <p>
          {t(
            "Bundle exact skill versions from this Hub. The suite follows the team's publication review.",
            "组合此 Hub 的精确技能版本，套装遵循团队的发布审核流程。",
          )}
        </p>
        <div className="form-grid">
          <Field label={t("Team", "团队")}>
            <Select
              required
              value={namespace}
              onChange={(e) => setNamespace(e.target.value)}
            >
              <option value="">{t("Choose a team", "选择团队")}</option>
              {spaces.value?.map((v) => (
                <option key={v.id} value={v.id}>
                  {v.name || v.id}
                </option>
              ))}
            </Select>
          </Field>
          <Field label={t("Name", "名称")}>
            <input
              required
              pattern="[a-z0-9]+(-[a-z0-9]+)*"
              maxLength={64}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field label={t("Version", "版本")}>
            <input
              required
              value={version}
              onChange={(e) => setVersion(e.target.value)}
            />
          </Field>
          <Field label={t("Visibility", "可见范围")}>
            <Select
              value={visibility}
              onChange={(e) => setVisibility(Number(e.target.value))}
            >
              <option value={Visibility.Visibility_VISIBILITY_MEMBERS}>
                {t("Hub members", "Hub 成员")}
              </option>
              <option value={Visibility.Visibility_VISIBILITY_PUBLIC}>
                {t("Public", "公开")}
              </option>
            </Select>
          </Field>
        </div>
        <Field label={t("Description", "描述")}>
          <textarea
            required
            maxLength={1000}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </Field>
        <div className="distribution-input-row">
          <input
            aria-label={t("Find a skill", "查找技能")}
            placeholder={t("Search this Hub…", "搜索当前 Hub…")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                setSearch(query.trim());
              }
            }}
          />
          <button
            type="button"
            disabled={!query.trim()}
            onClick={() => setSearch(query.trim())}
          >
            {t("Search", "搜索")}
          </button>
        </div>
        <ResourceState {...found} retry={found.reload}>
          {found.value && !found.value.some((page) => page.skills?.length) ? (
            <p role="status">
              {t("No skills match your search.", "没有匹配的技能。")}
            </p>
          ) : (
            found.value && (
              <Select
                aria-label={t("Skill", "技能")}
                value={selected}
                onChange={(e) => {
                  setSelected(e.target.value);
                  setRelease("");
                }}
              >
                <option value="">{t("Choose a skill", "选择技能")}</option>
                {found.value
                  .flatMap((p) => p.skills ?? [])
                  .map((s) => (
                    <option
                      key={s.namespace + "/" + s.name}
                      value={s.namespace + "/" + s.name}
                    >
                      {s.namespace}/{s.name}
                    </option>
                  ))}
              </Select>
            )
          )}
        </ResourceState>
        {selected && (
          <ResourceState {...versions} retry={versions.reload}>
            <div className="distribution-input-row">
              <Select
                aria-label={t("Version", "版本")}
                value={release}
                onChange={(e) => setRelease(e.target.value)}
              >
                <option value="">{t("Choose a version", "选择版本")}</option>
                {versions.value?.releases?.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.version}
                  </option>
                ))}
              </Select>
              <button
                type="button"
                disabled={
                  !release ||
                  members.length >= 50 ||
                  members.some((m) => m.name === selected.split("/")[1])
                }
                onClick={() => {
                  const r = versions.value?.releases?.find(
                    (r) => r.id === release,
                  );
                  if (!r) return;
                  const [ns, n] = selected.split("/");
                  setMembers([
                    ...members,
                    {
                      namespace: ns,
                      name: n,
                      version: r.version,
                      digest: r.digest,
                    },
                  ]);
                  if (!entry) setEntry(selected);
                  setSelected("");
                  setRelease("");
                }}
              >
                {t("Add", "添加")}
              </button>
            </div>
          </ResourceState>
        )}
        <ul className="distribution-list">
          {members.map((m) => (
            <li key={m.name}>
              <span>
                {m.namespace}/{m.name} · {m.version}
              </span>
              <button
                type="button"
                onClick={() => {
                  setMembers(members.filter((v) => v.name !== m.name));
                  if (entry === m.namespace + "/" + m.name) setEntry("");
                }}
              >
                {t("Remove", "移除")}
              </button>
            </li>
          ))}
        </ul>
        <Field label={t("Start with", "入口技能")}>
          <Select
            required
            disabled={!members.length}
            value={entry}
            onChange={(e) => setEntry(e.target.value)}
          >
            <option value="">
              {t("Choose the entry skill", "选择入口技能")}
            </option>
            {members.map((m) => (
              <option key={m.name} value={m.namespace + "/" + m.name}>
                {m.namespace}/{m.name}
              </option>
            ))}
          </Select>
        </Field>
        <button
          className="primary"
          disabled={ui.busy || !members.length || !entry}
        >
          {t("Submit suite", "提交套装")}
        </button>
      </form>
    </Modal>
  );
}
