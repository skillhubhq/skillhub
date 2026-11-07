import { useState } from "react";
import { flushSync } from "react-dom";
import { createRoot } from "react-dom/client";
import { Select } from "./Select";

// 只进入 native_smoke 构建，在真实 WebView 中回归键盘、表单和顶层对话框行为。
export async function runSelectSmoke(
  check: (condition: unknown, label: string) => void,
) {
  const container = document.createElement("div");
  document.body.append(container);
  const root = createRoot(container);
  const key = async (button: HTMLElement, value: string) => {
    flushSync(() => {
      button.dispatchEvent(
        new KeyboardEvent("keydown", {
          key: value,
          bubbles: true,
          cancelable: true,
        }),
      );
    });
  };
  function Fixture() {
    const [value, setValue] = useState("");
    const [groups, setGroups] = useState<string[]>([]);
    return (
      <dialog style={{ width: 360 }}>
        <form onSubmit={(event) => event.preventDefault()}>
          <label className="field">
            <span>Test choice</span>
            <Select
              required
              value={value}
              onChange={(e) => setValue(e.target.value)}
            >
              <option value="">Choose…</option>
              <option value="alpha">Alpha</option>
              <option value="disabled" disabled>
                Disabled
              </option>
              <option value="beta">Beta</option>
            </Select>
          </label>
          <label className="field">
            <span>Groups</span>
            <Select
              multiple
              value={groups}
              onChange={(e) =>
                setGroups(e.target.selectedOptions.map((o) => o.value))
              }
            >
              <option value="alpha">Alpha</option>
              <option value="beta">Beta</option>
            </Select>
          </label>
          <fieldset disabled>
            <Select
              aria-label="Disabled choice"
              value="alpha"
              onChange={() => {
                throw new Error("disabled selection changed");
              }}
            >
              <option value="alpha">Alpha</option>
            </Select>
          </fieldset>
          <button type="submit">Continue</button>
        </form>
      </dialog>
    );
  }
  try {
    // 测试直接读取 DOM，必须等待 React 提交；固定延时无法保证繁忙运行器上已挂载。
    flushSync(() => root.render(<Fixture />));
    const dialog = container.querySelector("dialog")!;
    dialog.showModal();
    const form = dialog.querySelector("form")!;
    const buttons = dialog.querySelectorAll<HTMLButtonElement>(".ui-select");
    check(
      !dialog.querySelector("select"),
      "custom choices render without native select controls",
    );
    flushSync(() => form.requestSubmit());
    check(
      buttons[0].getAttribute("aria-invalid") === "true" &&
        document.activeElement === buttons[0],
      "required choice prevents submit and focuses its themed trigger",
    );
    await key(buttons[0], "ArrowDown");
    check(
      !!dialog.querySelector(".ui-select-menu") &&
        buttons[0].getAttribute("aria-label") === "Test choice",
      "choice menu stays inside modal top layer and inherits its field label",
    );
    await key(buttons[0], "ArrowDown");
    await key(buttons[0], "ArrowDown");
    await key(buttons[0], "Enter");
    check(
      buttons[0].value === "beta" &&
        !dialog.querySelector(".ui-select-menu") &&
        form.checkValidity(),
      "keyboard selection skips disabled options and satisfies required validation",
    );
    await key(buttons[0], " ");
    await key(buttons[0], "a");
    await key(buttons[0], "Enter");
    check(
      buttons[0].value === "alpha",
      "type-ahead finds an option without committing until Enter",
    );
    await key(buttons[0], "ArrowDown");
    await key(buttons[0], "End");
    await key(buttons[0], "Escape");
    check(
      dialog.open &&
        buttons[0].value === "alpha" &&
        document.activeElement === buttons[0],
      "Escape cancels selection without dismissing the parent dialog or losing focus",
    );
    buttons[1].focus();
    await key(buttons[1], " ");
    await key(buttons[1], "Enter");
    await key(buttons[1], "ArrowDown");
    await key(buttons[1], " ");
    check(
      buttons[1].value === "alpha,beta" &&
        dialog.querySelectorAll('[role="option"][aria-selected="true"]')
          .length === 2,
      "multiple choices toggle without modifier keys and stay open",
    );
    await key(buttons[1], " ");
    check(
      buttons[1].value === "alpha",
      "multiple choice can be deselected independently",
    );
    const rect = dialog
      .querySelector(".ui-select-menu")!
      .getBoundingClientRect();
    check(
      rect.left >= 0 &&
        rect.right <= innerWidth &&
        rect.top >= 0 &&
        rect.bottom <= innerHeight,
      "choice popup stays within the WebView viewport",
    );
    await key(buttons[1], "Tab");
    check(
      !dialog.querySelector(".ui-select-menu"),
      "Tab dismisses the choice menu without trapping focus",
    );
    flushSync(() => buttons[2].click());
    check(
      !dialog.querySelector(".ui-select-menu"),
      "disabled fieldset prevents opening a choice menu",
    );
    dialog.close();
  } finally {
    root.unmount();
    container.remove();
  }
}
