import { flushSync } from "react-dom";
import { createRoot } from "react-dom/client";
import { useResource } from "./community-ui";

// 延迟响应验证后台刷新保留内容，切换身份和请求失败则立即隔离旧数据。
export async function runResourceSmoke(
  check: (condition: unknown, label: string) => void,
) {
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  const pending: Array<{
    resolve: (value: string) => void;
    reject: (error: Error) => void;
  }> = [];
  function Fixture({ source, revision }: { source: string; revision: number }) {
    const resource = useResource(
      () =>
        new Promise<string>((resolve, reject) =>
          pending.push({ resolve, reject }),
        ),
      [source],
      revision,
    );
    return resource.error ? (
      <p role="alert">{resource.error}</p>
    ) : resource.loading ? (
      <p role="status">Loading</p>
    ) : (
      <article>{resource.value}</article>
    );
  }
  const settle = async (ready: () => boolean) => {
    const deadline = Date.now() + 5000;
    while (!ready()) {
      if (Date.now() >= deadline)
        throw new Error(`Resource fixture did not settle: ${ready.toString()}`);
      await new Promise<void>((resolve) => setTimeout(resolve, 20));
    }
  };
  try {
    flushSync(() => root.render(<Fixture source="first" revision={0} />));
    pending.shift()!.resolve("Current account");
    await settle(
      () =>
        container.querySelector("article")?.textContent === "Current account",
    );
    const content = container.querySelector("article");
    check(
      content?.textContent === "Current account",
      "resource initial content loads",
    );
    flushSync(() => root.render(<Fixture source="first" revision={1} />));
    check(
      container.querySelector("article") === content &&
        !container.querySelector('[role="status"]'),
      "background refresh preserves loaded content and its layout",
    );
    pending.shift()!.resolve("Updated account");
    await settle(() => content?.textContent === "Updated account");
    check(
      content?.textContent === "Updated account",
      "background refresh commits new content in place",
    );
    flushSync(() => root.render(<Fixture source="second" revision={1} />));
    check(
      !container.querySelector("article") &&
        !!container.querySelector('[role="status"]'),
      "switching sources hides the previous account immediately",
    );
    pending.shift()!.resolve("Second account");
    await settle(
      () =>
        container.querySelector("article")?.textContent === "Second account",
    );
    flushSync(() => root.render(<Fixture source="second" revision={2} />));
    pending.shift()!.reject(new Error("Session expired"));
    await settle(
      () =>
        container.querySelector('[role="alert"]')?.textContent ===
        "Session expired",
    );
    check(
      !container.querySelector("article") &&
        container.querySelector('[role="alert"]')?.textContent ===
          "Session expired",
      "failed refresh clears stale authenticated content",
    );
  } finally {
    flushSync(() => root.unmount());
    container.remove();
  }
}
