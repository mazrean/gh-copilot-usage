# AGENTS.md

This file is the single source of truth for agent instructions in this repository. `CLAUDE.md` and other client-specific instruction files import this content rather than duplicating it.

## What this is

`gh-copilot-usage` is a **GitHub CLI extension** (`gh copilot-usage`) that visualizes GitHub Copilot CLI AI-credit (AIC) usage. It reads `~/.copilot/session-store.db` directly and serves a stacked time-series chart over a local HTTP server, plus cross-checks against the GitHub billing API. It reuses the invoking user's `gh` login via `go-gh` — there is no separate token/auth handling in this codebase.

## Commands

The frontend (`internal/server/web`) is generated — `go build`/`go vet`/`go test`/`go tool lint` all require both the templ-generated Go code and the built frontend assets to exist first. `mise.toml` defines tasks that wire this up (`mise tasks` to list them); CI and `.goreleaser.yaml`'s `before.hooks` use the same tasks, so this is the canonical way to reproduce a CI run locally:

```bash
mise run lint    # build:frontend + generate, then go tool lint ./...
mise run test    # build:frontend + generate, then go test ./...
mise run build   # build:frontend and build:go (go build ./...) in dependency order
mise run build:frontend   # just the pnpm/Vite build -> internal/server/web
mise run generate         # just `go tool templ generate`
```

Equivalent raw commands, useful for single-test runs or when iterating without mise:

```bash
pnpm --dir frontend install     # install frontend deps (first time / after package.json changes)
pnpm --dir frontend build       # bundles Lit + UnoCSS -> internal/server/web (go:embed target)
go tool templ generate          # internal/server/templates/*.templ -> *_templ.go
go build ./...                  # build everything
go vet ./...                    # standard vet
go tool lint ./...              # project linter: vet's default analyzers + staticcheck + stylecheck bundled (see tools/lint)
go test ./...                   # all tests
go test ./internal/store/ -run TestAggregateByModelDaily -v   # single test
gofmt -l .                      # must print nothing before committing
goreleaser check                # validate .goreleaser.yaml
```

Frontend-only iteration:

```bash
pnpm --dir frontend dev         # vite dev server, proxies /api to a `go run .` instance on 127.0.0.1:8765
pnpm --dir frontend typecheck   # tsc --noEmit
```

Run locally against the real DB:

```bash
go run . --addr 127.0.0.1:8765 --no-open   # start the server without opening a browser
go run . --json --dimension model --granularity day   # one-shot aggregation, no server
```

Run as an actual `gh` extension during development — `gh extension install .` requires the binary to exist and be named after the current directory (which must start with `gh-`):

```bash
go build -o gh-copilot-usage .
gh extension install .
gh copilot-usage --addr 127.0.0.1:8765
gh extension remove copilot-usage   # when done
```

There is no `golangci-lint` in this repo by design — `go tool lint` (built from `tools/lint/main.go`) is the canonical linter, registered via the go.mod `tool` directive. Treat its failures, and `gofmt -l .` output, as commit blockers.

## Architecture

Four packages, wired together in `main.go`:

- **`internal/store`** — reads AIC usage. Opens `~/.copilot/session-store.db` (SQLite, `modernc.org/sqlite`, pure Go / no cgo) **read-only**, because the Copilot CLI itself holds the DB open in WAL mode while running. If the read-only open fails, it falls back to copying the DB plus its `-wal`/`-shm` sidecars to a temp dir and reading the copy. All usage lives in one table, `assistant_usage_events`; the AIC value is `total_nano_aiu` (nano-scale — divide by 1e9 to get AIU). `Aggregate(dim, gran)` buckets by `created_at` via `strftime` (day/week/month) and stacks by `model` or `session_id`, returning series sorted by total descending.
- **`internal/billing`** — fetches the *billing-API* monthly AIC total (independent of the local DB) via `go-gh`'s `api.NewRESTClient`, which resolves the caller's `gh` auth/host automatically. Hits `GET user` to resolve the login, then `GET users/{login}/settings/billing/ai_credit/usage`. A missing/insufficient-scope token is a normal, non-fatal outcome — the caller (`internal/server`) must surface it as an error payload rather than fail the whole request. In this dev environment the `gh` token lacks the `user` scope, so this endpoint reliably 404s locally; that is expected, not a bug to chase.
- **`internal/server`** — HTTP layer. `GET /` renders a templ-generated static shell (`internal/server/templates/shell.templ`); `/assets/` serves the `go:embed`ded `internal/server/web` directory (the pnpm/Vite build output: a Lit custom-element bundle + UnoCSS stylesheet). Exposes `/api/usage`, `/api/session` (store) and `/api/monthly` (billing, `nil`-safe) as JSON — the frontend fetches these client-side, the shell itself never touches app data. New endpoints should follow the same "degrade gracefully, don't panic" pattern for the billing client.
- **`main.go`** — flag parsing (`--db`, `--addr`, `--no-open`, `--json`, `--dimension`, `--granularity`), opens the store, constructs an optional billing client (logs a warning and continues with `nil` if `gh` isn't authenticated), and either prints one JSON aggregation or starts the HTTP server and opens a browser.

`tools/lint/main.go` is a separate `main` package in the same module (not a nested module) registered as a build tool via the go.mod `tool` directive — that's why `go.mod` can reference it before/independently of feature code without breaking `go build`/`go test`.

### Frontend (`frontend/`)

A pnpm-managed TypeScript package, independent of the Go module, built with Vite. `vite.config.ts` fixes output filenames (no content hashing — this is a locally-served single-user tool, so cache-busting isn't a concern) and points `build.outDir` at `../internal/server/web`, the Go side's `go:embed` target.

- **Lit** web components (`frontend/src/components/`) own everything that needs client-side data fetching or interaction: `usage-dashboard` (root; owns `dimension`/`granularity` state and the `/api/usage` fetch), `usage-toggle-group` (dimension/granularity switches), `usage-chart` (Chart.js stacked bar, emits `session-click`), `usage-summary-cards` (local + billing cards, fetches `/api/monthly` itself, computes the month-end pace projection), `session-detail-modal` (fetches `/api/session` on open). Every component opts out of Shadow DOM (`createRenderRoot() { return this }`) so UnoCSS's single generated stylesheet — which is not Shadow-DOM-encapsulated — applies uniformly.
- **UnoCSS** (`frontend/uno.config.ts`) provides the utility classes plus a `preflight` carrying the GitHub-Primer-style CSS custom properties and `prefers-color-scheme` dark mode.
- **templ** (`internal/server/templates/shell.templ`) renders only the static page shell (head tags, page title, the `<usage-dashboard>` mount point) — it never fetches or embeds app data server-side, keeping initial HTML small.
- Neither `internal/server/templates/*_templ.go` nor `internal/server/web/*` (except `.gitkeep`) are committed — both are generated pre-build (see Commands above).

### Distribution

`.goreleaser.yaml` builds precompiled binaries named `gh-copilot-usage-<os>-<arch>` (no archive wrapping) per the `gh extension install owner/repo` naming convention. Publishing that way requires the GitHub repo name to start with `gh-` (already the case: `mazrean/gh-copilot-usage`).
