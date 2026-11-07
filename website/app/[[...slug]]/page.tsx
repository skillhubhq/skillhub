import { guide, source, sourceZh } from "../../lib/source";
import {
  DocsBody,
  DocsDescription,
  DocsPage,
  DocsTitle,
} from "fumadocs-ui/page";
import defaultMdxComponents from "fumadocs-ui/mdx";
import { notFound } from "next/navigation";

import { DocsChrome } from "../DocsChrome";

export const dynamicParams = false;
export function generateStaticParams() {
  return [
    ...source.generateParams(),
    ...sourceZh.generateParams().map((p) => ({ slug: ["zh", ...p.slug] })),
  ];
}
export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug?: string[] }>;
}) {
  const current = guide((await params).slug);
  const page = current.source.getPage(current.slug);
  return page
    ? { title: page.data.title, description: page.data.description }
    : {};
}
export default async function Page({
  params,
}: {
  params: Promise<{ slug?: string[] }>;
}) {
  const current = guide((await params).slug);
  const page = current.source.getPage(current.slug);
  if (!page) notFound();
  const Body = page.data.body;
  return (
    <DocsChrome locale={current.locale} tree={current.source.pageTree}>
      <article
        className="contents"
        lang={current.locale === "zh" ? "zh-CN" : "en"}
      >
        <DocsPage toc={page.data.toc} full={page.data.full}>
          <DocsTitle>{page.data.title}</DocsTitle>
          <DocsDescription>{page.data.description}</DocsDescription>
          <DocsBody>
            <Body components={defaultMdxComponents} />
          </DocsBody>
        </DocsPage>
      </article>
    </DocsChrome>
  );
}
