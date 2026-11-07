import { Call } from "@wailsio/runtime";
import { API } from "./platform";
import { setLanguage } from "./i18n";
import { setTheme } from "./preferences";

// 仅由 native-smoke 入口导入，在独立进程间验证持久数据与真实凭据库。
export async function runUpgradeSmoke(): Promise<boolean> {
  const mode = (await Call.ByName(
    "main.NativeSmokeReporter.UpgradeMode",
  )) as string;
  if (!mode) return false;
  const checks: string[] = [];
  let error = "";
  const started = performance.now();
  const check = (condition: unknown, label: string) => {
    if (!condition) throw new Error(label);
    checks.push(label);
  };
  const wait = async (condition: () => boolean, label = "native shell") => {
    const until = Date.now() + 15000;
    while (!condition()) {
      if (Date.now() > until) throw new Error(`upgrade UI timed out: ${label}`);
      await new Promise((r) => setTimeout(r, 50));
    }
  };
  // 弹窗可先于 ui.run 的 finally 渲染；只在按钮解除禁用后触发操作。
  const click = async (selector: string, label: string) => {
    const find = () =>
      Array.from(document.querySelectorAll<HTMLButtonElement>(selector)).find(
        (button) => button.textContent?.includes(label) && !button.disabled,
      );
    await wait(() => !!find(), `enabled button: ${label}`);
    find()!.click();
  };
  try {
    await wait(() => !!document.querySelector("nav button[aria-current=page]"));
    if (mode === "seed") {
      await Call.ByName("main.NativeSmokeReporter.SeedUpgrade");
      location.hash = "discover?source=partner";
      setLanguage("zh");
      setTheme("dark");
      await wait(
        () => localStorage.getItem("skillhub.community") === "partner",
      );
      check(true, "seeded persistent preferences and active Hub");
    } else {
      check(
        localStorage.getItem("skillhub.language") === "zh" &&
          document.documentElement.lang === "zh-CN",
        "language survived process and bundle replacement",
      );
      check(
        localStorage.getItem("skillhub.theme") === "dark" &&
          document.documentElement.dataset.theme === "dark",
        "theme survived process and bundle replacement",
      );
      check(
        localStorage.getItem("skillhub.community") === "partner",
        "active Hub survived process and bundle replacement",
      );
      const draft = (await Call.ByName(
        "main.NativeSmokeReporter.VerifyUpgrade",
      )) as { id: string };
      check(
        true,
        "source identities, credential references, inventory and projects preserved",
      );
      check(
        true,
        "system credential bytes retained without environment token injection",
      );
      check(
        true,
        "drafts, project locks, four installations and local modifications retained byte-for-byte",
      );
      const content = await API.ReadDraftFile(draft.id, "SKILL.md");
      check(
        content.text.includes("Saved before application replacement."),
        "saved draft readable through production bridge",
      );
      if (mode === "candidate" || mode === "restart") {
        const drafts = (await Call.ByName(
          "main.DesktopService.ListDrafts",
        )) as { id: string; name: string }[];
        check(
          drafts.length === 2 &&
            drafts.some(
              (d) => d.id === draft.id && d.name === "upgrade-draft",
            ) &&
            drafts.some((d) => !d.name),
          "saved and incomplete drafts are discoverable after upgrade",
        );
        location.hash = "publish?source=company";
        await click("button", "打开已保存草稿");
        await wait(
          () => !!document.querySelector(".saved-draft-list"),
          "saved draft picker",
        );
        for (const [label, expected] of [
          ["upgrade-draft", "Saved before application replacement."],
          ["未命名技能", "name: [unfinished"],
        ]) {
          if (!document.querySelector(".saved-draft-list")) {
            await click("button", "打开已保存草稿");
            await wait(
              () => !!document.querySelector(".saved-draft-list"),
              "saved draft picker",
            );
          }
          await click(".saved-draft-list button", label);
          await wait(
            () =>
              document
                .querySelector<HTMLTextAreaElement>(".skill-text-editor")
                ?.value.includes(expected) === true,
            `editor content: ${label}`,
          );
          check(
            true,
            `saved draft ${label} reopened with its original content in the real editor`,
          );
          await click("dialog[open] .modal-head button", "×");
          await wait(
            () => !document.querySelector(".skill-editor-layout"),
            "closed draft editor",
          );
        }
        // 恢复原选择，下一次启动仍须验证同一个偏好。
        location.hash = "discover?source=partner";
        await wait(
          () => localStorage.getItem("skillhub.community") === "partner",
        );
      }
    }
    for (const alias of ["company", "partner"]) {
      const identity = await API.Identity(alias);
      check(
        identity.username === "admin",
        `native App authenticated ${alias} using the OS credential store`,
      );
    }
    check(
      (await API.ListInstalled())?.length === 4,
      "both global and project installations visible to the App",
    );
    await new Promise((resolve) => setTimeout(resolve, 1200));
  } catch (failure) {
    error = failure instanceof Error ? failure.message : String(failure);
  }
  await Call.ByName("main.NativeSmokeReporter.Finish", {
    passed: !error,
    checks,
    error,
    user_agent: navigator.userAgent,
    location: location.href,
    platform: "",
    duration_ms: performance.now() - started,
  });
  return true;
}
