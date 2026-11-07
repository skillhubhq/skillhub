import { signInURL } from "./auth";
import { isWeb } from "./platform";
import { t } from "./i18n";
import { useUI } from "./ui";

export const needsSignIn = (error: string) =>
  /unauthenticated|authentication required|credentials? (?:invalid|expired)|login required|no saved credential|invalid source credential/i.test(
    error,
  );

export function AccessState({
  error,
  retry,
}: {
  error: string;
  retry?: () => void;
}) {
  const ui = useUI();
  const signIn = needsSignIn(error);
  const denied = /permission.denied|forbidden/i.test(error);
  return (
    <section
      className="access-state"
      role={signIn ? "region" : "alert"}
      aria-label={t("Community access", "社区访问")}
    >
      <h2>
        {signIn
          ? t("Sign in to your workspace", "登录你的工作空间")
          : denied
            ? t("You don’t have access", "你暂无访问权限")
            : t("Couldn’t load this content", "暂时无法加载内容")}
      </h2>
      <p>
        {signIn
          ? t(
              "Use the account provided by your organization to browse and share skills.",
              "使用组织提供的账号，浏览和分享团队技能。",
            )
          : denied
            ? t(
                "Ask your Hub or team administrator for access, or choose another account.",
                "请联系 Hub 或团队管理员开通权限，或选择其他账号。",
              )
            : t(
                "Check the connection and try again. If the problem continues, contact your administrator.",
                "请检查连接后重试。如果问题持续，请联系管理员。",
              )}
      </p>
      <div className="access-actions">
        {signIn &&
          (isWeb ? (
            <a className="primary" href={signInURL()}>
              {t("Sign in", "登录")}
            </a>
          ) : (
            <button className="primary" onClick={() => ui.navigate("sources")}>
              {t("Sign in", "登录")}
            </button>
          ))}
        {!isWeb && !signIn && (
          <button onClick={() => ui.navigate("sources")}>
            {t("Connections", "连接与账号")}
          </button>
        )}
        {!signIn && retry && (
          <button className={denied ? "" : "primary"} onClick={retry}>
            {t("Try again", "重试")}
          </button>
        )}
      </div>
      {signIn && (
        <small>
          {t(
            "Accounts are managed by your organization. Contact your administrator if you need access.",
            "账号由组织管理。如需访问，请联系管理员。",
          )}
        </small>
      )}
    </section>
  );
}
