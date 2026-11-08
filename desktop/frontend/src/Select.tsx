import {
  Children,
  Fragment,
  isValidElement,
  useEffect,
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
  type AriaAttributes,
} from "react";
import { createPortal } from "react-dom";
import { t } from "./i18n";

type Choice = { value: string; label: string; disabled: boolean };
type Selection = {
  target: { value: string; selectedOptions: { value: string }[] };
};
type Props = AriaAttributes & {
  children: ReactNode;
  value: string | number | readonly string[];
  onChange: (selection: Selection) => void;
  multiple?: boolean;
  required?: boolean;
  disabled?: boolean;
  className?: string;
  triggerContent?: ReactNode;
};
function text(children: ReactNode): string {
  return Children.toArray(children)
    .map((child) =>
      isValidElement<{ children?: ReactNode }>(child)
        ? text(child.props.children)
        : String(child),
    )
    .join("");
}
function choices(children: ReactNode): Choice[] {
  return Children.toArray(children).flatMap((child): Choice[] => {
    if (
      !isValidElement<{
        value?: string | number;
        children?: ReactNode;
        disabled?: boolean;
      }>(child)
    )
      return [];
    if (child.type === Fragment) return choices(child.props.children);
    if (child.type !== "option") return [];
    return [
      {
        value: String(child.props.value ?? text(child.props.children)),
        label: text(child.props.children),
        disabled: !!child.props.disabled,
      },
    ];
  });
}

