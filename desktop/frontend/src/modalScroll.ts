let locks = 0;
let restore: (() => void) | undefined;

// 原生 dialog 的模态性不会阻止背景滚动；计数锁允许安装确认等嵌套弹窗安全共存。
export function lockModalScroll() {
  if (locks++ === 0) {
    const root = document.documentElement;
    const body = document.body;
    const x = window.scrollX;
    const y = window.scrollY;
    const gap = Math.max(0, window.innerWidth - root.clientWidth);
    const saved = [
      [root, "overflow"],
      [root, "scroll-behavior"],
      [body, "position"],
      [body, "top"],
      [body, "left"],
      [body, "width"],
      [body, "padding-right"],
    ].map(([element, name]) => {
      const el = element as HTMLElement;
      const prop = name as string;
      return {
        el,
        prop,
        value: el.style.getPropertyValue(prop),
        priority: el.style.getPropertyPriority(prop),
      };
    });
    const padding = parseFloat(getComputedStyle(body).paddingRight) || 0;
    root.style.overflow = "hidden";
    body.style.position = "fixed";
    body.style.top = `${-y}px`;
    body.style.left = `${-x}px`;
    body.style.width = "100%";
    if (gap) body.style.paddingRight = `${padding + gap}px`;
    restore = () => {
      for (const { el, prop, value, priority } of saved)
        if (prop !== "scroll-behavior") {
          if (value) el.style.setProperty(prop, value, priority);
          else el.style.removeProperty(prop);
        }
      root.style.scrollBehavior = "auto";
      window.scrollTo(x, y);
      const original = saved.find((item) => item.prop === "scroll-behavior")!;
      if (original.value)
        root.style.setProperty(
          original.prop,
          original.value,
          original.priority,
        );
      else root.style.removeProperty(original.prop);
    };
  }
  let released = false;
  return () => {
    if (released) return;
    released = true;
    if (--locks === 0) {
      restore?.();
      restore = undefined;
    }
  };
}
