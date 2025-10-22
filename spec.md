# Quote Cards — Static‑First Specification (v2)

**Goal:** A self‑hosted, portable system that turns Markdown quote files into shareable PNGs and wrapper pages — deployable on any static host with zero server requirements.

---

## 1) Core Principles
- **Static outputs:** PNG + HTML are the product; any static host works (GitHub Pages, Netlify, Cloudflare Pages, S3+CDN, Nginx).
- **CI‑rendered:** Rendering happens in CI (e.g., GitHub Actions) or locally — no runtime server needed.
- **Portable by design:** Minimal dependencies (Node + Satori + Resvg). Docker optional.
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
- **PNG:** `/cards/<id>.png` (OG‑sized, 1200×630 default)
- **Wrapper page:** `/q/<id>/index.html` with OG/Twitter tags and a meta‑refresh back to the original `url`

For each source `url`:
- **Source index:** `/sources/<domain>/<article-slug>/index.html` listing all quotes from that article.

**Permalinks:**
- Quote: `/q/<id>/`
- Static image: `/cards/<id>.png`
- Source index: `/sources/<domain>/<article-slug>/`

---

## 4) Repository Layout
```
.
├─ quotes/                 # Markdown quote files (source of truth)
│  └─ *.md
├─ q/                      # generated: one folder per quote with index.html
├─ public/
│  └─ cards/               # generated: PNGs
├─ sources/                # generated: per‑URL indexes
├─ templates/
│  ├─ wrapper.html         # OG wrapper template
│  └─ source.html          # source index template
├─ assets/
│  └─ fonts/               # Inter-Regular.ttf, Inter-Bold.ttf (or your fonts)
├─ render.mjs              # build script (Node 18+)
└─ .github/workflows/build.yml   # CI that renders + commits artifacts
```

---

## 5) Build Pipeline

### 5.1 Local build (optional)
```bash
npm install
npm run build     # runs: node render.mjs
```
Outputs PNGs + HTML into `public/cards`, `q`, and `sources`.

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
      - uses: actions/setup-node@v4
        with:
          node-version: 20
      - run: npm ci || npm install
      - name: Render PNGs & Pages
        run: npm run build
      - name: Commit artifacts
        run: |
          git config user.name "github-actions"
          git config user.email "actions@users.noreply.github.com"
          git add public/cards q sources
          git commit -m "build: quote cards" || echo "Nothing to commit"
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

The push triggers GitHub Actions → PNG + wrapper generated → Pages updated.

### 6.2 Other apps (Drafts, iA Writer, Obsidian, VS Code)
- Template the frontmatter, save into `quotes/`, push to GitHub. Same CI flow.

---

## 7) Theming & Layout

- Modify `render.mjs` → `renderSvg()` for layout, colors, fonts, sizes, quotation mark style.
- Replace fonts by dropping TTF/OTF in `assets/fonts/` and adjusting the font load section.
- Default size is **1200×630** for ideal OG previews; can be changed project‑wide.

---

## 8) Performance & Caching

- PNGs are static files → naturally cacheable by the host/CDN.
- Use far‑future cache headers if your host supports custom headers; otherwise default host behavior is fine.
- PNGs are content‑addressed by `id` (immutable); wrappers can be cached as well.

---

## 9) Portability Notes

- Runs on Node 18+ anywhere (Mac/Win/Linux). Dockerfile provided for container builds.
- CI examples can be adapted to GitLab, CircleCI, etc.
- For project‑page URLs (e.g., `/repo-name` base path), ensure templates use relative paths or a configurable `BASE_PATH` if needed.

---

## 10) Error Handling & Validation

- Build script skips files missing required fields (`id`, `quote`, `name`, `url`) and logs a warning.
- Optional: add a pre‑commit hook or CI step to validate frontmatter schema.
- If fonts are missing, Satori will fall back; provide local fonts for consistent results.

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
**A:** Yes. Any method that commits a new `quotes/*.md` file (Shortcuts, Working Copy, GitHub mobile) will trigger the GitHub Actions workflow to build PNGs and pages automatically.

**Q: Do we need Cloudflare/Vercel?**  
**A:** No. They’re optional for live previews or dynamic query‑param variants. The static‑first spec needs only your static host + CI.

**Q: Can we keep the option for a dynamic preview?**  
**A:** Yes — add a small serverless function later. It doesn’t change the static pipeline.

---

## 14) Deliverables Checklist

- [ ] Repo structure created
- [ ] Fonts added (`assets/fonts/*.ttf`)
- [ ] At least one quote in `quotes/`
- [ ] `render.mjs` committed
- [ ] Templates customized (wrapper/source)
- [ ] GitHub Actions enabled + Pages enabled
- [ ] Mobile Shortcut or editor template ready

---

**End of spec** (v2)
