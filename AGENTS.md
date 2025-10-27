# Repository Guidelines

## Project Structure & Module Organization
- `quotes/`: Markdown source files with YAML front matter (ID, quote, name, url, etc.).
- `cmd/quote-cards/`: CLI entry point that wires argument parsing, env handling, and build orchestration.
- `internal/`: Go packages for quote loading, manifest diffing, HTML templating, and JPEG rendering (`internal/render` uses gg + embedded fonts).
- `internal/static/templates/`: `wrapper.html` and `source.html` strings embedded into the binary; edit these to change page chrome.
- Generated output (`cards/`, `q/`, `sources/`) and `build-manifest.json` are created by the CLI and ignored by git.

## Build, Test, and Development Commands
- `go build -o quote-cards ./cmd/quote-cards`: Compile the static renderer binary.
- `./quote-cards --check`: Validate front matter and URL normalization without touching the filesystem (CI runs this first).
- `./quote-cards`: Render JPEG cards, wrapper pages, and source indexes using the cached manifest for incremental work.
- `CARD_VERSION=3 ./quote-cards`: Force a regeneration of every card with `?v=3` cache busting (bump the value as needed).

## Coding Style & Naming Conventions
- Go code follows standard `gofmt` formatting, explicit error handling, and package-per-domain organization (`internal/build`, `internal/quotes`, etc.).
- Keep helper functions short and package-private unless they are reused across modules.
- Quote IDs stay in `YYYY-MM-DD-HHMM-shortid` form to preserve chronological sorting and uniqueness across manifests.

## Testing Guidelines
- No dedicated test suite yet; rely on `./quote-cards --check` plus manual inspection of generated assets.
- When adding tests, place them next to the package under test (e.g., `internal/quotes/quotes_test.go`) and keep fixtures in `testdata/`.
- After major renderer changes, spot-check a few cards and group pages locally to confirm layout parity.

## Commit & Pull Request Guidelines
- Use concise conventional-style prefixes (`feat:`, `fix:`, `chore:`, `build:`) and keep diffs narrowly scoped.
- Document PR intent, affected commands, and include before/after screenshots or sample quote IDs whenever the visual output changes.
- Reference related issues, ensure CI (build + deploy) passes, and verify that `build-manifest.json` stays in sync if you modify rendering logic.

## Deployment Workflow Notes
- GitHub Actions builds the Go binary, runs the validation + render steps, commits the updated `build-manifest.json`, and publishes the `cards/`, `q/`, and `sources/` bundle via `actions/deploy-pages@v4`.
- For emergency deploys outside CI, run `go build -o quote-cards ./cmd/quote-cards` followed by `BASE_PATH=… SITE_ORIGIN=… ./quote-cards`, then upload `cards/`, `q/`, and `sources/` to the host of choice (GitHub Pages artifact, Cloudflare direct upload, etc.).
