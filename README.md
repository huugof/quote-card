# Quote Cards

Static-first quote card pipeline: Markdown quotes in `quotes/` become PNG cards plus shareable wrapper pages and per-source indexes. Everything builds in CI or locally with Node — no runtime server needed.

## Requirements

- Node.js 18+
- npm 9+

## Install

```bash
npm install
```

## Validate Frontmatter

Run a fast schema check without writing any files:

```bash
npm run check
```

This ensures every quote contains required fields (`id`, `quote`, `name`, `url`) and that IDs are unique and URLs are well-formed.

## Build Assets

```bash
npm run build
```

Outputs land in:

- `public/cards/<id>.png` — Open Graph-ready PNG card
- `q/<id>/index.html` — wrapper page with OG/Twitter meta + redirect
- `sources/<domain>/<slug>/index.html` — grouped quotes per source article

Running the build wipes the three output directories before regenerating files.

## Authoring Quotes

1. Duplicate `quotes/2024-03-21-1200-sample.md` and update the YAML frontmatter.
2. Use `YYYY-MM-DD-HHMM-<shortid>` for `id` (any unique string works).
3. Keep `url` consistent across related quotes so they group on the same source page.
4. Commit the markdown file — GitHub Actions renders and pushes generated assets back to `main`.

Optional Markdown body text becomes supporting copy on the source index page.

## Fonts & Theming

The renderer bundles [Atkinson Hyperlegible](https://github.com/google/fonts/tree/main/ofl/atkinsonhyperlegible) (regular + bold) in `assets/fonts/`. Swap these files if you prefer different typography and adjust the inline styles inside `render.mjs` / templates.

To tweak colors or layout, edit `renderSvg()` inside `render.mjs` and the HTML templates under `templates/`.

## Continuous Integration

`.github/workflows/build.yml` installs dependencies, runs `npm run build`, and commits the `public/cards`, `q`, and `sources` directories back to `main` on every push touching quote content or build sources. Enable GitHub Pages for the repo to serve the generated static output.
