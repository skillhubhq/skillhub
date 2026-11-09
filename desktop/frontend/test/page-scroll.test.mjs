import test from "node:test";
import assert from "node:assert/strict";
import { revealPageStart } from "../src/pageScroll.ts";

test("new document starts respect desktop, sticky Web and mobile scroll boundaries", () => {
  const oldDocument = globalThis.document,
    oldWindow = globalThis.window,
    oldStyle = globalThis.getComputedStyle;
  try {
    for (const [desktop, position, boundary] of [
      [true, "sticky", 130],
      [false, "sticky", 170],
      [false, "static", 0],
    ]) {
      const calls = [];
      const scroller = {
        scrollTop: 500,
        matches: () => desktop,
        getBoundingClientRect: () => ({ top: 130, bottom: 800 }),
        scrollTo: (v) => calls.push(v),
      };
      const header = { getBoundingClientRect: () => ({ bottom: 170 }) };
      globalThis.document = {
        scrollingElement: scroller,
        querySelector: (q) =>
          q.includes("main") ? (desktop ? scroller : null) : header,
      };
      globalThis.window = { innerHeight: 800 };
      globalThis.getComputedStyle = () => ({ position });
      revealPageStart({
        getBoundingClientRect: () => ({ top: boundary + 20 }),
      });
      assert.equal(calls.length, 0, "visible content must not jump");
      revealPageStart({ getBoundingClientRect: () => ({ top: -30 }) });
      assert.equal(calls[0].top, 500 - 30 - boundary - 12);
      revealPageStart({ getBoundingClientRect: () => ({ top: 900 }) });
      assert.equal(calls[1].top, 500 + 900 - boundary - 12);
    }
  } finally {
    globalThis.document = oldDocument;
    globalThis.window = oldWindow;
    globalThis.getComputedStyle = oldStyle;
  }
});
