# Quote Cards

Static-first quote card pipeline: Markdown quotes in `quotes/` become JPEG cards plus shareable wrapper pages and per-source indexes. The renderer is a single Go binary — no runtimes or package installs required.

## Requirements

- Go 1.22+

## Build the CLI

```bash
go build -o quote-cards ./cmd/quote-cards
```

## Validate Frontmatter

Run a fast schema check without touching the filesystem:

```bash
./quote-cards --check
```

This ensures every quote contains the required fields (`id`, `quote`, `name`, `url`) and that IDs and URLs remain unique.

## Generate Assets

```bash
./quote-cards
```

Outputs land in:

- `cards/<id>.jpg` — Open Graph-ready JPEG card (1200×628, 1.91:1)
- `q/<id>/index.html` — wrapper page with OG/Twitter meta pointing back to the article
- `sources/<domain>/<slug>/index.html` — grouped quotes per source

The binary reads `build-manifest.json` to skip unchanged quotes. Delete the manifest or pass `--force` to rebuild everything from scratch.

### Refresh social previews

```bash
CARD_VERSION=3 ./quote-cards
```

Set `CARD_VERSION` to bump the cache-busting query string on every `og:image` / `twitter:image` URL so platforms invalidate their previews.

## Authoring Quotes

1. Duplicate `quotes/2024-03-21-1200-sample.md` and update the YAML frontmatter.
2. Use `YYYY-MM-DD-HHMM-<shortid>` for `id` (any unique string works).
3. Keep `url` consistent across related quotes so they group on the same source page.
4. Commit the Markdown file — GitHub Actions renders and deploys the generated assets to Pages.

Optional Markdown body text becomes supporting copy on the source index page.

## Fonts & Theming

The renderer bundles [Atkinson Hyperlegible](https://github.com/google/fonts/tree/main/ofl/atkinsonhyperlegible) (regular + bold) in `internal/static/fonts/`. Replace the TTFs to swap typefaces, and adjust the drawing logic in `internal/render` or the HTML templates in `internal/static/templates/` to tweak colors and layout.

## Continuous Integration

## Paths, Base URLs & Social Previews

By default the renderer produces links such as `/cards/<id>.jpg`. When deploying to a sub-path (e.g. GitHub Pages project site `https://user.github.io/quote-card`), set two environment variables before running the build:

```bash
export BASE_PATH=/quote-card
export SITE_ORIGIN=https://user.github.io
./quote-cards
```

- `BASE_PATH` prepends all internal links that start with `/` so they resolve under the project folder.
- `SITE_ORIGIN` turns relative asset paths into absolute URLs for the Open Graph image tags so social scrapers can fetch the JPEG without following redirects. These URLs include the current cache-busting query string (default `?v=2`).

Leave both empty when serving from your domain root. For user/organization pages (repo named `username.github.io`), keep `BASE_PATH` empty and only set `SITE_ORIGIN` to your live hostname.

## Continuous Integration

`.github/workflows/build.yml` builds the Go renderer, runs `./quote-cards` with the appropriate environment variables, commits the updated `build-manifest.json`, and deploys the static bundle to GitHub Pages via the Pages artifact flow.
