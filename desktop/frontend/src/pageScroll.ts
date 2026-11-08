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
