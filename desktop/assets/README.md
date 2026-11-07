# SkillHub brand — Common Thread (B)

Canonical SVGs live in `frontend/public/assets/brand/` and are copied unchanged into both Web and desktop builds.

- `logo-color.svg`: blue/ink symbol with outlined SkillHub wordmark, for light backgrounds.
- `logo-mono.svg` / `logo-white.svg`: single-color wordmarks for light/dark backgrounds.
- `mark-color.svg`, `mark-mono.svg`, `mark-white.svg`: symbol-only equivalents.
- `app-icon.svg`: blue tile with a white symbol and transparent macOS safe margins.
- `favicon.svg`: compact tile without external safe margins, for browser tabs.

Blue: `#063BEB`. Ink: `#0B1423`. White: `#FFFFFF`.
All SVGs contain vector paths, have transparent backgrounds except the icon tiles, and need no fonts or external resources. The wordmark uses outlined Arial Bold letterforms. Keep proportions; do not stretch or add effects. Use the symbol for compact placements and the wordmark where the name needs to be visible.

`app-icon.png` is the 1024 px raster export of `app-icon.svg`. After editing the SVG, export it again at 1024 × 1024 with an SVG renderer, then run from `desktop/`:

```sh
go tool wails3 generate icons -input frontend/public/assets/brand/app-icon.png -macfilename assets/SkillHub.icns -windowsfilename assets/SkillHub.ico
```

The build script places ICNS into the macOS bundle and declares `CFBundleIconFile`. On Windows it generates architecture-specific executable resources from ICO and the application manifest. The Wails application also embeds the PNG for its application icon. No rendering tool is required at application runtime.
