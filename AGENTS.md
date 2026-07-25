# AGENTS.md

This file is the single source of truth for agent instructions in this repository. `CLAUDE.md` and other client-specific instruction files import this content rather than duplicating it.

## What this is

`gh-copilot-usage` is a **GitHub CLI extension** (`gh copilot-usage`) that visualizes GitHub Copilot CLI AI-credit (AIC) usage. It reads `~/.copilot/session-store.db` directly and serves a stacked time-series chart over a local HTTP server, plus cross-checks against the GitHub billing API. It reuses the invoking user's `gh` login via `go-gh` — there is no separate token/auth handling in this codebase.

## Commands

```bash
go build ./...                  # build everything
go vet ./...                    # standard vet
go tool lint ./...              # project linter: vet's default analyzers + staticcheck + stylecheck bundled (see tools/lint)
go test ./...                   # all tests
go test ./internal/store/ -run TestAggregateByModelDaily -v   # single test
gofmt -l .                      # must print nothing before committing
goreleaser check                # validate .goreleaser.yaml
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
- **`internal/server`** — HTTP layer. `go:embed`s `internal/server/web` (an `index.html` with vanilla JS + a vendored `chart.umd.min.js` — no build step, no CDN dependency) and exposes `/api/usage` (store) and `/api/monthly` (billing, `nil`-safe). New endpoints should follow the same "degrade gracefully, don't panic" pattern for the billing client.
- **`main.go`** — flag parsing (`--db`, `--addr`, `--no-open`, `--json`, `--dimension`, `--granularity`), opens the store, constructs an optional billing client (logs a warning and continues with `nil` if `gh` isn't authenticated), and either prints one JSON aggregation or starts the HTTP server and opens a browser.

`tools/lint/main.go` is a separate `main` package in the same module (not a nested module) registered as a build tool via the go.mod `tool` directive — that's why `go.mod` can reference it before/independently of feature code without breaking `go build`/`go test`.

### Distribution

`.goreleaser.yaml` builds precompiled binaries named `gh-copilot-usage-<os>-<arch>` (no archive wrapping) per the `gh extension install owner/repo` naming convention. Publishing that way requires the GitHub repo name to start with `gh-` (already the case: `mazrean/gh-copilot-usage`).
