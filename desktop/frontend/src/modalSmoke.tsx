import { ActionMenu } from "./ActionMenu";
import { pageScroller } from "./pageScroll";
import { StrictMode, useState } from "react";
import { flushSync } from "react-dom";
import { createRoot } from "react-dom/client";
import { Modal, UIContext, type UI } from "./ui";
import { useActionFeedback } from "./actionFeedback";

const describeFailure = (error: unknown) => String(error);

// 在系统 WebView 中验证真实尺寸、滚动容器、嵌套关闭与焦点恢复。
export async function runModalSmoke(
  check: (value: unknown, label: string) => void,
) {
  const host = document.createElement("div");
  const spacer = document.createElement("div");
  spacer.style.height = "2000px";
  const scroller = pageScroller();
  const native = scroller.matches(".desktop-shell > main");
  (native ? scroller : document.body).append(host, spacer);
  const root = createRoot(host);
  const originalY = scroller.scrollTop;
  const originalBody = document.body.style.cssText;
  const originalMain = scroller.style.cssText;
  const originalRoot = document.documentElement.style.cssText;
  const ui: UI = {
    busy: false,
    error: "",
    revision: 0,
    sources: [],
    activeSource: "",
    access: "guest",
    run: (task) => task(),
    changed: () => {},
    plan: async () => false,
    preview: () => {},
    confirm: () => {},
    navigate: () => {},
    refresh: () => {},
  };
  let rejectPending: ((reason: Error) => void) | undefined;
  let unmountDialog = () => {};
  function Fixture() {
    const feedback = useActionFeedback(describeFailure);
    const [open, setOpen] = useState(false);
    const [long, setLong] = useState(false);
    const [nested, setNested] = useState(false);
    unmountDialog = () => setOpen(false);
    return (
      <UIContext.Provider
        value={{ ...ui, ...feedback, busy: feedback.pending > 0 }}
      >
        <button
          data-page-error
          onClick={() => feedback.setError("Previous page failure")}
        >
          Page error
        </button>
        {!feedback.errorScope && feedback.error && (
          <p data-page-notice>{feedback.error}</p>
        )}
        <button data-modal-trigger onClick={() => setOpen(true)}>
          Open fixture
        </button>
        {open && (
          <Modal
            title="Modal fixture"
            stable
            onClose={() => setOpen(false)}
            toolbar={
              <div>
                <button data-short onClick={() => setLong(false)}>
                  Short tab
                </button>
                <button data-long onClick={() => setLong(true)}>
                  Long tab
                </button>
                <ActionMenu label="Fixture actions">
                  <button data-nested onClick={() => setNested(true)}>
                    Nested dialog
                  </button>
                  <button data-menu-last>Another action</button>
                </ActionMenu>
                <button
                  data-fail
                  onClick={() =>
                    void feedback.run(async () => {
                      throw new Error("Dialog failure");
                    })
                  }
                >
                  Fail operation
                </button>
                <button
                  data-late
                  onClick={() =>
                    void feedback.run(
                      () =>
                        new Promise((_, reject) => {
                          rejectPending = reject;
                        }),
                    )
                  }
                >
                  Delayed failure
                </button>
              </div>
            }
          >
            {long ? (
              Array.from({ length: 60 }, (_, i) => (
                <p key={i}>Scrollable fixture row {i + 1}</p>
              ))
            ) : (
              <p>Short content</p>
            )}
            {nested && (
              <Modal title="Nested fixture" onClose={() => setNested(false)}>
                <p>Nested content</p>
              </Modal>
            )}
          </Modal>
        )}
      </UIContext.Provider>
    );
  }
  const click = (selector: string) =>
    flushSync(() => {
      const button = document.querySelector<HTMLButtonElement>(selector)!;
      button.focus({ preventScroll: true });
      button.click();
    });
  try {
    flushSync(() =>
      root.render(
        <StrictMode>
          <Fixture />
        </StrictMode>,
      ),
    );
    scroller.scrollTo(0, 120);
    const before = scroller.scrollTop;
    if (native) {
      const nav = document
        .querySelector(".navigation-shell")!
        .getBoundingClientRect();
      check(
        window.scrollY === 0 &&
          nav.top === 0 &&
          scroller.getBoundingClientRect().top >= nav.bottom,
        "desktop scroll starts below stationary navigation",
      );
    }
    click("[data-page-error]");
    click("[data-modal-trigger]");
    check(
      !document.querySelector("dialog .notice.error"),
      "new modal does not inherit an existing page error",
    );
    const dialog = document.querySelector<HTMLDialogElement>("dialog")!;
    const body = dialog.querySelector<HTMLElement>(".modal-body")!;
    const bounds = dialog.getBoundingClientRect();
    check(
      before > 0 &&
        (native || document.body.style.position === "fixed") &&
        (native
          ? scroller.style.overflow === "hidden"
          : document.body.style.top === `${-before}px`),
      "modal locks the page at its existing scroll position",
    );
    click("[data-long]");
    check(
      Math.abs(dialog.getBoundingClientRect().height - bounds.height) < 1 &&
        Math.abs(dialog.getBoundingClientRect().top - bounds.top) < 1,
      "tab content does not resize or recenter a stable modal",
    );
    check(
      body.scrollHeight > body.clientHeight &&
        getComputedStyle(body).overflowY === "auto" &&
        getComputedStyle(body).overscrollBehaviorY === "contain",
      "long modal content has an isolated bounded scroll region",
    );
    body.scrollTop = 160;
    check(
      body.scrollTop === 160 &&
        dialog.scrollTop === 0 &&
        Math.abs(
          dialog.querySelector(".modal-head")!.getBoundingClientRect().top -
            bounds.top -
            1,
        ) < 1,
      "scrolling modal content keeps its title and close control fixed",
    );
    check(
      bounds.top >= 19 && bounds.bottom <= window.innerHeight - 19,
      "modal reserves viewport edges for reachable controls",
    );
    click('[aria-label="Fixture actions"]');
    check(
      document.activeElement === document.querySelector("[data-nested]"),
      "menu inside modal receives keyboard focus",
    );
    click("[data-nested]");
    check(
      document.querySelectorAll("dialog[open]").length === 2,
      "nested modal opens above its parent",
    );
    const nested = document.querySelectorAll("dialog")[1];
    flushSync(() =>
      nested.dispatchEvent(
        new Event("cancel", { bubbles: false, cancelable: true }),
      ),
    );
    await new Promise(requestAnimationFrame);
    check(
      (native
        ? scroller.style.overflow === "hidden"
        : document.body.style.position === "fixed") &&
        document.querySelectorAll("dialog[open]").length === 1 &&
        document.activeElement ===
          document.querySelector('[aria-label="Fixture actions"]'),
      `closing nested modal keeps page locked and restores parent focus (open=${document.querySelectorAll("dialog[open]").length}, lock=${document.body.style.position}, focus=${document.activeElement?.textContent?.slice(0, 40)})`,
    );
    click("[data-short]");
    check(
      body.scrollTop === 0 &&
        Math.abs(dialog.getBoundingClientRect().height - bounds.height) < 1,
      "short tab returns to its content start without shrinking the dialog",
    );
    click(".modal-head button");
    await new Promise(requestAnimationFrame);
    check(
      scroller.scrollTop === before &&
        scroller.style.cssText === originalMain &&
        document.body.style.cssText === originalBody &&
        document.documentElement.style.cssText === originalRoot &&
        document.activeElement === host.querySelector("[data-modal-trigger]"),
      `last modal restores page position, inline styles and trigger focus (scroll=${scroller.scrollTop}/${before}, height=${scroller.scrollHeight}/${scroller.clientHeight}, main=${scroller.style.cssText === originalMain}, body=${document.body.style.cssText === originalBody}, root=${document.documentElement.style.cssText === originalRoot}, focus=${document.activeElement === host.querySelector("[data-modal-trigger]")})`,
    );
    click("[data-modal-trigger]");
    click("[data-fail]");
    await new Promise(requestAnimationFrame);
    check(
      document.querySelector("dialog .notice.error")?.textContent ===
        "Error: Dialog failure" && !host.querySelector("[data-page-notice]"),
      "dialog operation errors remain local to their owner",
    );
    click('[aria-label="Fixture actions"]');
    check(
      document.activeElement === document.querySelector("[data-nested]"),
      "menu inside modal receives keyboard focus",
    );
    click("[data-nested]");
    check(
      !document.querySelectorAll("dialog")[1].querySelector(".notice.error"),
      "nested dialog does not inherit its parent's error",
    );
    click("dialog:last-of-type .modal-head button");
    click("[data-late]");
    flushSync(unmountDialog);
    click("[data-modal-trigger]");
    rejectPending!(new Error("Late failure"));
    await new Promise(requestAnimationFrame);
    check(
      !document.querySelector("dialog .notice.error") &&
        !host.querySelector("[data-page-notice]"),
      "late failure from a closed dialog cannot leak into its replacement",
    );
  } finally {
    flushSync(() => root.unmount());
    host.remove();
    spacer.remove();
    scroller.scrollTo(0, originalY);
  }
}
