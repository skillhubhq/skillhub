import { StrictMode, useState } from "react";
import { flushSync } from "react-dom";
import { createRoot } from "react-dom/client";
import { Modal, UIContext, type UI } from "./ui";

// 在系统 WebView 中验证真实尺寸、滚动容器、嵌套关闭与焦点恢复。
export async function runModalSmoke(
  check: (value: unknown, label: string) => void,
) {
  const host = document.createElement("div");
  const spacer = document.createElement("div");
  spacer.style.height = "2000px";
  document.body.append(host, spacer);
  const root = createRoot(host);
  const originalY = window.scrollY;
  const originalBody = document.body.style.cssText;
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
    plan: () => {},
    preview: () => {},
    confirm: () => {},
    navigate: () => {},
    refresh: () => {},
  };
  function Fixture() {
    const [open, setOpen] = useState(false);
    const [long, setLong] = useState(false);
    const [nested, setNested] = useState(false);
    return (
      <UIContext.Provider value={ui}>
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
                <button data-nested onClick={() => setNested(true)}>
                  Nested dialog
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
      const button = host.querySelector<HTMLButtonElement>(selector)!;
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
    window.scrollTo(0, 120);
    const before = window.scrollY;
    click("[data-modal-trigger]");
    const dialog = host.querySelector<HTMLDialogElement>("dialog")!;
    const body = dialog.querySelector<HTMLElement>(".modal-body")!;
    const bounds = dialog.getBoundingClientRect();
    check(
      before > 0 &&
        document.body.style.position === "fixed" &&
        document.body.style.top === `${-before}px`,
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
    click("[data-nested]");
    check(
      host.querySelectorAll("dialog[open]").length === 2,
      "nested modal opens above its parent",
    );
    const nested = host.querySelectorAll("dialog")[1];
    flushSync(() =>
      nested.dispatchEvent(
        new Event("cancel", { bubbles: false, cancelable: true }),
      ),
    );
    await new Promise(requestAnimationFrame);
    check(
      document.body.style.position === "fixed" &&
        host.querySelectorAll("dialog[open]").length === 1 &&
        document.activeElement === host.querySelector("[data-nested]"),
      `closing nested modal keeps page locked and restores parent focus (open=${host.querySelectorAll("dialog[open]").length}, lock=${document.body.style.position}, focus=${document.activeElement?.textContent?.slice(0,40)})`,
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
      window.scrollY === before &&
        document.body.style.cssText === originalBody &&
        document.documentElement.style.cssText === originalRoot &&
        document.activeElement === host.querySelector("[data-modal-trigger]"),
      "last modal restores page position, inline styles and trigger focus",
    );
  } finally {
    flushSync(() => root.unmount());
    host.remove();
    spacer.remove();
    window.scrollTo(0, originalY);
  }
}
