// 直接链接到项目，不依赖额外请求或动态计数改变导航宽度。
export function GitHubStar({ language }: { language: "en" | "zh" }) {
  return (
    <a
      className="github-star"
      href="https://github.com/skillhubhq/skillhub"
      target="_blank"
      rel="noopener noreferrer"
      referrerPolicy="no-referrer"
      aria-label={
        language === "zh"
          ? "在 GitHub 上为 SkillHub 点 Star"
          : "Star SkillHub on GitHub"
      }
      title={language === "zh" ? "在 GitHub 上点 Star" : "Star on GitHub"}
    >
      <svg viewBox="0 0 24 24" aria-hidden="true">
        <path d="m12 3 2.8 5.7 6.2.9-4.5 4.4 1.1 6.2-5.6-3-5.6 3 1.1-6.2L3 9.6l6.2-.9Z" />
      </svg>
      <span>GitHub</span>
    </a>
  );
}
