"use client";
import { RootProvider } from "fumadocs-ui/provider/next";
import { DocsLayout } from "fumadocs-ui/layouts/docs";
import type { Root } from "fumadocs-core/page-tree";
import type { ReactNode } from "react";
import { DocsHeader } from "./DocsHeader";
export function DocsChrome({
  locale,
  tree,
  children,
}: {
  locale: "en" | "zh";
  tree: Root;
  children: ReactNode;
}) {
  return (
    <RootProvider
      theme={{
        storageKey: "skillhub.theme",
        defaultTheme: "system",
        enableSystem: true,
      }}
      search={{
        options: {
          type: "static",
          api: locale === "zh" ? "/docs/api/search-zh" : "/docs/api/search",
        },
      }}
      i18n={{
        locale,
        translations:
          locale === "zh"
            ? {
                "Search(search trigger)": "搜索文档",
                "Search(search dialog)": "搜索文档",
                "Open Search(search trigger)(aria-label)": "搜索文档",
                "Close Search(search dialog)(aria-label)": "关闭搜索",
                "No results found(search dialog)": "没有找到结果",
                "Search failed(search dialog)": "搜索失败",
                "Search failed, showing results for {query}(search dialog)":
                  "搜索失败，显示 {query} 的结果",
                "On this page(table of contents)": "本页内容",
                "Table of Contents(inline table of contents)": "本页目录",
                "No Headings(table of contents)": "暂无标题",
                "Previous Page(pagination)": "上一页",
                "Next Page(pagination)": "下一页",
                "Last updated on(page footer)": "最后更新",
                "Open Sidebar(sidebar)(aria-label)": "打开目录",
                "Close Sidebar(sidebar)(aria-label)": "关闭目录",
                "Close Sidebar(aria-label)": "关闭目录",
                "Collapse Sidebar(sidebar)(aria-label)": "收起目录",
                "Hide Sidebar(sidebar)": "隐藏目录",
                "Show Sidebar(sidebar)": "显示目录",
                "Skip to Content(skip link)": "跳至正文",
                "Copy Text(code block)(aria-label)": "复制代码",
                "Copied Text(code block)(aria-label)": "已复制",
                "Copy Anchor Link(heading anchor)(aria-label)": "复制章节链接",
                "Copied Anchor Link(heading anchor)(aria-label)":
                  "已复制章节链接",
                "Toggle Menu(mobile menu)(aria-label)": "切换菜单",
                "Edit on GitHub(edit page)": "在 GitHub 编辑",
                displayName: "简体中文",
              }
            : undefined,
      }}
    >
      <DocsHeader locale={locale} />
      <DocsLayout
        tree={tree}
        nav={{ title: locale === "zh" ? "使用文档" : "Documentation", url: locale === "zh" ? "/zh/" : "/" }}
        themeSwitch={{ enabled: false }}
        sidebar={{ collapsible: false }}
      >
        {children}
      </DocsLayout>
    </RootProvider>
  );
}
