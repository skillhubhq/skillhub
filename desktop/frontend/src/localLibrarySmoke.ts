import { API } from "./ui";

export async function runLocalLibrarySmoke(
  check: (ok: unknown, label: string) => void,
  wait: (ready: () => boolean) => Promise<void>,
) {
  const installed = (await API.ListInstalled()) ?? [];
  const source = installed.find((i) => i.name === "release-check");
  if (!source) throw new Error("local library fixture installation missing");
  const project = ((await API.ListProjects()) ?? [])[0];
  const destination = `${project}/independent/${source.name}`;
  const copied = await API.PlanLocalAction(source.target, "copy", destination);
  const target = copied.local!.target;
  await API.ApplyPlan(copied.handle);
  const original = await API.ReadLocalSkill(target, "SKILL.md");
  const inventory = await API.LocalInventory();
  const local = inventory.skills?.find((i) => i.target === target);
  check(
    local && !local.installation && !local.association,
    "independent copy is discovered without an invented source or version",
  );
  location.hash = "installed";
  await wait(
    () =>
      !!Array.from(document.querySelectorAll(".installed-row")).find((r) =>
        r.textContent?.includes(target),
      ),
  );
  const row = Array.from(document.querySelectorAll(".installed-row")).find(
    (r) => r.textContent?.includes(target),
  )!;
  const click = (label: string, root: ParentNode = document) => {
    const button = Array.from(
      root.querySelectorAll<HTMLButtonElement>("button"),
    ).find((b) => b.textContent?.trim() === label);
    if (!button) throw new Error(`Missing local library action: ${label}`);
    button.click();
  };
  click("View files", row);
  await wait(() => !!document.querySelector("dialog[open] .document-prose"));
  check(
    (document.querySelector("dialog[open]")?.getBoundingClientRect().bottom ??
      Infinity) <= window.innerHeight,
    "local file viewer fits inside the native window",
  );
  click("Edit", document.querySelector("dialog[open]")!);
  await wait(() => !!document.querySelector(".local-editor"));
  const editor = document.querySelector<HTMLTextAreaElement>(".local-editor")!;
  Object.getOwnPropertyDescriptor(
    HTMLTextAreaElement.prototype,
    "value",
  )!.set!.call(editor, `${original.file.content}\nLocal editor acceptance.\n`);
  editor.dispatchEvent(new Event("input", { bubbles: true }));
  await wait(() =>
    Array.from(
      document.querySelectorAll<HTMLButtonElement>("dialog[open] button"),
    ).some((b) => b.textContent === "Review changes" && !b.disabled),
  );
  click("Review changes", document.querySelector("dialog[open]")!);
  await wait(() => !!document.querySelector("dialog[open] .local-plan"));
  check(
    document.querySelector(".local-plan")?.textContent?.includes("SKILL.md"),
    "native text editing previews file differences before writing",
  );
  click("Apply changes", document.querySelector("dialog[open]")!);
  await wait(
    () =>
      document
        .querySelector("dialog[open] .modal-head")
        ?.textContent?.includes("Changes complete") ?? false,
  );
  document
    .querySelector<HTMLButtonElement>("dialog[open] .modal-head > button")!
    .click();
  await wait(() => !document.querySelector("dialog[open]"));
  const edited = await API.ReadLocalSkill(target, "SKILL.md");
  check(
    edited.file.content.includes("Local editor acceptance."),
    "native editor persists the selected local file",
  );
  const menuTrigger = document.querySelector<HTMLButtonElement>(
    ".library-toolbar .action-menu-trigger",
  )!;
  menuTrigger.focus();
  menuTrigger.dispatchEvent(
    new KeyboardEvent("keydown", { key: "ArrowDown", bubbles: true }),
  );
  await wait(
    () => !!document.querySelector(".action-menu-panel:not([hidden])"),
  );
  check(
    document.activeElement?.getAttribute("role") === "menuitem",
    `library menu supports keyboard entry (focus=${document.activeElement?.outerHTML.slice(0, 280)})`,
  );
  document.activeElement!.dispatchEvent(
    new KeyboardEvent("keydown", { key: "Escape", bubbles: true }),
  );
  await wait(() => !document.querySelector(".action-menu-panel:not([hidden])"));
  check(
    document.activeElement === menuTrigger,
    "closing menu restores its trigger focus",
  );
  menuTrigger.click();
  await wait(
    () => !!document.querySelector(".action-menu-panel:not([hidden])"),
  );
  const menuBounds = document
    .querySelector(".action-menu-panel:not([hidden])")!
    .getBoundingClientRect();
  check(
    menuBounds.top >= 0 && menuBounds.bottom <= innerHeight,
    "library menu remains within the viewport",
  );
  click("Backups", document.querySelector(".action-menu-panel:not([hidden])")!);
  await wait(() => !!document.querySelector(".local-backup"));
  const backup = Array.from(document.querySelectorAll(".local-backup")).find(
    (r) => r.textContent?.includes(target),
  );
  if (!backup) throw new Error("native local recovery point missing");
  click("Review restore", backup);
  await wait(() => !!document.querySelector("dialog[open] .local-plan"));
  click("Apply changes", document.querySelector("dialog[open]")!);
  await wait(
    () =>
      document
        .querySelector("dialog[open] .modal-head")
        ?.textContent?.includes("Changes complete") ?? false,
  );
  document
    .querySelector<HTMLButtonElement>("dialog[open] .modal-head > button")!
    .click();
  await wait(() => !document.querySelector("dialog[open]"));
  check(
    (await API.ReadLocalSkill(target, "SKILL.md")).fingerprint ===
      original.fingerprint,
    "native recovery restores exact original content",
  );
  const association = await API.PlanLocalSource(
    target,
    `${source.source}/${source.namespace}/${source.name}@${source.version}`,
    false,
  );
  check(
    association.local?.differences?.length === 0 && association.local?.after,
    "exact source association requires no file replacement",
  );
  await API.ApplyPlan(association.handle);
  const attached = (await API.LocalInventory()).skills?.find(
    (i) => i.target === target,
  );
  check(
    attached?.state === "matched" &&
      attached.installation?.version === source.version,
    "associated local copy exposes its verified version",
  );
  const detached = await API.PlanLocalAction(target, "detach", "");
  await API.ApplyPlan(detached.handle);
  await API.SetLocalHidden(target, true);
  check(
    (await API.LocalInventory()).skills?.find((i) => i.target === target)
      ?.hidden,
    "hiding a skill preserves files and changes only inventory visibility",
  );
  await API.SetLocalHidden(target, false);
  const remove = await API.PlanLocalAction(target, "remove", "");
  await API.ApplyPlan(remove.handle);
  check(
    !(await API.LocalInventory()).skills?.some((i) => i.target === target),
    "local uninstall removes only the selected independent copy",
  );
}
