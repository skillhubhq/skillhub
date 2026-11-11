import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";
import { Icon } from "./Icon";
import "./action-menu.css";

// 次要操作使用同一浮层；子组件持续挂载，打开的对话框不受菜单关闭影响。
export function ActionMenu({
  label,
  children,
  iconOnly = false,
}: {
  label: string;
  children: ReactNode;
  iconOnly?: boolean;
}) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [position, setPosition] = useState({ top: 0, left: 0, maxHeight: 320 });
  const items = () =>
    Array.from(
      panel.current?.querySelectorAll<HTMLElement>(
        "button:not(:disabled), a[href]",
      ) ?? [],
    ).filter((item) => !item.closest("[hidden]"));
  function close(restore = false) {
    if (restore) trigger.current?.focus({ preventScroll: true });
    setOpen(false);
  }
  useLayoutEffect(() => {
    if (!open) return;
    const place = () => {
      const rect = trigger.current!.getBoundingClientRect();
      const view = window.visualViewport;
      const width = view?.width ?? innerWidth,
        height = view?.height ?? innerHeight;
      const x = view?.offsetLeft ?? 0,
        y = view?.offsetTop ?? 0;
      const below = height + y - rect.bottom - 12,
        above = rect.top - y - 12;
      const upwards =
        below < Math.min(panel.current?.scrollHeight ?? 320, 320) &&
        above > below;
      const maxHeight = Math.max(48, Math.min(400, upwards ? above : below));
      const size = Math.min(
        panel.current?.scrollHeight ?? maxHeight,
        maxHeight,
      );
      const menuWidth = Math.min(260, width - 24);
      setPosition({
        top: upwards ? rect.top - size - 6 : rect.bottom + 6,
        left: Math.max(
          x + 12,
          Math.min(rect.right - menuWidth, x + width - menuWidth - 12),
        ),
        maxHeight,
      });
    };
    const outside = (event: PointerEvent) => {
      if (
        !panel.current?.contains(event.target as Node) &&
        !trigger.current?.contains(event.target as Node)
      )
        close();
    };
    const scroll = (event: Event) => {
      if (!panel.current?.contains(event.target as Node)) place();
    };
    place();
    for (const item of items()) {
      item.setAttribute("role", "menuitem");
      item.tabIndex = -1;
    }
    items()[0]?.focus({ preventScroll: true });
    document.addEventListener("pointerdown", outside);
    window.addEventListener("resize", place);
    window.addEventListener("scroll", scroll, true);
    viewListen("add", place);
    return () => {
      document.removeEventListener("pointerdown", outside);
      window.removeEventListener("resize", place);
      window.removeEventListener("scroll", scroll, true);
      viewListen("remove", place);
    };
  }, [open]);
  return (
    <>
      <button
        ref={trigger}
        className={`action-menu-trigger${iconOnly ? " icon-only" : ""}`}
        type="button"
        aria-label={label}
        title={iconOnly ? label : undefined}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => (open ? close(true) : setOpen(true))}
        onKeyDown={(event) => {
          if (["ArrowDown", "ArrowUp"].includes(event.key)) {
            event.preventDefault();
            setOpen(true);
          }
        }}
      >
        {!iconOnly && <span>{label}</span>}
        <Icon name="more" />
      </button>
      {createPortal(
        <div
          id={id}
          ref={panel}
          role="menu"
          aria-label={label}
          hidden={!open}
          className="action-menu-panel"
          style={position}
          onBlur={(event) => {
            if (
              open &&
              !event.currentTarget.contains(event.relatedTarget as Node)
            )
              close();
          }}
          onClickCapture={(event) => {
            if (!open || !panel.current?.contains(event.target as Node)) return;
            const button = (event.target as Element).closest("button,a[href]");
            if (button && !button.matches(":disabled")) close(true);
          }}
          onKeyDown={(event) => {
            if (!open) return;
            const list = items(),
              index = list.indexOf(document.activeElement as HTMLElement);
            if (event.key === "Escape") {
              event.preventDefault();
              event.stopPropagation();
              close(true);
            } else if (event.key === "Tab") {
              close(true);
            } else if (
              ["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)
            ) {
              event.preventDefault();
              const next =
                event.key === "Home"
                  ? 0
                  : event.key === "End"
                    ? list.length - 1
                    : (index +
                        (event.key === "ArrowDown" ? 1 : -1) +
                        list.length) %
                      list.length;
              list[next]?.focus({ preventScroll: true });
              const el = list[next],
                box = panel.current;
              if (el && box) {
                if (el.offsetTop < box.scrollTop) box.scrollTop = el.offsetTop;
                else if (
                  el.offsetTop + el.offsetHeight >
                  box.scrollTop + box.clientHeight
                )
                  box.scrollTop =
                    el.offsetTop + el.offsetHeight - box.clientHeight;
              }
            }
          }}
        >
          {children}
        </div>,
        trigger.current?.closest("dialog") ?? document.body,
      )}
    </>
  );
}
function viewListen(action: "add" | "remove", listener: () => void) {
  const view = window.visualViewport;
  if (action === "add") {
    view?.addEventListener("resize", listener);
    view?.addEventListener("scroll", listener);
  } else {
    view?.removeEventListener("resize", listener);
    view?.removeEventListener("scroll", listener);
  }
}
