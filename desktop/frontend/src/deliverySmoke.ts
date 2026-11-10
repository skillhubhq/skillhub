import { API } from "./ui";
import { Visibility } from "../bindings/github.com/skillhubhq/skillhub/gen/common/registry/v1/models";

// 独立 fixture 的原生桥接回归，所有测试凭据和文件由 fixture 清理。
export async function runDeliverySmoke(
  check: (value: unknown, label: string) => void,
  wait: (p: () => boolean) => Promise<void>,
) {
  const source = "public",
    coordinate = "public/platform/incident-brief";
  const releases = await API.Releases(coordinate, "");
  const member = releases.releases?.[0];
  if (!member) throw new Error("delivery fixture missing published skill");
  await API.Subscribe(coordinate, true, await API.NewRequestID());
  check(
    await API.Subscription(coordinate),
    "native skill subscription persists",
  );
  check(
    (await API.Subscriptions(source, "")).items?.some(
      (s) => s.name === "incident-brief",
    ),
    "native following list includes skill",
  );
  await API.Subscribe(coordinate, false, await API.NewRequestID());
  const token = await API.CreateAccessToken(source, "native CI", "read", "", 1);
  check(
    token.secret.startsWith("sht_"),
    "native automation token returns secret once",
  );
  await API.RevokeAccessToken(source, token.token.id);
  check(
    (await API.AccessTokens(source))?.find((v) => v.id === token.token.id)
      ?.revoked,
    "native automation token revocation persists",
  );
  const first = await API.PublishSuite({
    source,
    namespace: "platform",
    name: "delivery-suite",
    version: "1.0.0",
    description: "Delivery suite",
    visibility: Visibility.Visibility_VISIBILITY_PUBLIC,
    members: [
      {
        namespace: "platform",
        name: "incident-brief",
        version: member.version,
        digest: member.digest,
      },
    ],
    entry: "platform/incident-brief",
    request_id: await API.NewRequestID(),
  });
  const second = await API.PublishSuite({
    source,
    namespace: "platform",
    name: "delivery-suite",
    version: "1.1.0",
    description: "Delivery suite with updated guidance. ".repeat(15),
    visibility: Visibility.Visibility_VISIBILITY_PUBLIC,
    members: [
      {
        namespace: "platform",
        name: "incident-brief",
        version: member.version,
        digest: member.digest,
      },
    ],
    entry: "platform/incident-brief",
    request_id: await API.NewRequestID(),
  });
  check(
    (
      await API.CompareReleases(
        "public/platform/delivery-suite",
        first.id,
        second.id,
      )
    ).files?.some((f) => f.path === "SKILL.md"),
    "native exact version comparison returns content changes",
  );
  const projects = await API.ListProjects();
  if (!projects?.length) throw new Error("delivery fixture project missing");
  const plan = await API.PlanInstall(
    "public/platform/delivery-suite@1.1.0",
    "gemini-cli",
    "project",
    projects[0],
  );
  check(plan.changes?.length === 2, "native install expands suite and member");
  await API.ApplyPlan(plan.handle);
  const suite = (await API.Suites())?.find(
    (s) => s.coordinate.Name === "delivery-suite",
  );
  if (!suite) throw new Error("native suite ownership missing");
  const remove = await API.PlanRemove(suite.target);
  check(
    remove.changes?.length === 2,
    "native remove previews owned suite members",
  );
  await API.ApplyPlan(remove.handle);
  location.hash =
    "discover?source=public&skill=platform/delivery-suite&version=1.1.0";
  await wait(
    () =>
      document
        .querySelector(".detail-hero h1")
        ?.textContent?.includes("delivery-suite") ?? false,
  );
  const button = [
    ...document.querySelectorAll<HTMLButtonElement>("button"),
  ].find((b) => b.textContent === "Compare versions");
  button?.click();
  await wait(() => !!document.querySelector("dialog[open] .version-file"));
  check(
    document.querySelector("dialog[open]")!.getBoundingClientRect().height <=
      window.innerHeight,
    "native version comparison stays inside the window",
  );
  check(
    !!document.querySelector("dialog[open] .diff-added"),
    "native comparison highlights added lines",
  );
  const diff = document.querySelector<HTMLElement>(".version-unified")!;
  check(
    diff.scrollWidth <= diff.clientWidth + 1 &&
      getComputedStyle(diff.querySelector("span")!).whiteSpace === "pre-wrap",
    "native comparison wraps long text without horizontal scrolling",
  );
  document
    .querySelector<HTMLButtonElement>(
      'dialog[open] button[aria-label="Close dialog"]',
    )
    ?.click();
  await wait(() => !document.querySelector("dialog[open]"));
  const follow = () =>
    [
      ...document.querySelectorAll<HTMLButtonElement>("button[aria-pressed]"),
    ].find(
      (b) =>
        b.textContent === "Follow updates" || b.textContent === "Following",
    );
  await wait(() => !!follow() && !follow()!.disabled);
  follow()!.click();
  await wait(
    () =>
      follow()?.getAttribute("aria-pressed") === "true" && !follow()!.disabled,
  );
  document.querySelector<HTMLButtonElement>(".utility-inbox")!.click();
  await wait(() => !!document.querySelector("dialog[open]"));
  const dialogButton = (text: string) =>
    [
      ...document.querySelectorAll<HTMLButtonElement>("dialog[open] button"),
    ].find((b) => b.textContent === text && !b.disabled);
  await wait(() => !!dialogButton("Updates"));
  dialogButton("Updates")!.click();
  await wait(() => !!dialogButton("Following"));
  dialogButton("Following")!.click();
  await wait(() => !!dialogButton("Unfollow"));
  const subscriptionsDialog = [
    ...document.querySelectorAll<HTMLDialogElement>("dialog[open]"),
  ].at(-1)!;
  const subscriptionsHeight =
    subscriptionsDialog.getBoundingClientRect().height;
  check(
    subscriptionsHeight <= 440,
    "subscription management uses a compact dialog",
  );
  dialogButton("Unfollow")!.click();
  await wait(
    () =>
      !document.querySelector("dialog[open] .loading-skeleton") &&
      !dialogButton("Unfollow"),
  );
  await wait(
    () =>
      follow()?.getAttribute("aria-pressed") === "false" && !follow()!.disabled,
  );
  check(true, "native unfollow in Inbox synchronizes the mounted skill detail");
  check(
    Math.abs(
      subscriptionsDialog.getBoundingClientRect().height - subscriptionsHeight,
    ) < 2 &&
      subscriptionsDialog.textContent?.includes("No followed skills yet"),
    "removing the last subscription shows an explicit empty state without resizing the dialog",
  );
  while (document.querySelector("dialog[open]")) {
    const dialogs = document.querySelectorAll("dialog[open]");
    const top = dialogs[dialogs.length - 1];
    top
      .querySelector<HTMLButtonElement>('button[aria-label="Close dialog"]')!
      .click();
    await wait(() => !top.isConnected);
  }
  location.hash = "publish?source=public";
  await wait(() =>
    [...document.querySelectorAll("button")].some(
      (b) => b.textContent === "Create a skill suite",
    ),
  );
  [...document.querySelectorAll<HTMLButtonElement>("button")]
    .find((b) => b.textContent === "Create a skill suite")!
    .click();
  await wait(
    () =>
      !!document.querySelector('dialog[open] input[aria-label="Find a skill"]'),
  );
  const searchInput = document.querySelector<HTMLInputElement>(
    'dialog[open] input[aria-label="Find a skill"]',
  )!;
  const setInput = Object.getOwnPropertyDescriptor(
    HTMLInputElement.prototype,
    "value",
  )!.set!;
  setInput.call(searchInput, "no-such-fixture-skill");
  searchInput.dispatchEvent(new Event("input", { bubbles: true }));
  await wait(() => !!dialogButton("Search"));
  const enter = new KeyboardEvent("keydown", {
    key: "Enter",
    bubbles: true,
    cancelable: true,
  });
  searchInput.dispatchEvent(enter);
  await wait(
    () =>
      document
        .querySelector("dialog[open]")
        ?.textContent?.includes("No skills match your search.") ?? false,
  );
  check(
    enter.defaultPrevented,
    "suite search Enter searches without submitting the publication form",
  );
  check(true, "suite search renders an explicit no-results state");
  document
    .querySelector<HTMLButtonElement>(
      'dialog[open] button[aria-label="Close dialog"]',
    )!
    .click();
  await wait(() => !document.querySelector("dialog[open]"));
  location.hash = "sources";
  await wait(() => !!document.querySelector(".source-card"));
}
