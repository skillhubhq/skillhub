import toolAdapters from "../../../internal/client/tools.json";
import { skillAppURL } from "./official";
import { ReportButton } from "./Moderation";
import { Select } from "./Select";
import { ProjectLink, releasesURL } from "./Downloads";
import { signInURL } from "./auth";
import { useEffect, useState } from "react";
import { API, Field, ToolSelect, useUI } from "./ui";
import { Icon } from "./Icon";
import { t } from "./i18n";
import { isWeb } from "./platform";
import { useMutationKey, type Discussion } from "./community-ui";

export function SkillActions({
  source,
  id,
  summary,
  changed,
  comment,
}: {
  source: string;
  id: string;
  summary: Discussion | null;
  changed: (value: Discussion) => void;
  comment: () => void;
}) {
  const ui = useUI(),
    key = useMutationKey();
  const [epoch, setEpoch] = useState(0);
  async function react(bookmark: boolean) {
    if (!summary) return;
    if (!summary.can_interact) {
      if (isWeb) location.href = signInURL();
      else ui.navigate("sources");
      return;
    }
    const active = !(bookmark ? summary.bookmarked : summary.liked);
    await ui.run(async () => {
      changed(
        await API.CommunitySetReaction(
          source,
          id,
          bookmark,
          active,
          await key([source, id, bookmark, active, epoch]),
        ),
      );
      setEpoch((v) => v + 1);
    });
  }
  return (
    <div className="skill-social-actions">
      <button
        aria-pressed={summary?.liked ?? false}
        disabled={!summary || ui.busy}
        onClick={() => void react(false)}
      >
        <Icon name="heart" />
        {summary?.liked ? t("Liked", "已点赞") : t("Like", "点赞")}
        <span>{summary?.like_count ?? "—"}</span>
      </button>
      <button
        aria-pressed={summary?.bookmarked ?? false}
        disabled={!summary || ui.busy}
        onClick={() => void react(true)}
        title={t(
          "Save this skill privately in this hub",
          "在此 Hub 私密收藏此技能",
        )}
      >
        <Icon name="bookmark" />
        {summary?.bookmarked ? t("Saved", "已收藏") : t("Save", "收藏")}
      </button>
      <button onClick={comment} disabled={!summary}>
        <Icon name="message" />
        {t("Comment", "评论")}
        <span>{summary?.comment_count ?? "—"}</span>
      </button>
      {summary?.can_interact && <ReportButton source={source} release={id} />}
    </div>
  );
}

export function CopyValue({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false),
    [failed, setFailed] = useState(false);
  useEffect(() => {
    setCopied(false);
    setFailed(false);
  }, [value]);
  return (
    <div className="copy-value">
      <div>
        <span>{label}</span>
        <button
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(value);
              setCopied(true);
              setFailed(false);
            } catch {
              setFailed(true);
            }
          }}
        >
          {copied ? t("Copied", "已复制") : t("Copy", "复制")}
        </button>
      </div>
      <pre tabIndex={0}>{value}</pre>
      <span role="status" className="muted">
        {failed
          ? t(
              "Select the text above and copy it manually.",
              "请选择上方文本手动复制。",
            )
          : copied
            ? t("Copied to clipboard.", "已复制到剪贴板。")
            : ""}
      </span>
    </div>
  );
}

