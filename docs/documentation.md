# Documentation website

SkillHub serves its official guides at `/docs/` alongside the community. The documentation is public software documentation, including on private Hubs. It contains no Hub accounts, packages, configuration values or runtime data.

## Authoring

Edit the canonical Markdown in `docs/`, `desktop/README.md` or `sdk/typescript/README.md`. The website build maps these files into a searchable Fumadocs sidebar; do not edit generated files in `website/content/`. Maintain the corresponding `docs/zh/SLUG.md` page for every guide. Chinese guide links are relative to their actual Markdown files, so they also work on GitHub; the generator maps them to Chinese routes. Use `[#english-heading-id]` on translated headings when preserving an anchor. Keep command names, fields and protocol identifiers unchanged.

The website uses Next.js, Fumadocs, MDX and Tailwind. It reuses the community header structure and brand styles, reads site identity through the generated Connect client, and shares the device theme preference with the main site. Account information is fetched only in the browser and never embedded in the export. It exports static HTML and a local search index. The Hub does not require Node.js at runtime. Documentation is available in English at `/docs/` and Simplified Chinese at `/docs/zh/`, with matching navigation and separate local search indexes. English is the default. The language menu shares the main site preference and keeps the corresponding page. Matching heading IDs preserve section links.

```sh
npm --prefix sdk/typescript ci --ignore-scripts
npm --prefix website ci --ignore-scripts
npm --prefix website run dev
```

Open `http://localhost:3001/docs/`. To build everything for the Hub:

```sh
make web
go build -o bin/skillhub-server ./cmd/skillhub-server
```

`website/scripts/prepare.mjs` owns sidebar ordering and the explicit source list. Add a page there when introducing a new guide. Relative guide links are mapped to documentation routes; other repository links point to the public GitHub repository. The build checks generated pages, links and search assets.

## Hosting

The default is the same origin as the Hub, with `/docs/` reserved for documentation. A separately hosted official site can serve the static contents of `website/out/` at `/docs/`, preserving trailing-slash directory indexes. Route `/docs/_next/` to the exported assets. No authentication or secret environment variables are used during the documentation build.

Do not use the documentation source for private company runbooks. Keep deployment-specific configuration, backups, credentials and evidence outside the public repository.
