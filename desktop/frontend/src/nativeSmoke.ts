import { runModalSmoke } from "./modalSmoke";
import { runEvaluationSmoke } from "./evaluationSmoke";
import { runUpgradeSmoke } from "./nativeUpgrade";
import { runAccessSmoke } from "./accessSmoke";
import { runSelectSmoke } from "./selectSmoke";
import { skillTitle } from "./SkillIdentity";
import { setLanguage, type Language } from "./i18n";
import { setTheme, type Theme } from "./preferences";
import {
  ReportResolution as Resolution,
  NamespaceRole as Role,
  PlatformRole as Duty,
  PrincipalKind as Principal,
  ReleaseStatus as Status,
} from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";
import { Call } from "@wailsio/runtime";
import { API } from "./ui";

// 运行在测试构建的真实系统 WebView 内，通过生产绑定验证原生桥接与产品流程。
export async function runNativeSmoke() {
  if (await runUpgradeSmoke()) return;
  const checks: string[] = [];
  const started = performance.now();
  let error = "";
  const check = (condition: unknown, label: string) => {
    if (!condition) throw new Error(label);
    checks.push(label);
  };
  try {
    const deadline = Date.now() + 15000;
    while (!document.querySelector("nav button[aria-current=page]")) {
      if (Date.now() > deadline)
        throw new Error("native React window did not render");
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
    check(
      document.querySelector("main h1")?.textContent?.trim(),
      "native React shell rendered",
    );
    check(
      document.documentElement.scrollWidth <= window.innerWidth + 1,
      "native shell fits window width",
    );
    check(
      document.querySelectorAll(".primary-navigation button").length === 3 &&
        !!document.querySelector(".quick-language .ui-select") &&
        !!document.querySelector(".quick-theme .ui-select"),
      "task navigation and persistent preference shortcuts rendered",
    );
    await runSelectSmoke(check);
    await runModalSmoke(check);
    await runAccessSmoke(check);
    // 模拟无类型调用方：偏好存储只接受枚举字面量，不持久化任意输入。
    setLanguage("fixture-untrusted-value" as Language);
    check(
      localStorage.getItem("skillhub.language") === "en" &&
        document.documentElement.lang === "en",
      "language preference persists only an allowed value",
    );
    setTheme("fixture-untrusted-value" as Theme);
    check(
      localStorage.getItem("skillhub.theme") === "system",
      "theme preference persists only an allowed value",
    );
    for (const mode of ["light", "dark", "system"])
      await API.ApplyAppearance(mode, mode === "dark");
    let invalidAppearanceRejected = false;
    try {
      await API.ApplyAppearance("unknown", false);
    } catch {
      invalidAppearanceRejected = true;
    }
    check(
      invalidAppearanceRejected,
      "native appearance modes applied and invalid mode rejected",
    );
    check(
      skillTitle({ name: "stable-id", display_name_zh: "仅中文" }) ===
        "仅中文" && skillTitle({ name: "stable-id" }) === "stable-id",
      "missing translation falls back to authored title then stable name",
    );
    const sources = (await API.ListSources()) ?? [];
    check(
      sources.length === 2 &&
        sources.some((s) => s.alias === "company") &&
        sources.some((s) => s.alias === "public"),
      "isolated sources loaded through Wails",
    );
    const identity = await API.Identity("company");
    check(
      identity.username === "admin" && identity.administrator,
      "independent Hub identity verified",
    );
    const pages = (await API.Search("", "", "", [], "", 0, false)) ?? [];
    check(
      pages.length === 2 &&
        pages.every((p) => !p.error && p.skills?.length === 1),
      "two-source catalog discovered",
    );
    const waitForUI = async (predicate: () => boolean) => {
      const until = Date.now() + 15000;
      while (!predicate()) {
        if (Date.now() > until)
          throw new Error(
            `UI did not settle: ${predicate.toString().slice(0, 240)}`,
          );
        await new Promise((resolve) => setTimeout(resolve, 50));
      }
    };
    const webSkill =
      pages.find((page) => page.source === "company")?.skills?.[0] ??
      pages[0].skills![0];
    const webSource = sources.find((source) => source.alias === "company")!;
    const webLink = `skillhub://open?${new URLSearchParams({ hub: webSource.url, skill: `${webSkill.namespace}/${webSkill.name}`, version: webSkill.version })}`;
    const installedBeforeLink = JSON.stringify(await API.ListInstalled());
    await Call.ByName("main.NativeSmokeReporter.OpenLink", webLink);
    await waitForUI(() => !!document.querySelector("dialog[open]"));
    check(
      document
        .querySelector("dialog[open]")
        ?.textContent?.includes(webSource.url),
      "web link requires explicit source confirmation",
    );
    const linkButton = (label: string) =>
      Array.from(
        document.querySelectorAll<HTMLButtonElement>("dialog[open] button"),
      ).find((button) => button.textContent === label)!;
    linkButton("Cancel").click();
    await waitForUI(() => !document.querySelector("dialog[open]"));
    check(
      JSON.stringify(await API.ListInstalled()) === installedBeforeLink,
      "cancelled web link preserves local installations",
    );
    await Call.ByName("main.NativeSmokeReporter.OpenLink", webLink);
    await waitForUI(() => !!document.querySelector("dialog[open]"));
    linkButton("View skill").click();
    await waitForUI(
      () =>
        !document.querySelector("dialog[open]") &&
        !!document.querySelector(".skill-detail .detail-hero"),
    );
    check(
      location.hash.includes(`version=${webSkill.version}`) &&
        location.hash.includes("source=company"),
      "confirmed web link opens its exact version and existing connection",
    );
    check(
      JSON.stringify(await API.ListInstalled()) === installedBeforeLink,
      "opening web detail never installs files",
    );
    check(
      document
        .querySelector<HTMLAnchorElement>(".source-web-link")
        ?.href.startsWith(webSource.url + "/#discover?"),
      "native detail links back to its source website",
    );
    location.hash = "discover?source=company";
    await waitForUI(() => !!document.querySelector(".search-scope"));
    const appearance = document.querySelector<HTMLButtonElement>(
      ".quick-theme .ui-select",
    )!;
    appearance.click();
    await waitForUI(() => !!document.querySelector(".ui-select-menu"));
    const themeMenu = document.querySelector<HTMLElement>(".ui-select-menu")!;
    const themeBounds = themeMenu.getBoundingClientRect();
    check(
      themeMenu.contains(
        document.elementFromPoint(themeBounds.right - 20, themeBounds.top + 20),
      ),
      "appearance menu paints above the Hub switcher",
    );
    check(
      themeMenu.querySelectorAll("[role=option]").length === 3,
      "appearance uses the shared accessible select",
    );
    appearance.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
    );
    await waitForUI(() => !document.querySelector(".ui-select-menu"));
    const clickManagementTab = async (label: string) => {
      // 权限入口在身份返回后才出现；列表也可能先于全局请求状态更新。
      const findButton = () =>
        Array.from(
          document.querySelectorAll<HTMLButtonElement>(".tabs button"),
        ).find((button) => button.textContent === label && !button.disabled);
      await waitForUI(() => !!findButton());
      findButton()!.click();
    };
    const chooseSelect = async (selector: string, value: string) => {
      await waitForUI(() => {
        const trigger = document.querySelector<HTMLButtonElement>(selector);
        return !!trigger && !trigger.matches(":disabled");
      });
      document.querySelector<HTMLButtonElement>(selector)!.click();
      await waitForUI(() => !!document.querySelector(".ui-select-menu"));
      const option = Array.from(
        document.querySelectorAll<HTMLElement>(".ui-select-menu [role=option]"),
      ).find((item) => item.dataset.value === value);
      if (!option) throw new Error(`Missing choice in ${selector}`);
      option.click();
    };
    await waitForUI(() => !!document.querySelector(".search-scope"));
    await chooseSelect(".search-scope", "all");
    await waitForUI(
      () =>
        document.querySelectorAll(".source-results .skill-card-link").length ===
        2,
    );
    check(
      location.hash.includes("scope=all"),
      "aggregate discovery uses explicit search scope",
    );
    check(
      document.querySelectorAll(".topic-buttons button").length === 1,
      "aggregate discovery does not mix independent category taxonomies",
    );
    check(
      document
        .querySelector<HTMLAnchorElement>(".source-results .skill-card-link")
        ?.getAttribute("href")
        ?.includes("skill="),
      "skill title exposes a real detail link",
    );
    document
      .querySelector<HTMLElement>(".source-results .catalog-item > p")!
      .click();
    await waitForUI(
      () => !!document.querySelector(".skill-detail .detail-hero"),
    );
    check(
      location.hash.includes("source=company") &&
        location.hash.includes("scope=all"),
      "card description opens aggregate result in its concrete source",
    );
    await waitForUI(
      () =>
        !!document.querySelector<HTMLButtonElement>(
          ".skill-social-actions button",
        ) &&
        !document.querySelector<HTMLButtonElement>(
          ".skill-social-actions button",
        )!.disabled,
    );
    check(
      document.querySelectorAll(".skill-social-actions button").length === 4,
      "like save comment and report are visible beside installation",
    );
    Array.from(
      document.querySelectorAll<HTMLButtonElement>(
        ".skill-social-actions button",
      ),
    )
      .find((b) => b.textContent === "Report")!
      .click();
    await waitForUI(() => !!document.querySelector("dialog[open] textarea"));
    check(
      !document
        .querySelector<HTMLDialogElement>("dialog[open]")!
        .textContent?.includes("does not support"),
      "native report dialog supports the connected Hub",
    );
    check(
      document.activeElement?.tagName === "TEXTAREA",
      "report reason receives keyboard focus",
    );
    const socialAction = document.querySelector<HTMLButtonElement>(
      ".skill-social-actions > button",
    )!;
    check(
      ["inline-flex", "flex"].includes(
        getComputedStyle(socialAction).display,
      ) && parseFloat(getComputedStyle(socialAction).columnGap) >= 5,
      "skill social actions retain readable icon and count spacing",
    );
    const reportModal =
      document.querySelector<HTMLDialogElement>("dialog[open]")!;
    const submitStyle = getComputedStyle(
      reportModal.querySelector<HTMLButtonElement>("button.primary")!,
    );
    check(
      submitStyle.backgroundColor !== "rgba(0, 0, 0, 0)" &&
        submitStyle.backgroundColor !== submitStyle.color,
      "report submit button keeps its primary contrast inside detail actions",
    );
    check(
      reportModal.getBoundingClientRect().height <= 441,
      "report form uses a compact stable dialog",
    );
    document
      .querySelector<HTMLButtonElement>(
        "dialog[open] button[aria-label='Close dialog']",
      )!
      .click();
    await waitForUI(() => !document.querySelector("dialog[open]"));
    document
      .querySelector<HTMLButtonElement>(".skill-social-actions button")!
      .click();
    await waitForUI(
      () =>
        document
          .querySelector(".skill-social-actions button")
          ?.getAttribute("aria-pressed") === "true",
    );
    check(
      (
        await API.CommunityGetDiscussion(
          "company",
          pages.find((p) => p.source === "company")
            ? (await API.Preview("company/platform/release-check@1.0.0"))
                .release.id
            : "",
        )
      ).liked,
      "detail like action persists to the Hub",
    );
    document
      .querySelectorAll<HTMLButtonElement>(".skill-social-actions button")[2]
      .click();
    await waitForUI(
      () =>
        document.activeElement?.getAttribute("aria-label") === "Add a comment",
    );
    check(
      location.hash.includes("tab=discussion"),
      "comment action opens composer and preserves its route",
    );
    document.querySelector<HTMLButtonElement>(".install-primary")!.click();
    await waitForUI(() => !!document.querySelector("dialog[open] .scope-hint"));
    check(
      document
        .querySelector("dialog[open]")
        ?.textContent?.includes("Only available in the project"),
      "installation explains project destination before planning",
    );
    document
      .querySelector<HTMLButtonElement>("dialog[open] .modal-head > button")!
      .click();
    document
      .querySelector<HTMLButtonElement>(
        ".skill-detail > .section-heading button",
      )!
      .click();
    await waitForUI(
      () =>
        document.querySelectorAll(".source-results .skill-card-link").length ===
        2,
    );
    check(
      document.querySelector<HTMLButtonElement>(".search-scope")?.value ===
        "all",
      "back to discovery preserves aggregate scope",
    );
    const themeRect = document
      .querySelector(".quick-theme .ui-select .icon")!
      .getBoundingClientRect();
    const settingsRect = document
      .querySelector(".utility-settings .icon")!
      .getBoundingClientRect();
    check(
      Math.abs(
        themeRect.y +
          themeRect.height / 2 -
          settingsRect.y -
          settingsRect.height / 2,
      ) < 1,
      "header utility icons share a vertical center",
    );
    const preview = await API.Preview("company/platform/release-check@1.0.0");
    const reference = await API.ReadPreviewFile(
      preview.target,
      "references/checklist.md",
    );
    check(
      preview.release.version === "1.0.0" &&
        reference.text &&
        reference.content.includes("<script>"),
      "fixed package and literal reference text crossed native bridge",
    );
    const profile = await API.CommunityGetProfile("company", "");
    const edited = await API.CommunityUpdateProfile(
      "company",
      "Native reviewer",
      "Shared bridge verification",
      profile.revision,
      crypto.randomUUID(),
    );
    check(
      edited.display_name === "Native reviewer",
      "community profile saved through native bridge",
    );
    const avatarCanvas = document.createElement("canvas");
    avatarCanvas.width = 32;
    avatarCanvas.height = 32;
    const avatarContext = avatarCanvas.getContext("2d")!;
    avatarContext.fillStyle = "#063beb";
    avatarContext.fillRect(0, 0, 32, 32);
    const photographed = await API.CommunityUpdateAvatar(
      "company",
      avatarCanvas.toDataURL("image/png").split(",")[1],
      edited.revision,
      await API.NewRequestID(),
    );
    check(
      photographed.avatar.startsWith("data:image/png;base64,"),
      "avatar persisted through native bridge",
    );
    const withoutPhoto = await API.CommunityUpdateAvatar(
      "company",
      "",
      photographed.revision,
      await API.NewRequestID(),
    );
    check(withoutPhoto.avatar === "", "avatar removal crossed native bridge");
    const comment = await API.CommunityCreateComment(
      "company",
      preview.release.id,
      "",
      "Native community verification",
      crypto.randomUUID(),
    );
    const reply = await API.CommunityCreateComment(
      "company",
      preview.release.id,
      comment.comment_id,
      "Reply from the same shared client",
      crypto.randomUUID(),
    );
    const reactions = await API.CommunitySetReaction(
      "company",
      preview.release.id,
      true,
      true,
      crypto.randomUUID(),
    );
    const saved = await API.CommunityListBookmarks("company", "");
    check(
      reactions.bookmarked &&
        saved.items?.some((item) => item.release_id === preview.release.id),
      "native bookmarks persist through Hub",
    );
    const replies = await API.CommunityListComments(
      "company",
      preview.release.id,
      comment.comment_id,
      "",
    );
    check(
      replies.items?.some((item) => item.comment_id === reply.comment_id),
      "native comment replies read from Hub",
    );
    const participation = await API.CommunityParticipation("company");
    check(
      participation.supported && !participation.community_mode,
      "enterprise mode and moderation capability cross native bridge",
    );
    const reported = await API.CommunityReport(
      "company",
      preview.release.id,
      reply.comment_id,
      "Synthetic content report",
      await API.NewRequestID(),
    );
    let reportDenied = false;
    try {
      await API.CommunityReports("limited", false, "");
    } catch {
      reportDenied = true;
    }
    check(reportDenied, "non-administrator cannot read report queue");
    const reports = await API.CommunityReports("company", false, "");
    check(
      reports.items?.some((r) => r.id === reported && r.content === reply.body),
      "report queue includes exact comment and reason",
    );
    await API.CommunityResolveReport(
      "company",
      reported,
      Resolution.ReportResolution_REPORT_RESOLUTION_REMOVE_CONTENT,
      "Synthetic moderation decision",
      await API.NewRequestID(),
    );
    const resolvedReports = await API.CommunityReports("company", true, "");
    check(
      resolvedReports.items?.some((r) => r.id === reported && r.resolved),
      "moderation decision persists through native bridge",
    );
    const moderatedReplies = await API.CommunityListComments(
      "company",
      preview.release.id,
      comment.comment_id,
      "",
    );
    check(
      moderatedReplies.items?.some(
        (r) => r.comment_id === reply.comment_id && r.deleted && !r.body,
      ),
      "moderated comment body is no longer readable",
    );
    await API.CommunityDeleteComment(
      "company",
      comment.comment_id,
      crypto.randomUUID(),
    );
    check(
      document.documentElement.lang === "en",
      "new native session defaults to English",
    );
    const projects = (await API.ListProjects()) ?? [];
    check(
      projects.length === 1 && /[\\/]demo-project$/.test(projects[0]),
      "isolated project loaded",
    );
    check(
      ((await API.ListInstalled()) ?? []).length === 0,
      "fixture starts with no installations",
    );
    const restore = await API.PlanRestore(projects[0]);
    check(restore.changes?.length === 2, "exact restore plan created");
    const results = await API.ApplyPlan(restore.handle);
    check(
      results?.length === 2 && results.every((r) => r.applied),
      "two tool installations restored through native bridge",
    );
    let replayDenied = false;
    try {
      await API.ApplyPlan(restore.handle);
    } catch {
      replayDenied = true;
    }
    check(replayDenied, "consumed native plan rejected");
    const submissions = await API.Submissions("company", "", "");
    const pending = submissions.releases?.find(
      (r) =>
        r.version === "1.1.0" &&
        r.status === Status.ReleaseStatus_RELEASE_STATUS_PENDING,
    );
    if (!pending) throw new Error("pending fixture submission not found");
    const reviewed = await API.PreviewSubmission("company", pending.id);
    check(
      reviewed.release.display_name_en === "Release Readiness" &&
        reviewed.release.display_name_zh === "发布就绪检查" &&
        reviewed.release.icon === 7,
      "review preview retains bilingual identity and built-in icon",
    );
    check(
      reviewed.release.markdown.includes("final handoff"),
      "fixed review content loaded",
    );
    location.hash = `review?source=company&release=${pending.id}`;
    await waitForUI(
      () =>
        document
          .querySelector(".release-progress")
          ?.textContent?.includes("Ready for your review") ?? false,
    );
    check(
      document
        .querySelector(".review-heading")
        ?.textContent?.includes("release-check"),
      "submission deep link opens the exact pending version",
    );
    await waitForUI(() =>
      Array.from(document.querySelectorAll("button")).some(
        (b) => b.textContent === "Approve release",
      ),
    );
    check(
      (await API.NamespaceAccess("company", pending.namespace)).can_review,
      "review actions follow namespace capability",
    );
    const exact = await API.GetSkill("company", "platform", "release-check");
    check(
      exact.name === "release-check",
      "detail uses exact coordinate bridge",
    );
    const filtered = await API.FilterSubmissions(
      "company",
      "1.1.0",
      Status.ReleaseStatus_RELEASE_STATUS_PENDING,
      false,
      "",
    );
    check(
      filtered.releases?.some((r) => r.id === pending.id),
      "submission filtering uses server query",
    );
    const tasks = await API.ReviewTasks("company", "");
    check(
      tasks.supported &&
        tasks.tasks?.some((task) => task.release_id === pending.id) &&
        tasks.total > 0 &&
        !!tasks.oldest_at,
      "actionable review queue through native bridge",
    );
    await API.SetFeaturedSkill(
      "company",
      "platform",
      "release-check",
      true,
      await API.NewRequestID(),
    );
    const picks = await API.Search("company", "", "", [], "", 2, true);
    check(
      picks?.[0]?.skills?.length === 1 && picks?.[0]?.skills?.[0]?.featured,
      "curated catalog through native bridge",
    );
    check(
      (await API.GetSkill("company", "platform", "release-check")).featured,
      "exact skill preserves featured projection",
    );
    await API.SetFeaturedSkill(
      "company",
      "platform",
      "release-check",
      false,
      await API.NewRequestID(),
    );
    check(
      !(await API.Search("company", "", "", [], "", 2, true))?.[0]?.skills
        ?.length,
      "removing curated selection persists",
    );
    const scan = await API.ScanReport("company", pending.id);
    check(
      scan.supported &&
        !!scan.engine &&
        scan.complete &&
        !scan.blocking &&
        scan.digest === pending.digest,
      "immutable scan report through native bridge",
    );
    check(
      !!document.querySelector(".scan-panel"),
      "compact scan panel rendered beside review content",
    );
    await runEvaluationSmoke(pending.id, pending.digest, check, waitForUI);
    const duties = await API.PlatformAccess("company", "");
    check(
      duties.supported &&
        duties.administrator &&
        duties.can_manage_users &&
        duties.can_read_audit,
      "platform duties projected by server",
    );
    const approved = await API.Review(
      "company",
      pending.id,
      "Native runtime fixture checked",
      await API.NewRequestID(),
      true,
    );
    check(
      approved.status === Status.ReleaseStatus_RELEASE_STATUS_PUBLISHED,
      "independent review published version",
    );
    document
      .querySelector<HTMLButtonElement>(".queue-heading > button")!
      .click();
    await waitForUI(
      () =>
        document
          .querySelector(".release-progress")
          ?.textContent?.includes("Native runtime fixture checked") ?? false,
    );
    check(
      document
        .querySelector(".release-progress")
        ?.textContent?.includes("View published skill"),
      "refresh exposes review feedback and published skill action",
    );
    const releaseTop = document
      .querySelector(".review-heading")!
      .getBoundingClientRect().top;
    const proseTop = document
      .querySelector(".review-content .document-prose")!
      .getBoundingClientRect().top;
    check(
      proseTop - releaseTop < 240,
      `release status and feedback leave document near the top (${Math.round(proseTop - releaseTop)}px)`,
    );
    const feedback =
      document.querySelector<HTMLDetailsElement>(".release-feedback")!;
    feedback.querySelector<HTMLElement>("summary")!.click();
    check(
      feedback.open &&
        feedback
          .querySelector("blockquote")!
          .textContent!.includes("Native runtime fixture checked"),
      "review feedback expands without losing the full note",
    );
    feedback.querySelector<HTMLElement>("summary")!.click();
    const updates = (await API.CheckUpdates()) ?? [];
    const update = updates.find((r) => r.installation.name === "release-check");
    check(update?.available_version === "1.1.0", "stable update discovered");
    if (!update) throw new Error("update target missing");
    const updatePlan = await API.PlanUpdate(update.installation.target, "");
    check(
      (await API.ApplyPlan(updatePlan.handle))?.every((r) => r.applied),
      "native update applied",
    );
    await API.AdminChange({
      source: "public",
      action: "category",
      request_id: await API.NewRequestID(),
      id: "",
      name: "native-smoke",
      title: "Native runtime test",
      password: "",
      namespace: "",
      subject: "",
      revision: "0",
      enabled: true,
      administrator: false,
      subject_kind: Principal.PrincipalKind_PRINCIPAL_KIND_UNSPECIFIED,
      role: Role.NamespaceRole_NAMESPACE_ROLE_UNSPECIFIED,
    });
    const categories = await API.AdminList("public", "categories", "", "");
    check(
      categories.rows?.some((r) => r.name === "native-smoke"),
      "native governance mutation and read succeeded",
    );
    location.hash = "management?source=public";
    await clickManagementTab("Users");
    for (const [name, label, enabled] of [
      ["pending-approve", "Approve access", true],
      ["pending-reject", "Reject", false],
    ] as const) {
      await waitForUI(() =>
        Array.from(document.querySelectorAll("article")).some((r) =>
          r.textContent?.includes(name),
        ),
      );
      const row = Array.from(document.querySelectorAll("article")).find((r) =>
        r.textContent?.includes(name),
      )!;
      check(
        row.textContent?.includes("Awaiting approval"),
        "pending enterprise identity is clearly labeled",
      );
      (
        Array.from(row.querySelectorAll("button")).find(
          (b) => b.textContent === label,
        ) as HTMLButtonElement
      ).click();
      await waitForUI(
        () =>
          !!document.querySelector("dialog[open] .actions button:last-child"),
      );
      document
        .querySelector<HTMLButtonElement>(
          "dialog[open] .actions button:last-child",
        )!
        .click();
      await waitForUI(
        () =>
          !document.querySelector("dialog[open]") &&
          !!Array.from(document.querySelectorAll("article")).find((r) =>
            r.textContent?.includes(name),
          ) &&
          !Array.from(document.querySelectorAll("article"))
            .find((r) => r.textContent?.includes(name))
            ?.textContent?.includes("Awaiting approval"),
      );
      const users = await API.AdminList("public", "users", "", "");
      const user = users.rows?.find((u) => u.name === name);
      check(
        user?.enabled === enabled &&
          !user.approval_pending &&
          !user.administrator,
        "enterprise approval decision persisted without granting administrator",
      );
      if (enabled && user) {
        await waitForUI(() =>
          Array.from(document.querySelectorAll("article")).some(
            (r) =>
              r.textContent?.includes(name) &&
              r.textContent?.includes("Platform duties"),
          ),
        );
        const currentRow = Array.from(
          document.querySelectorAll("article"),
        ).find((r) => r.textContent?.includes(name))!;
        Array.from(currentRow.querySelectorAll("button"))
          .find((b) => b.textContent === "Platform duties")!
          .click();
        await waitForUI(
          () =>
            document.querySelectorAll(".platform-duties input").length === 2,
        );
        document
          .querySelectorAll<HTMLInputElement>(".platform-duties input")[1]
          .click();
        await waitForUI(
          () => document.querySelectorAll("dialog[open]").length === 2,
        );
        const confirm = Array.from(
          document.querySelectorAll("dialog[open]"),
        ).at(-1)!;
        confirm
          .querySelector<HTMLButtonElement>(".actions button:last-child")!
          .click();
        await waitForUI(
          () =>
            !document.querySelector("dialog[open]") &&
            !!Array.from(document.querySelectorAll("article")).find(
              (r) =>
                r.textContent?.includes(name) &&
                r.textContent?.includes("Auditor"),
            ),
        );
        check(
          (await API.PlatformAccess("public", user.id)).can_read_audit,
          "platform duty confirmation persists through native bridge",
        );
        await API.SetPlatformRole(
          "public",
          user.id,
          Duty.PlatformRole_PLATFORM_ROLE_AUDITOR,
          false,
          await API.NewRequestID(),
        );
      }
    }
    await clickManagementTab("Role grants");
    await waitForUI(
      () => !!document.querySelector(".grant-scope input[role=combobox]"),
    );
    (
      Array.from(document.querySelectorAll("button")).find(
        (b) => b.textContent === "Add role grant",
      ) as HTMLButtonElement
    ).click();
    await waitForUI(
      () =>
        document.querySelectorAll(".editor input[role=combobox]").length === 2,
    );
    check(
      !document.querySelector(".editor .actions button:not(:disabled)"),
      "grant requires selected directory entries",
    );
    const editorRect = document
      .querySelector(".management-editor")!
      .getBoundingClientRect();
    check(
      editorRect.height < 400 &&
        !!document.querySelector("dialog[open] .management-editor"),
      "role editor opens compactly in a modal above the record list",
    );
    const namespaceInput = document.querySelector<HTMLInputElement>(
      ".editor input[role=combobox]",
    )!;
    namespaceInput.focus();
    await waitForUI(() => !!document.querySelector(".editor [role=option]"));
    (
      document.querySelector(".editor [role=option]") as HTMLButtonElement
    ).click();
    await waitForUI(
      () =>
        document.querySelectorAll<HTMLInputElement>(
          ".editor input[role=combobox]",
        )[1]?.disabled === false,
    );
    const userInput = document.querySelectorAll<HTMLInputElement>(
      ".editor input[role=combobox]",
    )[1];
    userInput.focus();
    Object.getOwnPropertyDescriptor(
      HTMLInputElement.prototype,
      "value",
    )!.set!.call(userInput, "admin");
    userInput.dispatchEvent(new Event("input", { bubbles: true }));
    await waitForUI(() => !!document.querySelector(".editor [role=option]"));
    userInput.dispatchEvent(
      new KeyboardEvent("keydown", { key: "Enter", bubbles: true }),
    );
    await waitForUI(
      () => !!document.querySelector(".editor .actions button:not(:disabled)"),
    );
    check(
      userInput.value.includes("@"),
      "keyboard user selection displays readable account identity",
    );
    await chooseSelect(
      ".editor .ui-select",
      String(Principal.PrincipalKind_PRINCIPAL_KIND_GROUP),
    );
    await waitForUI(
      () => !document.querySelector(".editor .actions button:not(:disabled)"),
    );
    check(
      document.querySelectorAll<HTMLInputElement>(
        ".editor input[role=combobox]",
      )[1].value === "",
      "changing principal kind clears previous user selection",
    );
    check(
      !document.querySelector(".studio-identity") &&
        !document.querySelector(".page-management .subtitle"),
      "studio removes duplicate identity and explanatory heading",
    );
    document
      .querySelector<HTMLButtonElement>("dialog[open] .modal-head button")!
      .click();
    location.hash =
      "discover?source=company&skill=platform/release-check&version=1.1.0";
    await waitForUI(
      () => !!document.querySelector(".skill-detail .document-prose"),
    );
    await chooseSelect(".version-selector .ui-select", "1.0.0");
    await waitForUI(() => location.hash.includes("version=1.0.0"));
    check(
      location.hash.includes("version=1.0.0"),
      "version selection updates the shareable route",
    );
    location.hash =
      "discover?source=company&skill=platform/missing-skill&version=1.0.0";
    await waitForUI(
      () => !!document.querySelector(".detail-load-state [role=alert]"),
    );
    check(
      document
        .querySelector(".detail-load-state")
        ?.textContent?.includes("Try again"),
      "unavailable skill offers retry instead of endless loading",
    );
    location.hash = "";
    await waitForUI(
      () => !!document.querySelector(".source-results .skill-card-link"),
    );
    check(
      !document.querySelector(".skill-detail"),
      "empty hash restores home after browser navigation",
    );
    const cache = await API.CacheInfo();
    check(
      cache.entries === 3 &&
        cache.bytes > 0 &&
        cache.bytes <= cache.limit_bytes,
      "shared cache bounds reported",
    );
    check(
      (await API.ClearCache()).entries === 0,
      "native cache cleanup completed",
    );
    const installed = (await API.ListInstalled()) ?? [];
    check(installed.length === 2, "cache cleanup preserved installations");
    const remove = installed.find((r) => r.name === "incident-brief");
    if (!remove) throw new Error("removal fixture target missing");
    const removePlan = await API.PlanRemove(remove.target);
    check(
      (await API.ApplyPlan(removePlan.handle))?.every((r) => r.applied),
      "native removal applied",
    );
    const final = (await API.ListInstalled()) ?? [];
    check(
      final.length === 1 &&
        final[0].name === "release-check" &&
        final[0].version === "1.1.0",
      "final native inventory verified",
    );
    check(
      final[0].display_name_en === "Release Readiness" &&
        final[0].display_name_zh === "发布就绪检查" &&
        final[0].icon === 7,
      "update stores exact release identity in local inventory",
    );
    location.hash = "installed";
    await waitForUI(
      () =>
        document.querySelector(".installation-heading h2")?.textContent ===
        "Release Readiness",
    );
    setLanguage("zh");
    await waitForUI(
      () =>
        document.querySelector(".installation-heading h2")?.textContent ===
        "发布就绪检查",
    );
    check(
      document.querySelector(".installation-content .skill-coordinate")
        ?.textContent === "platform/release-check",
      "local language switch preserves stable coordinate",
    );
    setLanguage("en");
    const project = await API.OpenProject(projects[0], false);
    check(
      !project.needs_lock &&
        project.manifest.skills?.every((s) => s.version === "1.0.0"),
      "local update preserved project declaration",
    );
    check(
      !document.querySelector(".project-footer"),
      "installed App has no software-download footer",
    );
    location.hash = "projects";
    await waitForUI(() => !!document.querySelector(".project-detail"));
    check(
      !document.querySelector("main input"),
      "project page shows recent projects without a permanent directory form",
    );
    check(
      !!document.querySelector(
        '.project-toolbar [aria-label="Current project"]',
      ) && !document.querySelector(".project-shortcuts, .project-detail .path"),
      "project context uses one selector without duplicate paths or removal cards",
    );
    await waitForUI(() =>
      Array.from(document.querySelectorAll("button")).some(
        (b) => b.textContent === "Add a skill",
      ),
    );
    Array.from(document.querySelectorAll("button"))
      .find((b) => b.textContent === "Add a skill")!
      .click();
    await waitForUI(
      () =>
        !!document.querySelector("dialog[open] .project-skill-results button"),
    );
    check(
      !!Array.from(
        document.querySelectorAll<HTMLButtonElement>("dialog[open] button"),
      ).find((b) => b.textContent === "Save to manifest")?.disabled,
      "project save stays disabled until a skill version is selected",
    );
    document
      .querySelector<HTMLButtonElement>(
        "dialog[open] .project-skill-results button",
      )!
      .click();
    await waitForUI(
      () => !!document.querySelector(".project-skill-picker .ui-select"),
    );
    check(
      true,
      "project dialog selects real skills and exact versions without IDs",
    );
    const projectSave = Array.from(
      document.querySelectorAll<HTMLButtonElement>("dialog[open] button"),
    ).find((b) => b.textContent === "Save to manifest")!;
    const projectDialog = document
      .querySelector("dialog[open]")!
      .getBoundingClientRect();
    check(
      !projectSave.disabled &&
        projectSave.getBoundingClientRect().bottom <= projectDialog.bottom,
      "selected project version keeps the save action visible and enabled",
    );
    document
      .querySelector<HTMLButtonElement>("dialog[open] .modal-head button")!
      .click();
    location.hash = "settings?source=company";
    await waitForUI(() =>
      Array.from(document.querySelectorAll("button")).some(
        (b) => b.textContent === "Change password",
      ),
    );
    check(
      !document.querySelector("input[type=password]"),
      "account security keeps password fields out of settings overview",
    );
    const passwordTrigger = Array.from(
      document.querySelectorAll("button"),
    ).find((b) => b.textContent === "Change password")!;
    await waitForUI(
      () =>
        !!document.querySelector<HTMLButtonElement>(
          '[data-testid="check-app-update"]',
        ) &&
        !document.querySelector<HTMLButtonElement>(
          '[data-testid="check-app-update"]',
        )!.disabled,
    );
    check(
      !document.querySelector(".app-update-result"),
      "App update checks require an explicit user action",
    );
    document
      .querySelector<HTMLButtonElement>('[data-testid="check-app-update"]')!
      .click();
    await waitForUI(
      () => !!document.querySelector(".app-update-result strong"),
    );
    check(
      document.querySelector(".app-update-result strong")!.textContent ===
        "v9.0.0",
      "App update result displays the available version",
    );
    document
      .querySelector<HTMLButtonElement>(".app-update-result button")!
      .click();
    await waitForUI(
      () => !!document.querySelector("dialog[open] .app-update-notes"),
    );
    check(
      !document.querySelector("dialog[open] script") &&
        document
          .querySelector(".app-update-notes")!
          .textContent!.includes("<script>"),
      "Untrusted release notes remain inert text",
    );
    check(
      document.querySelector<HTMLAnchorElement>("dialog[open] a")!.href ===
        "https://github.com/skillhubhq/skillhub/releases/tag/v9.0.0",
      "Manual download targets the exact official release",
    );
    document
      .querySelector<HTMLButtonElement>("dialog[open] .modal-head button")!
      .click();
    document
      .querySelector<HTMLInputElement>(".app-update-preview input")!
      .click();
    document
      .querySelector<HTMLButtonElement>('[data-testid="check-app-update"]')!
      .click();
    await waitForUI(
      () =>
        !!document
          .querySelector(".app-update-result")
          ?.textContent?.includes("Could not reach GitHub"),
    );
    check(
      !document.querySelector(".app-update-result strong"),
      "Failed update checks never claim the App is current",
    );
    passwordTrigger.focus();
    passwordTrigger.click();
    await waitForUI(
      () =>
        document.querySelectorAll("dialog[open] input[type=password]")
          .length === 3,
    );
    check(true, "password change opens in a focused modal");
    document
      .querySelector<HTMLButtonElement>("dialog[open] .modal-head button")!
      .click();
    await waitForUI(() => !document.querySelector("dialog[open]"));
    check(
      document.activeElement === passwordTrigger,
      "closing a dialog restores the keyboard trigger focus",
    );
    passwordTrigger.click();
    await waitForUI(() => !!document.querySelector("dialog[open]"));
    document
      .querySelector("dialog[open]")!
      .dispatchEvent(new Event("cancel", { bubbles: false, cancelable: true }));
    await waitForUI(() => !document.querySelector("dialog[open]"));
    check(
      document.activeElement === passwordTrigger,
      "canceling a reopened dialog restores the same keyboard trigger focus",
    );
    location.hash = "sources";
    await waitForUI(() =>
      Array.from(document.querySelectorAll("button")).some(
        (b) => b.textContent === "Connect a hub",
      ),
    );
    await waitForUI(
      () =>
        document.querySelectorAll(".connection-account p").length === 2 &&
        Array.from(document.querySelectorAll(".connection-account p")).every(
          (p) => p.textContent === "admin",
        ),
    );
    check(
      !document.querySelector(".cache-section") &&
        Array.from(document.querySelectorAll(".source-card")).every(
          (card) => card.getBoundingClientRect().height < 230,
        ),
      "connections show verified accounts in compact rows without cache clutter",
    );
    Array.from(document.querySelectorAll("button"))
      .find((b) => b.textContent === "Connect a hub")!
      .click();
    await waitForUI(
      () => !!document.querySelector("input[placeholder=company]"),
    );
    const sourceInputs = document.querySelectorAll<HTMLInputElement>(
      "dialog[open] form input",
    );
    const inputValue = (input: HTMLInputElement, value: string) => {
      Object.getOwnPropertyDescriptor(
        HTMLInputElement.prototype,
        "value",
      )!.set!.call(input, value);
      input.dispatchEvent(new Event("input", { bubbles: true }));
    };
    inputValue(sourceInputs[0], "limited");
    inputValue(
      sourceInputs[1],
      sources.find((s) => s.alias === "company")!.url,
    );
    await waitForUI(() =>
      Array.from(document.querySelectorAll<HTMLButtonElement>("button")).some(
        (b) => b.textContent === "Connect hub" && !b.disabled,
      ),
    );
    (
      Array.from(document.querySelectorAll("button")).find(
        (b) => b.textContent === "Connect hub",
      ) as HTMLButtonElement
    ).click();
    await waitForUI(() =>
      Array.from(document.querySelectorAll(".source-card")).some((el) =>
        el.textContent?.includes("limited"),
      ),
    );
    const inbox = await API.Notifications("limited", "");
    check(
      inbox.supported &&
        inbox.items?.some((n) => n.release_id === pending.id && n.approved),
      "review outcome persisted for submitting identity",
    );
    location.hash = "submissions?source=limited";
    await waitForUI(() =>
      Array.from(document.querySelectorAll("button")).some((b) =>
        b.getAttribute("aria-label")?.startsWith("Inbox"),
      ),
    );
    Array.from(document.querySelectorAll("button"))
      .find((b) => b.getAttribute("aria-label")?.startsWith("Inbox"))!
      .click();
    await waitForUI(() => !!document.querySelector(".review-outcomes button"));
    const inboxTrigger =
      document.querySelector<HTMLButtonElement>(".utility-inbox")!;
    check(
      inboxTrigger.getBoundingClientRect().width === 44 &&
        inboxTrigger.getAttribute("aria-expanded") === "true",
      "inbox uses a compact accessible header control",
    );
    const inboxTabs = Array.from(
      document.querySelectorAll<HTMLButtonElement>(".inbox-tabs button"),
    );
    check(
      inboxTabs.length === 2 &&
        inboxTabs[1].getAttribute("aria-pressed") === "true",
      "submission outcomes open for an author without pending reviews",
    );
    const inboxBounds = document
      .querySelector("dialog[open]")!
      .getBoundingClientRect();
    inboxTabs[0].click();
    await waitForUI(() => !!document.querySelector(".review-tasks"));
    check(
      !document.querySelector(".review-outcomes"),
      "inbox separates review tasks from submission outcomes",
    );
    check(
      Math.abs(
        document.querySelector("dialog[open]")!.getBoundingClientRect().height -
          inboxBounds.height,
      ) < 1,
      "inbox keeps a stable height across task and outcome tabs",
    );
    inboxTabs[1].click();
    await waitForUI(() => !!document.querySelector(".review-outcomes button"));
    check(
      document.querySelector("dialog[open]")!.getBoundingClientRect().right <=
        window.innerWidth,
      "notification dialog fits native window",
    );
    document
      .querySelector<HTMLButtonElement>(".review-outcomes button")!
      .click();
    await waitForUI(
      () =>
        !document.querySelector("dialog[open]") &&
        !!document.querySelector(".review-content"),
    );
    check(
      (await API.Notifications("limited", "")).items?.some(
        (n) => n.release_id === pending.id && n.read,
      ),
      "opening notification marks read and navigates to feedback",
    );
    location.hash = "management?source=limited";
    await clickManagementTab("Role grants");
    await waitForUI(() =>
      Array.from(document.querySelectorAll("button")).some(
        (b) => b.textContent === "Add role grant",
      ),
    );
    (
      Array.from(document.querySelectorAll("button")).find(
        (b) => b.textContent === "Add role grant",
      ) as HTMLButtonElement
    ).click();
    await waitForUI(
      () => !!document.querySelector(".editor input[role=combobox]"),
    );
    document
      .querySelector<HTMLInputElement>(".editor input[role=combobox]")!
      .focus();
    await waitForUI(() => !!document.querySelector(".editor [role=option]"));
    (
      document.querySelector(".editor [role=option]") as HTMLButtonElement
    ).click();
    await waitForUI(
      () =>
        document
          .querySelector(".grant-access-status")
          ?.textContent?.includes("Only namespace or Hub administrators") ??
        false,
    );
    check(
      document.querySelectorAll<HTMLInputElement>(
        ".editor input[role=combobox]",
      )[1]?.disabled,
      "non-admin grant editor blocks user lookup before directory access",
    );
    check(
      !document.querySelector(".editor .actions button:not(:disabled)"),
      "namespace grant form explains denied capability and prevents submission",
    );
    location.hash = "downloads";
    await waitForUI(() => !!document.querySelector(".downloads-page"));
    check(
      document.querySelectorAll(
        '.downloads-page a[href="https://github.com/skillhubhq/skillhub/releases"]',
      ).length === 2,
      "download page links app and CLI to the official release channel",
    );
    check(
      Array.from(
        document.querySelectorAll<HTMLAnchorElement>(".downloads-page a"),
      ).every(
        (a) =>
          a.referrerPolicy === "no-referrer" && !a.href.includes("localhost"),
      ),
      "project links omit private Hub context and suppress referrers",
    );
    check(
      document.documentElement.scrollWidth <= window.innerWidth + 1,
      "native downloads page fits window width",
    );
    location.hash = "publish?source=public";
    await waitForUI(() => !!document.querySelector(".publish-main input"));
    inputValue(
      document.querySelector<HTMLInputElement>(".publish-main input")!,
      projects[0].replace(/[\\/]demo-project$/, "/incident-brief"),
    );
    const clickButton = async (label: string) => {
      await waitForUI(() =>
        Array.from(document.querySelectorAll("button")).some(
          (b) => b.textContent?.trim() === label && !b.disabled,
        ),
      );
      (
        Array.from(document.querySelectorAll("button")).find(
          (b) => b.textContent?.trim() === label && !b.disabled,
        ) as HTMLButtonElement
      ).click();
    };
    await clickButton("Preview folder");
    await waitForUI(
      () => !!document.querySelector(".publish-main .document-prose"),
    );
    await clickButton("Edit skill files");
    await waitForUI(() => !!document.querySelector(".skill-text-editor"));
    const textEditor =
      document.querySelector<HTMLTextAreaElement>(".skill-text-editor")!;
    const initialText = textEditor.value;
    Object.getOwnPropertyDescriptor(
      HTMLTextAreaElement.prototype,
      "value",
    )!.set!.call(
      textEditor,
      initialText + "\nNative draft editor verification.\n",
    );
    textEditor.dispatchEvent(new Event("input", { bubbles: true }));
    await clickButton("Save draft");
    await waitForUI(
      () =>
        document.querySelector("dialog .actions [role=status]")?.textContent ===
        "Changes saved to this draft",
    );
    check(true, "native draft editor saves through shared Go core");
    document
      .querySelector<HTMLButtonElement>("dialog[open] .modal-head button")!
      .click();
    await waitForUI(
      () =>
        document
          .querySelector(".publish-main .document-prose")
          ?.textContent?.includes("Native draft editor verification.") ?? false,
    );
    check(true, "saved draft is revalidated and previewed before publication");
    window.scrollTo(0, document.body.scrollHeight);
    await clickButton("Continue to details");
    await waitForUI(() => !!document.querySelector('input[lang="en"]'));
    check(
      window.scrollY === 0,
      "publish step starts at the top of the new form",
    );
    inputValue(
      document.querySelector<HTMLInputElement>('input[lang="en"]')!,
      "Incident Brief",
    );
    inputValue(
      document.querySelector<HTMLInputElement>('input[lang="zh-CN"]')!,
      "事件简报",
    );
    inputValue(
      document.querySelector<HTMLInputElement>(".publish-main input[pattern]")!,
      "1.2.0",
    );
    document
      .querySelector<HTMLInputElement>('input[name="skill-icon"][value="3"]')!
      .click();
    await clickButton("Review submission");
    await waitForUI(
      () =>
        document
          .querySelector(".publish-main")
          ?.textContent?.includes("Ready to share?") ?? false,
    );
    check(
      document
        .querySelector(".publish-main")
        ?.textContent?.includes("事件简报"),
      "publish confirmation includes both names and exact package ID",
    );
    await clickButton("Publish version");
    await waitForUI(() => !!document.querySelector(".publish-result"));
    check(
      document.querySelector(".publish-result h3")?.textContent ===
        "Incident Brief",
      "native publish form persists reader-facing title",
    );
    location.hash = "discover?source=public&q=事件简报";
    await waitForUI(
      () =>
        document.querySelector(".skill-card-link")?.textContent ===
        "Incident Brief",
    );
    check(
      document.querySelector(".skill-coordinate")?.textContent ===
        "platform/incident-brief",
      "localized search displays title and stable coordinate separately",
    );
    document.querySelector<HTMLAnchorElement>(".skill-card-link")!.click();
    await waitForUI(
      () =>
        document.querySelector(".detail-hero h1")?.textContent ===
        "Incident Brief",
    );
    setLanguage("zh");
    await waitForUI(
      () =>
        document.querySelector(".detail-hero h1")?.textContent === "事件简报",
    );
    check(
      (document
        .querySelector(".detail-symbol .skill-emblem")
        ?.getBoundingClientRect().width ?? 0) > 16,
      "detail uses authored built-in icon and language preference",
    );
    setLanguage("en");
    await API.ClearCache();
    for (const [alias, origin] of [
      ["signed-out", "company"],
      ["visitor", "public"],
    ]) {
      location.hash = "sources";
      await waitForUI(() =>
        Array.from(document.querySelectorAll("button")).some(
          (b) => b.textContent === "Connect a hub",
        ),
      );
      Array.from(document.querySelectorAll("button"))
        .find((b) => b.textContent === "Connect a hub")!
        .click();
      await waitForUI(
        () => document.querySelectorAll("dialog[open] form input").length === 2,
      );
      const fields = document.querySelectorAll<HTMLInputElement>(
        "dialog[open] form input",
      );
      inputValue(fields[0], alias);
      inputValue(fields[1], sources.find((s) => s.alias === origin)!.url);
      await waitForUI(() =>
        Array.from(document.querySelectorAll<HTMLButtonElement>("button")).some(
          (b) => b.textContent === "Connect hub" && !b.disabled,
        ),
      );
      Array.from(document.querySelectorAll<HTMLButtonElement>("button"))
        .find((b) => b.textContent === "Connect hub")!
        .click();
      await waitForUI(() =>
        Array.from(document.querySelectorAll(".source-card")).some((el) =>
          el.textContent?.includes(alias),
        ),
      );
      location.hash = `discover?source=${alias}`;
      if (alias === "signed-out") {
        await waitForUI(() => !!document.querySelector(".access-state"));
        check(
          document.querySelector(".access-state h2")?.textContent ===
            "Sign in to your workspace" &&
            !document.querySelector(
              ".discovery-search,.source-results,.team-collections",
            ),
          "private anonymous browsing presents sign-in without false empty content",
        );
        check(
          !document
            .querySelector("main")
            ?.textContent?.includes("no saved credential"),
          "credential details are not exposed in access guidance",
        );
      } else {
        await waitForUI(() => !!document.querySelector(".catalog-item"));
        check(
          !document.querySelector(".access-state"),
          "public Hub remains browsable without an account",
        );
      }
    }
  } catch (failure) {
    error = failure instanceof Error ? failure.message : String(failure);
  }
  await Call.ByName("main.NativeSmokeReporter.Finish", {
    passed: !error,
    checks,
    error,
    user_agent: navigator.userAgent,
    location: window.location.href,
    platform: "",
    duration_ms: performance.now() - started,
  });
}
