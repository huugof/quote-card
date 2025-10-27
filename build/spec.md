# Quote Cards — Static‑First Specification (v2)

**Goal:** A self‑hosted, portable system that turns Markdown quote files into shareable JPEGs and wrapper pages — deployable on any static host with zero server requirements.

---

## 1) Core Principles
- **Static outputs:** JPEG + HTML are the product; any static host works (GitHub Pages, Netlify, Cloudflare Pages, S3+CDN, Nginx).
- **CI‑rendered:** Rendering happens in CI (e.g., GitHub Actions) or locally — no runtime server needed.
- **Portable by design:** Minimal dependencies (single Go binary with embedded fonts/templates). Docker optional.
- **Source of truth:** Each quote is a Markdown file with YAML frontmatter in `/quotes`.
- **Lightweight UX:** Create quotes from phone or computer (Shortcuts, Drafts, Working Copy, GitHub mobile, etc.).

---

## 2) Content Model

### 2.1 Quote file (Markdown + YAML frontmatter)
```md
---
id: 2025-10-21-0745-kc7j          # required, unique per quote
quote: "Small changes compound into big shifts."   # required
name: "Jane Doe"                  # required
url: "https://example.com/article"                # required
article_title: "Rethinking the Everyday"          # optional but recommended
source_domain: "example.com"                      # optional; inferred from url if absent
created_at: "2025-10-21T07:45:00-04:00"          # optional
tags: [design, habits]                            # optional
---
(optional commentary body)
```

**Uniqueness:** `id` must be unique. Recommended pattern: `YYYY-MM-DD-HHMM-<shortid>` generated on the client (Shortcut) or by a helper script.

**Grouping rule:** Quotes with the same **`url`** are grouped together on a source index page.

---

## 3) Outputs

For each quote:
- **JPEG:** `/cards/<id>.jpg` (OG‑sized, 1200×628 default)
- **Wrapper page:** `/q/<id>/index.html` with OG/Twitter tags and a prominent link back to the original `url`
  - Social meta tags (`og:image`, `twitter:image`) include a cache-busting query string (e.g., `?v=2`).

For each source `url`:
- **Source index:** `/sources/<domain>/<article-slug>/index.html` listing all quotes from that article.

**Permalinks:**
- Quote: `/q/<id>/`
- Static image: `/cards/<id>.jpg`
- Source index: `/sources/<domain>/<article-slug>/`

---

## 4) Repository Layout
```
.
├─ quotes/                 # Markdown quote files (source of truth)
│  └─ *.md
├─ q/                      # generated: one folder per quote with index.html
├─ cards/                  # generated: JPEGs
├─ sources/                # generated: per‑URL indexes
├─ cmd/quote-cards/        # CLI entry point for the Go renderer
├─ internal/               # Go packages (quotes, build orchestration, renderer, templating, static assets)
│  └─ static/              # Embedded fonts and HTML templates
├─ build/spec.md           # this document
├─ assets/
│  └─ fonts/               # Atkinson Hyperlegible (bundled locally)
└─ .github/workflows/build.yml   # CI that renders + commits artifacts
```

---

## 5) Build Pipeline

### 5.1 Local build (optional)
```bash
go build -o quote-cards ./cmd/quote-cards
./quote-cards --check        # schema validation only
./quote-cards                # render cards, wrappers, sources
CARD_VERSION=3 ./quote-cards # rebuild cards with ?v=3 cache busting
```
Outputs JPEGs + HTML into `cards`, `q`, and `sources` while updating `build-manifest.json` for incremental builds.

### 5.2 GitHub Actions (recommended)
Trigger on changes to `quotes/**`:
```yaml
name: Build Quote Cards

on:
  push:
    paths:
      - 'quotes/**'
  workflow_dispatch:

permissions:
  contents: write

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: 1.22
      - run: go build -o quote-cards ./cmd/quote-cards
      - name: Validate front matter
        run: ./quote-cards --check
      - name: Render JPEGs & pages
        env:
          CARD_VERSION: 2
        run: ./quote-cards
      - name: Commit manifest
        run: |
          git config user.name "github-actions"
          git config user.email "actions@users.noreply.github.com"
          git add build-manifest.json
          git commit -m "chore: update build manifest" || echo "Nothing to commit"
          git push
```
**Hosting:** Enable GitHub Pages for the repo. It will serve `/cards`, `/q`, and `/sources` as static assets.

