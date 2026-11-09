// App 的正文滚动独立于导航；Web 使用浏览器文档滚动。
export function pageScroller(): HTMLElement {
  return (
    document.querySelector<HTMLElement>(".desktop-shell > main") ??
    (document.scrollingElement as HTMLElement)
  );
}
export function scrollPageToTop() {
  pageScroller().scrollTo({ top: 0, left: 0, behavior: "instant" });
}

// 仅在新内容起点已离开可视区时回位；导航高度随语言与窗口宽度变化。
export function revealPageStart(element: HTMLElement, force = false) {
  const scroller = pageScroller();
  const header = document.querySelector<HTMLElement>(".navigation-shell");
  const desktop = scroller.matches(".desktop-shell > main");
  const top = desktop
    ? scroller.getBoundingClientRect().top
    : header && getComputedStyle(header).position === "sticky"
      ? Math.max(0, header.getBoundingClientRect().bottom)
      : 0;
  const bottom = desktop
    ? scroller.getBoundingClientRect().bottom
    : window.innerHeight;
  const rect = element.getBoundingClientRect();
  if (force || rect.top < top || rect.top >= bottom) {
    scroller.scrollTo({
      top: Math.max(0, scroller.scrollTop + rect.top - top - 12),
      behavior: "instant",
    });
  }
}
