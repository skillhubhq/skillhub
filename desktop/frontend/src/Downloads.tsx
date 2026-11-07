import type { ReactNode } from "react";
import { isWeb } from "./platform";
import { useUI } from "./ui";
import { t } from "./i18n";
import { Icon } from "./Icon";

export const projectURL = "https://github.com/skillhubhq/skillhub";
export const releasesURL = `${projectURL}/releases`;

// 外部页面始终交给浏览器；不把当前 Hub 路由、凭据或查询附加到链接。
export function ProjectLink({
  href,
  children,
  className,
}: {
  href: string;
  children: ReactNode;
  className?: string;
}) {
  const ui = useUI();
  return (
    <a
      href={href}
      className={className}
      target="_blank"
      rel="noopener noreferrer"
      referrerPolicy="no-referrer"
      onClick={(event) => {
        if (!isWeb) {
          event.preventDefault();
          void ui.run(async () => {
            const { Browser } = await import("@wailsio/runtime");
            await Browser.OpenURL(href);
          });
        }
      }}
    >
      {children}
    </a>
  );
}

export function Downloads() {
  const ui = useUI();
  const source = ui.sources.find((s) => s.alias === ui.activeSource);
  return (
    <div className="downloads-page">
      <div className="download-intro">
        <img
          className="download-mark-light"
          src="./assets/brand/mark-color.svg"
          alt=""
          width="48"
          height="48"
        />
        <img
          className="download-mark-dark"
          src="./assets/brand/mark-white.svg"
          alt=""
          width="48"
          height="48"
        />
        <div>
          <h2>
            {t(
              "Good skills. Wherever you work.",
              "让好技能，走进你的工作工具。",
            )}
          </h2>
          <p>
            {t(
              "Install skills into Codex and Claude Code with the desktop app or CLI. Connect this community and any other hubs you use.",
              "通过桌面 App 或 CLI，将技能安装到 Codex 和 Claude Code。连接当前社区，也可以添加其他 Hub。",
            )}
          </p>
        </div>
      </div>
      <div className="download-options">
        <article>
          <Icon name="device" />
          <h3>{t("Desktop app", "桌面 App")}</h3>
          <p>
            {t(
              "Browse communities, preview changes and manage installed skills without a terminal.",
              "浏览社区、预览安装变更、管理本地技能，无需使用终端。",
            )}
          </p>
          <dl>
            <dt>macOS</dt>
            <dd>
              {t("Apple silicon · macOS 13+", "Apple 芯片 · macOS 13 及以上")}
            </dd>
            <dt>Windows</dt>
            <dd>{t("x64 · WebView2 required", "x64 · 需要 WebView2")}</dd>
          </dl>
          <ProjectLink href={releasesURL} className="download-button primary">
            <Icon name="installed" />
            {t("Get the app on GitHub", "前往 GitHub 下载 App")}
          </ProjectLink>
          <small>
            {t(
              "Choose a skillhub-app archive for your platform. Linux desktop packages are not available yet.",
              "选择对应平台的 skillhub-app 安装包。暂不提供 Linux 桌面版本。",
            )}
          </small>
        </article>
        <article>
          <Icon name="terminal" />
          <h3>{t("Command line", "命令行")}</h3>
          <p>
            {t(
              "Install, update and restore project skills. The CLI shares connections and your installed library with the app.",
              "安装、更新和恢复项目技能。CLI 与 App 共用连接和本地技能库。",
            )}
          </p>
          <dl>
            <dt>{t("Platforms", "平台")}</dt>
            <dd>Linux · macOS · Windows</dd>
            <dt>{t("Package", "安装包")}</dt>
            <dd>{t("CLI + self-hosted Hub", "CLI + 自托管 Hub")}</dd>
          </dl>
          <ProjectLink href={releasesURL} className="download-button">
            <Icon name="installed" />
            {t("Get CLI releases", "下载 CLI 发行版")}
          </ProjectLink>
          <small>
            {t(
              "Choose the OS and architecture in the filename. Read the release notes before upgrading.",
              "按文件名选择操作系统与架构，升级前请阅读版本说明。",
            )}
          </small>
        </article>
      </div>
      <div className="download-followup">
        <section>
          <h3>{t("Then connect your community", "下载后，连接你的社区")}</h3>
          <p>
            {t(
              "In the app, open Settings → Connections, add the Hub address and sign in. Each Hub keeps its own account and permissions.",
              "在 App 中打开「设置 → 连接与账号」，添加 Hub 地址并登录。每个 Hub 的账号和权限相互独立。",
            )}
          </p>
          {source?.url && <code className="download-origin">{source.url}</code>}
          <ProjectLink href={`${projectURL}/blob/main/docs/usage.md`}>
            {t("Installation and CLI guide", "安装与 CLI 使用指南")}
          </ProjectLink>
        </section>
        <section>
          <h3>{t("Know what you download", "了解你下载的版本")}</h3>
          <p>
            {t(
              "Preview desktop builds are unsigned and do not update automatically. Verify SHA256SUMS and review the platform requirements on the release page.",
              "预览版桌面安装包尚未签名，也不会自动更新。请校验 SHA256SUMS，并查看发行页的平台要求。",
            )}
          </p>
          <ProjectLink href={`${projectURL}/blob/main/docs/distribution.md`}>
            {t("Verification and release policy", "校验方法与发布说明")}
          </ProjectLink>
        </section>
      </div>
      <section className="project-community">
        <div>
          <h3>
            {t("Built in the open. Hosted by you.", "代码开放，数据由你掌握。")}
          </h3>
          <p>
            {t(
              "Apache-2.0. GitHub hosts the project and downloads; your Hub owns its skills, people and permissions.",
              "采用 Apache-2.0。GitHub 托管项目和安装包；技能、成员和权限保存在各自的 Hub。",
            )}
          </p>
        </div>
        <div className="project-actions">
          <ProjectLink href={projectURL}>
            <Icon name="star" />
            {t("Star on GitHub", "在 GitHub 点 Star")}
          </ProjectLink>
          <ProjectLink href={`${projectURL}/issues`}>
            {t("Report an issue", "反馈问题")}
          </ProjectLink>
        </div>
      </section>
      <p className="download-access-note">
        {t(
          "GitHub links open in your browser. Private repositories require GitHub access; no Hub credentials or skill content are sent with these links.",
          "GitHub 链接会在浏览器中打开。私有仓库需要 GitHub 访问权限；这些链接不会携带 Hub 凭据或技能内容。",
        )}
      </p>
    </div>
  );
}

export function ProjectFooter() {
  const ui = useUI();
  if (!isWeb) return null;
  return (
    <footer className="project-footer">
      <span>SkillHub</span>
      <nav aria-label={t("Project links", "项目链接")}>
        <button onClick={() => ui.navigate("downloads")}>
          {t("Downloads", "下载")}
        </button>
        <ProjectLink href={projectURL}>GitHub</ProjectLink>
        <ProjectLink href={`${projectURL}/blob/main/LICENSE`}>
          Apache-2.0
        </ProjectLink>
      </nav>
    </footer>
  );
}