export function WebInstallGuide({
  suite = false,
  url,
  coordinate,
}: {
  url: string;
  suite?: boolean;
  coordinate: string;
}) {
  const ui = useUI();
  const [downloaded, setDownloaded] = useState(false);
  const [method, setMethod] = useState("app"),
    [tool, setTool] = useState("codex"),
    [scope, setScope] = useState("project"),
    [alias, setAlias] = useState("my-hub");
  const quote = (value: string) => "'" + value.replace(/'/g, "'\\''") + "'";
  const validAlias = /^[a-z][a-z0-9-]{0,62}$/.test(alias);
  return (
    <div className="install-guide">
      <div className="content-tabs">
        <button
          disabled={suite}
          title={
            suite
              ? t(
                  "Install suite members with the App or CLI",
                  "使用 App 或 CLI 安装套装成员",
                )
              : undefined
          }
          aria-pressed={method === "zip"}
          onClick={() => setMethod("zip")}
        >
          {t("Download ZIP", "下载 ZIP")}
        </button>
        <button
          aria-pressed={method === "app"}
          onClick={() => setMethod("app")}
        >
          {t("Desktop app", "桌面 App")}
        </button>
        <button
          aria-pressed={method === "cli"}
          onClick={() => setMethod("cli")}
        >
          {t("Command line", "命令行")}
        </button>
      </div>
      {method === "zip" ? (
        <div className="zip-install">
          <p>
            {t(
              "Download this exact version, then extract it into your tool’s skills folder.",
              "下载当前精确版本，然后解压到工具的 skills 文件夹。",
            )}
          </p>
          <Field label={t("Tool")}>
            <ToolSelect value={tool} onChange={setTool} />
          </Field>
          <CopyValue
            label={t("Folder in your project", "项目内的目标文件夹")}
            value={`${toolAdapters.find((item) => item.id === tool)?.project_directory}/${coordinate.split("/").pop()?.split("@")[0]}/`}
          />
          <p className="muted">
            {t(
              "Place SKILL.md directly inside this folder, together with all bundled files. Manual downloads are not tracked for updates by SkillHub.",
              "将 SKILL.md 和随包文件直接放入该文件夹。手动下载不会被 SkillHub 自动记录或检查更新。",
            )}
          </p>
          <button
            className="primary"
            disabled={ui.busy}
            onClick={() =>
              void ui.run(async () => {
                const { downloadSkillZIP } = await import("./web-api");
                await downloadSkillZIP(coordinate);
                setDownloaded(true);
              })
            }
          >
            {t("Download skill ZIP", "下载技能 ZIP")}
          </button>
          {downloaded && (
            <p role="status">
              {t(
                "Download started. Check your browser’s downloads.",
                "已开始下载，请查看浏览器下载列表。",
              )}
            </p>
          )}
        </div>
      ) : method === "app" ? (
        <div className="stack app-install-guide">
          <h3>
            {t("Open this version in SkillHub", "在 SkillHub 中打开当前版本")}
          </h3>
          <p>
            {t(
              "Choose your tool and a project or global destination in the App. Review the file changes before installing.",
              "在 App 内选择目标工具以及项目或全局位置。确认文件变更后再安装。",
            )}
          </p>
          <a
            className="primary app-open-link"
            href={skillAppURL(url, coordinate)}
          >
            {t("Open in SkillHub", "在 SkillHub 中打开")} ↗
          </a>
          <p className="muted">
            {t(
              "If nothing opens, install the current App first, then return here. Your browser may ask permission to open it.",
              "如果没有打开，请先安装最新 App 后返回。浏览器可能会询问是否允许打开应用。",
            )}
          </p>
          <ProjectLink href={releasesURL}>
            {t("Download the desktop app", "下载桌面 App")}
          </ProjectLink>
          <details>
            <summary>{t("Connect manually", "手动连接")}</summary>
            <CopyValue label={t("Hub address", "Hub 地址")} value={url} />
            <CopyValue
              label={t("Exact version", "精确版本")}
              value={coordinate}
            />
          </details>
        </div>
      ) : (
        <>
          <p>
            {t(
              "With the SkillHub CLI installed, use an existing source alias or add this Hub once.",
              "安装 SkillHub CLI 后，可填写已有来源别名，或先添加此 Hub。",
            )}
          </p>
          <Field label={t("Source alias", "来源别名")}>
            <input
              value={alias}
              onChange={(e) => setAlias(e.target.value)}
              maxLength={63}
            />
          </Field>
          {validAlias ? (
            <details className="cli-setup">
              <summary>
                {t("First time connecting this Hub?", "首次连接此 Hub？")}
              </summary>
              <CopyValue
                label={t("Add source", "添加来源")}
                value={`skillhub source add ${alias} ${quote(url)}`}
              />
              <CopyValue
                label={t(
                  "Sign in if access requires it",
                  "如需身份权限，请登录",
                )}
                value={`skillhub login ${alias}`}
              />
            </details>
          ) : (
            <p role="alert">
              {t(
                "Use a lowercase letter first, then letters, numbers or hyphens.",
                "以小写字母开头，仅使用小写字母、数字或连字符。",
              )}
            </p>
          )}
          <div className="form-grid">
            <Field label={t("Tool")}>
              <ToolSelect value={tool} onChange={setTool} />
            </Field>
            <Field label={t("Install for", "安装范围")}>
              <Select value={scope} onChange={(e) => setScope(e.target.value)}>
                <option value="project">{t("This project", "当前项目")}</option>
                <option value="global">
                  {t("All your projects", "你的所有项目")}
                </option>
              </Select>
            </Field>
          </div>
          {validAlias && (
            <CopyValue
              label={
                scope === "project"
                  ? t("Run in your project folder", "在项目目录中执行")
                  : t("Run in your terminal", "在终端中执行")
              }
              value={`skillhub install ${quote(`${alias}/${coordinate}`)} --tool ${tool} --scope ${scope}`}
            />
          )}
        </>
      )}
    </div>
  );
}