---

## 6) Client Workflows (Mobile & Desktop)

### 6.1 Apple Shortcuts (example flow)
1. Get current page URL + selection (Safari share sheet or clipboard).
2. Prompt for: *quote text*, *author*, (optional) *article title*.
3. Generate `id` (timestamp + random suffix).
4. Build Markdown with frontmatter.
5. Commit to GitHub:
   - Via the **GitHub API** (HTTP action), or
   - Using the **Working Copy** app, or
   - Using GitHub’s **mobile app** (manual paste & commit).

The push triggers GitHub Actions → JPEG + wrapper generated → Pages updated.

### 6.2 Other apps (Drafts, iA Writer, Obsidian, VS Code)
- Template the frontmatter, save into `quotes/`, push to GitHub. Same CI flow.

---

## 7) Theming & Layout

- Update `internal/render` to tweak layout constants, palette, padding, or typography; regenerate to preview changes.
- Replace fonts by dropping TTF/OTF in `internal/static/fonts/` and adjusting the font load section if weights change.
- Default size is **1200×628** for ideal OG previews (1.91:1); update the constants in `internal/render` to change it project-wide.

---

## 8) Performance & Caching

- JPEGs are static files → naturally cacheable by the host/CDN.
- Use far‑future cache headers if your host supports custom headers; otherwise default host behavior is fine.
- JPEGs are content‑addressed by `id` (immutable); wrappers can be cached as well.

---

## 9) Portability Notes

- Runs anywhere Go 1.22+ is available (macOS, Windows, Linux). Distribute the compiled binary for repeatable builds.
- CI examples can be adapted to GitLab, CircleCI, etc.; install Go, compile the CLI, run `./quote-cards`.
- For project-page URLs (e.g., `/repo-name` base path), keep `BASE_PATH` configurable (env var or flag) so generated links stay correct.

---

## 10) Error Handling & Validation

- CLI exits non-zero when required front matter fields (`id`, `quote`, `name`, `url`) are missing or malformed.
- Optional: add a pre-commit hook or CI step (`./quote-cards --check`) to gate merges.
- If embedded fonts are replaced, ensure the new TTFs load correctly; parse failures surface as build errors.

---

## 11) Security & Privacy

- No secrets required for local builds.
- GitHub Actions only needs `contents: write` to commit built artifacts.
- The only data published is what’s committed under `/quotes` and the generated outputs.

---

## 12) Roadmap (optional enhancements)

- **Tag indexes:** build `/tags/<tag>/` pages from frontmatter.
- **Alternate themes:** support `theme: dark|light|minimal` in frontmatter to change card design at build time.
- **Multi‑size outputs:** also render square (1080×1080) assets for social grids.
- **RSS/JSON export:** generate a feed of new quotes for downstream automation.
- **Bookmarklet:** quick capture from desktop browsers posting a file via GitHub API.

---

## 13) FAQ

**Q: Can pushes from mobile trigger everything?**  
**A:** Yes. Any method that commits a new `quotes/*.md` file (Shortcuts, Working Copy, GitHub mobile) will trigger the GitHub Actions workflow to build JPEGs and pages automatically.

**Q: Do we need Cloudflare/Vercel?**  
**A:** No. They’re optional for live previews or dynamic query‑param variants. The static‑first spec needs only your static host + CI.

**Q: Can we keep the option for a dynamic preview?**  
**A:** Yes — add a small serverless function later. It doesn’t change the static pipeline.

---

## 14) Deliverables Checklist

- [ ] Repo structure created
- [ ] Fonts added (`internal/static/fonts/*.ttf`)
- [ ] At least one quote in `quotes/`
- [ ] `build/render.mjs` committed
- [ ] Templates customized (wrapper/source)
- [ ] GitHub Actions enabled + Pages enabled
- [ ] Mobile Shortcut or editor template ready

---

**End of spec** (v2)