// 所有平台共用选择交互；菜单挂在当前对话框内，避免被顶层 modal 遮挡或变成 inert。
export function Select({
  children,
  value,
  onChange,
  multiple = false,
  required = false,
  disabled = false,
  className = "",
  triggerContent,
  ...aria
}: Props) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [cursor, setCursor] = useState(0);
  const [invalid, setInvalid] = useState(false);
  const [label, setLabel] = useState("");
  const [position, setPosition] = useState({
    top: 0,
    left: 0,
    width: 200,
    maxHeight: 280,
  });
  const search = useRef({ text: "", at: 0 });
  const items = choices(children);
  const selected = Array.isArray(value) ? value.map(String) : [String(value)];
  const chosen = items.filter((item) => selected.includes(item.value));
  const selectionKey = selected.join("\u0000");
  const itemKey = JSON.stringify(items);
  const enabled = items
    .map((item, index) => (item.disabled ? -1 : index))
    .filter((index) => index >= 0);
  const hasValue = chosen.some((item) => item.value !== "");
  useLayoutEffect(() => {
    setLabel(
      trigger.current?.labels?.[0]?.querySelector(":scope > span")
        ?.textContent ?? "",
    );
  }, [children]);
  useEffect(() => {
    if (hasValue) setInvalid(false);
  }, [hasValue]);
  useEffect(() => {
    if (disabled) setOpen(false);
  }, [disabled]);
  useEffect(() => {
    if (!open) return;
    setCursor((current) =>
      enabled.includes(current) ? current : (enabled[0] ?? -1),
    );
  }, [open, itemKey]);
  useLayoutEffect(() => {
    if (!open || !trigger.current) return;
    const update = () => {
      const rect = trigger.current!.getBoundingClientRect();
      const viewport = window.visualViewport;
      const width = viewport?.width ?? window.innerWidth;
      const height = viewport?.height ?? window.innerHeight;
      const x = viewport?.offsetLeft ?? 0,
        y = viewport?.offsetTop ?? 0;
      const below = height + y - rect.bottom - 12,
        above = rect.top - y - 12;
      const upwards =
        below < Math.min(280, menu.current?.scrollHeight ?? 280) &&
        above > below;
      const maxHeight = Math.max(48, Math.min(280, upwards ? above : below));
      const menuHeight = Math.min(
        menu.current?.scrollHeight ?? maxHeight,
        maxHeight,
      );
      const menuWidth = Math.min(Math.max(rect.width, 200), width - 16);
      setPosition({
        top: upwards ? rect.top - menuHeight - 6 : rect.bottom + 6,
        left: Math.max(x + 8, Math.min(rect.left, x + width - menuWidth - 8)),
        width: menuWidth,
        maxHeight,
      });
    };
    const outside = (event: PointerEvent) => {
      if (
        !trigger.current?.contains(event.target as Node) &&
        !menu.current?.contains(event.target as Node)
      )
        setOpen(false);
    };
    const scroll = (event: Event) => {
      if (!menu.current?.contains(event.target as Node)) update();
    };
    update();
    document.addEventListener("pointerdown", outside);
    window.addEventListener("resize", update);
    window.addEventListener("scroll", scroll, true);
    window.visualViewport?.addEventListener("resize", update);
    window.visualViewport?.addEventListener("scroll", update);
    return () => {
      document.removeEventListener("pointerdown", outside);
      window.removeEventListener("resize", update);
      window.removeEventListener("scroll", scroll, true);
      window.visualViewport?.removeEventListener("resize", update);
      window.visualViewport?.removeEventListener("scroll", update);
    };
  }, [open, itemKey, selectionKey]);
  useEffect(() => {
    const option = menu.current?.querySelector<HTMLElement>(
      `[data-index="${cursor}"]`,
    );
    if (!option || !menu.current) return;
    // 仅滚动菜单自身，避免浏览器顺带滚动页面和发布对话框。
    const box = menu.current;
    if (option.offsetTop < box.scrollTop) box.scrollTop = option.offsetTop;
    if (
      option.offsetTop + option.offsetHeight >
      box.scrollTop + box.clientHeight
    )
      box.scrollTop = option.offsetTop + option.offsetHeight - box.clientHeight;
  }, [cursor, open]);
  const show = (last = false) => {
    if (disabled || trigger.current?.matches(":disabled")) return;
    trigger.current?.focus({ preventScroll: true });
    const index = items.findIndex(
      (item) => selected.includes(item.value) && !item.disabled,
    );
    setCursor(
      index >= 0 ? index : ((last ? enabled.at(-1) : enabled[0]) ?? -1),
    );
    search.current = { text: "", at: 0 };
    setOpen(true);
  };
  const choose = (index: number) => {
    const item = items[index];
    if (
      !item ||
      item.disabled ||
      disabled ||
      trigger.current?.matches(":disabled")
    )
      return;
    const values = multiple
      ? selected.includes(item.value)
        ? selected.filter((v) => v !== item.value)
        : [...selected, item.value]
      : [item.value];
    onChange({
      target: {
        value: values[0] ?? "",
        selectedOptions: values.map((value) => ({ value })),
      },
    });
    setInvalid(required && !values.some(Boolean));
    if (!multiple) setOpen(false);
  };
  return (
    <>
      <button
        ref={trigger}
        type="button"
        value={selected.join(",")}
        className={`ui-select ${className}`}
        disabled={disabled}
        {...aria}
        aria-label={aria["aria-label"] || label || undefined}
        role="combobox"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-controls={open ? `${id}-menu` : undefined}
        aria-activedescendant={
          open && items[cursor] ? `${id}-${cursor}` : undefined
        }
        aria-required={required || undefined}
        aria-invalid={invalid || undefined}
        aria-describedby={
          [aria["aria-describedby"], invalid ? `${id}-error` : ""]
            .filter(Boolean)
            .join(" ") || undefined
        }
        onClick={() => (open ? setOpen(false) : show())}
        onBlur={() => setOpen(false)}
        onKeyDown={(event) => {
          if (event.key === "Escape" && open) {
            event.preventDefault();
            event.stopPropagation();
            setOpen(false);
            return;
          }
          if (event.key === "Tab") {
            setOpen(false);
            return;
          }
          if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
            event.preventDefault();
            if (!open) {
              show(event.key === "ArrowUp" || event.key === "End");
              if (event.key === "Home") setCursor(enabled[0] ?? -1);
              if (event.key === "End") setCursor(enabled.at(-1) ?? -1);
              return;
            }
            if (!enabled.length) return;
            const at = enabled.indexOf(cursor);
            setCursor(
              event.key === "Home"
                ? enabled[0]
                : event.key === "End"
                  ? enabled.at(-1)!
                  : enabled[
                      Math.max(
                        0,
                        Math.min(
                          enabled.length - 1,
                          at + (event.key === "ArrowDown" ? 1 : -1),
                        ),
                      )
                    ],
            );
            return;
          }
          if (event.key === "Enter" || event.key === " ") {
            event.preventDefault();
            if (open) choose(cursor);
            else show();
            return;
          }
          if (
            event.key.length === 1 &&
            !event.altKey &&
            !event.ctrlKey &&
            !event.metaKey
          ) {
            event.preventDefault();
            const key = event.key.toLocaleLowerCase();
            const prefix =
              Date.now() - search.current.at < 700
                ? search.current.text + key
                : key;
            search.current = { text: prefix, at: Date.now() };
            const query = [...prefix].every((c) => c === key) ? key : prefix;
            const order = [
              ...enabled.filter((i) => i > cursor),
              ...enabled.filter((i) => i <= cursor),
            ];
            const match = order.find((i) =>
              items[i].label.toLocaleLowerCase().startsWith(query),
            );
            if (!open) setOpen(true);
            if (match !== undefined) setCursor(match);
          }
        }}
      >
        <span className="ui-select-value">
          {triggerContent ??
            (chosen.length
              ? chosen.map((item) => item.label).join(", ")
              : t("Choose…", "请选择…"))}
        </span>
        <svg
          className="ui-select-chevron"
          viewBox="0 0 16 16"
          width="16"
          height="16"
          aria-hidden="true"
        >
          <path
            d="m4 6 4 4 4-4"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.5"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </button>
      {required && (
        <input
          className="ui-select-validation"
          aria-hidden="true"
          tabIndex={-1}
          required
          disabled={disabled}
          value={hasValue ? "selected" : ""}
          onChange={() => {}}
          onInvalid={(event) => {
            event.preventDefault();
            setInvalid(true);
            trigger.current?.focus();
          }}
        />
      )}
      {invalid && (
        <small id={`${id}-error`} className="ui-select-error">
          {t("Choose an option to continue.", "请选择后继续。")}
        </small>
      )}
      {open &&
        createPortal(
          <div
            ref={menu}
            id={`${id}-menu`}
            className="ui-select-menu"
            role="listbox"
            aria-label={aria["aria-label"] || label || undefined}
            aria-multiselectable={multiple || undefined}
            style={position}
            onPointerDown={(event) => event.preventDefault()}
            onMouseDown={(event) => event.preventDefault()}
          >
            {items.map((item, index) => (
              <div
                key={item.value}
                id={`${id}-${index}`}
                role="option"
                data-value={item.value}
                data-index={index}
                aria-selected={selected.includes(item.value)}
                aria-disabled={item.disabled || undefined}
                className={`ui-select-option ${index === cursor ? "highlighted" : ""}`}
                onPointerMove={() => {
                  if (!item.disabled) setCursor(index);
                }}
                onClick={(event) => {
                  event.preventDefault();
                  event.stopPropagation();
                  choose(index);
                }}
              >
                <span>{item.label}</span>
                <span
                  className={`ui-select-check ${multiple ? "multiple" : ""}`}
                  aria-hidden="true"
                >
                  {selected.includes(item.value) ? "✓" : ""}
                </span>
              </div>
            ))}
            {!items.length && (
              <div className="ui-select-empty">
                {t("No options available", "暂无可选项")}
              </div>
            )}
          </div>,
          trigger.current?.closest("dialog") ?? document.body,
        )}
    </>
  );
}
