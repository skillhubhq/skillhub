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
    description: "Delivery suite with updated guidance",
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
  document
    .querySelector<HTMLButtonElement>('dialog[open] button[aria-label="Close"]')
    ?.click();
  location.hash = "sources";
  await wait(() => !!document.querySelector(".source-card"));
}
