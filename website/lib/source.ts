import { docs, docsZh } from "../.source/server";
import { loader } from "fumadocs-core/source";

export const source = loader({ baseUrl: "/", source: docs.toFumadocsSource() });

export const sourceZh = loader({
  baseUrl: "/zh",
  source: docsZh.toFumadocsSource(),
});
export function guide(slug: string[] = []) {
  const zh = slug[0] === "zh";
  return {
    locale: zh ? ("zh" as const) : ("en" as const),
    source: zh ? sourceZh : source,
    slug: zh ? slug.slice(1) : slug,
  };
}
