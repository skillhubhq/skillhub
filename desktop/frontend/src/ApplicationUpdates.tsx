import { useEffect, useState } from "react";
import { API, Modal } from "./ui";
import { t } from "./i18n";
import { ProjectLink, releasesURL } from "./Downloads";

type Info = Awaited<ReturnType<typeof API.ApplicationInfo>>;
type Result = Awaited<ReturnType<typeof API.CheckApplicationUpdate>>;

export function ApplicationUpdates() {
  const [info, setInfo] = useState<Info | null>(null);
  const [result, setResult] = useState<Result | null>(null);
  const [preview, setPreview] = useState(false);
  const [checking, setChecking] = useState(false);
  const [details, setDetails] = useState(false);
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    void API.ApplicationInfo()
      .then(setInfo)
      .catch(() => setFailed(true));
  }, []);
  const check = async () => {
    setChecking(true);
    setFailed(false);
    setResult(null);
    try {
      setResult(await API.CheckApplicationUpdate(preview));
    } catch {
      setFailed(true);
    } finally {
      setChecking(false);
    }
  };
  useEffect(() => {
    const requested = () => {
      if (sessionStorage.getItem("skillhub-check-update")) {
        sessionStorage.removeItem("skillhub-check-update");
        void check();
      }
    };
    requested();
    window.addEventListener("skillhub-check-update", requested);
    return () => window.removeEventListener("skillhub-check-update", requested);
  }, []);
  const messages: Record<string, string> = {
    available: t("An update is ready to download.", "发现可下载的新版本。"),
    up_to_date: t(
      "No newer version in this channel.",
      "当前通道没有更新的版本。",
    ),
    no_release: t(
      "No releases in this channel yet. You can include preview releases.",
      "当前通道尚无发布版本，可以选择包含预览版本。",
    ),
    no_package: t(
      "A newer version exists, but its package for this device is not available yet.",
      "已有新版本，但尚未提供适配这台设备的安装包。",
    ),
    restricted: t(
      "Releases are not publicly accessible yet. Open GitHub and sign in if you have repository access.",
      "发布信息暂未公开。如有仓库权限，请前往 GitHub 登录查看。",
    ),
    offline: t(
      "Could not reach GitHub. Check your connection and try again.",
      "无法连接 GitHub，请检查网络后重试。",
    ),
    rate_limited: t(
      "GitHub temporarily limited this check. Try again later or open Releases.",
      "GitHub 暂时限制了检查请求，请稍后重试或打开发布页面。",
    ),
    unsupported: t(
      "No desktop package is currently offered for this platform.",
      "目前未提供此平台的桌面安装包。",
    ),
    development: t(
      "This development build has no release version. Browse releases to choose a package.",
      "此开发构建没有发布版本号，请前往发布页选择安装包。",
    ),
    unavailable: t(
      "Could not verify release information. Try again later or open Releases.",
      "暂时无法确认发布信息，请稍后重试或打开发布页面。",
    ),
  };
  return (
    <section
      className="preference-section app-updates"
      aria-label={t("App updates", "App 更新")}
    >
      <div className="app-update-heading">
        <div>
          <h2>{t("App updates", "App 更新")}</h2>
          <p>
            {info
              ? `SkillHub ${info.version} · ${info.platform === "darwin" ? "macOS" : info.platform === "windows" ? "Windows" : info.platform} ${info.architecture === "arm64" ? "Apple silicon" : "x64"}`
              : t("Loading version…", "正在读取版本…")}
          </p>
          {info && (
            <small>
              {info.build === "local" || info.build === "development"
                ? t("Local preview", "本地预览构建")
                : t("Source build", "源码构建")}{" "}
              · {info.revision.slice(0, 12)}
            </small>
          )}
        </div>
        <button
          onClick={() => void check()}
          disabled={checking || !info}
          data-testid="check-app-update"
        >
          {checking
            ? t("Checking…", "正在检查…")
            : t("Check for updates", "检查更新")}
        </button>
      </div>
      <label className="app-update-preview">
        <input
          type="checkbox"
          checked={preview}
          disabled={checking}
          onChange={(e) => {
            setPreview(e.target.checked);
            setResult(null);
          }}
        />
        {t("Include preview releases", "包含预览版本")}
      </label>
      <p className="app-update-hint">
        {t(
          "Checks GitHub only when you ask. App updates do not change your hubs or installed skills.",
          "仅在你主动检查时访问 GitHub。App 更新不会更新 Hub 或已安装的技能。",
        )}
      </p>
      <div aria-live="polite" role="status">
        {failed && (
          <p>
            {t(
              "Could not check this build. Try again.",
              "无法检查当前构建，请重试。",
            )}
          </p>
        )}
        {result && (
          <div className="app-update-result">
            <p>
              {messages[result.status]}{" "}
              {result.status === "available" && (
                <strong>v{result.version}</strong>
              )}
            </p>
            <small>
              {t("Checked", "检查时间")}{" "}
              {new Date(result.checked_at).toLocaleString()}
            </small>
            <div className="actions">
              {result.status === "available" && (
                <button className="primary" onClick={() => setDetails(true)}>
                  {t("View update", "查看更新")}
                </button>
              )}
              <ProjectLink href={result.url || releasesURL}>
                {t("Open GitHub Releases", "打开 GitHub 发布页")}
              </ProjectLink>
            </div>
          </div>
        )}
      </div>
      {details && result && (
        <Modal
          title={`${t("Update available", "发现新版本")} · v${result.version}`}
          onClose={() => setDetails(false)}
        >
          <div className="stack">
            <p>
              {info?.platform === "darwin"
                ? t(
                    "Download the package, quit SkillHub, then replace the app in Applications. Your connections and local library stay in their existing location.",
                    "下载安装包后，退出 SkillHub，再替换 Applications 中的 App。连接配置和本地技能库保留在原位置。",
                  )
                : t(
                    "Download the package, quit SkillHub and replace the existing application. Keep your client data folder and follow the release instructions.",
                    "下载安装包后退出 SkillHub，再按发布说明替换原应用。请保留现有客户端数据目录。",
                  )}
            </p>
            <p>
              <strong>{result.asset}</strong>
            </p>
            <div className="app-update-notes">
              {result.notes ||
                t("No release notes were provided.", "此版本未提供更新说明。")}
            </div>
            <p>
              {t(
                "Preview packages may show an unidentified-developer warning. Review the release notes and checksum before installing. Installation remains manual.",
                "预览安装包可能显示开发者未识别提示。安装前请阅读发布说明并核对校验和，此流程不会自动安装。",
              )}
            </p>
            <div className="actions">
              <button onClick={() => setDetails(false)}>
                {t("Later", "稍后")}
              </button>
              <ProjectLink
                href={result.url}
                className="download-button primary"
              >
                {t("Download on GitHub", "前往 GitHub 下载")}
              </ProjectLink>
            </div>
          </div>
        </Modal>
      )}
    </section>
  );
}
