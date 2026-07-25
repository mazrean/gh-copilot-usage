# gh-copilot-usage

[日本語](README.ja.md)

A [`gh` CLI](https://cli.github.com/) extension that visualizes your GitHub Copilot CLI AI-credit (AIC) usage as a stacked time-series chart, right in your browser — and cross-checks it against GitHub's billing API.

> The screenshots below use synthetic sample data generated for documentation purposes, not any real account's usage.

## What it does

`gh-copilot-usage` reads `~/.copilot/session-store.db` — the local SQLite database the Copilot CLI already keeps on your machine — and turns it into an interactive dashboard:

- **Stacked usage chart** — AI-credit usage over time, bucketed daily/weekly/monthly and stacked by model or by session.
- **Billing cross-check** — fetches this month's AI-credit total from the GitHub billing API (via your existing `gh` login) and shows a month-end pace projection next to your local measurement.
- **Session drill-down** — click a bar to see a session's per-model breakdown, per-turn AIU chart, and (when your Copilot CLI version records them) per-turn duration, token counts, and the trace of individual calls — including sub-agent delegations.
- **Model drill-down** — click a model's segment to see its token-cost breakdown by category (input / cached input / cache write / output).
- **Built-in English/Japanese UI toggle**, independent of your terminal locale.
- **Scriptable JSON mode** (`--json`) for piping aggregated usage into other tools, no server required.

Everything runs locally: the local-DB read path needs no token at all, and the billing cross-check reuses your existing `gh` authentication — there's no separate credential setup.

## Screenshots

**Daily usage, stacked by model** — with the local measurement and billing cross-check summary cards up top:

![Daily usage stacked by model](docs/screenshots/dashboard-model-daily.png)

**Weekly usage, stacked by session** — each segment is one Copilot CLI session:

![Weekly usage stacked by session](docs/screenshots/dashboard-session-weekly.png)

**Session drill-down** — per-model totals, a per-turn chart, and the selected turn's token counts and call trace:

![Session detail modal](docs/screenshots/session-detail-modal.png)

**Model drill-down** — token-cost breakdown by category:

![Model detail modal](docs/screenshots/model-detail-modal.png)

## Install

```bash
gh extension install mazrean/gh-copilot-usage
```

## Usage

```bash
gh copilot-usage
```

This starts a local server (default `127.0.0.1:8765`, falling back to a random port if that one is busy) and opens it in your browser.

| Flag | Default | Description |
| --- | --- | --- |
| `--db` | `~/.copilot/session-store.db` | Path to the Copilot CLI session-store DB to read. |
| `--addr` | `127.0.0.1:8765` | Address to serve the web UI on. |
| `--no-open` | `false` | Don't open a browser automatically. |
| `--json` | `false` | Print aggregated usage as JSON and exit (no server). |
| `--dimension` | `model` | Stacking dimension for `--json`: `model` or `session`. |
| `--granularity` | `day` | Time bucket for `--json`: `day`, `week`, or `month`. |

For example, to get a weekly-by-model summary without starting the server:

```bash
gh copilot-usage --json --dimension model --granularity week
```

### Billing cross-check

The "this month (billing)" card calls the GitHub billing API through your existing `gh` login. If your token lacks the `user` scope, that card degrades gracefully (it reports the check as unavailable) instead of failing the whole page — run `gh auth refresh -s user` to enable it.

## Development

See [AGENTS.md](AGENTS.md) for the full architecture and build/test/lint commands. In short:

```bash
mise run build   # build the frontend and the gh-copilot-usage binary
mise run test    # go test ./...
mise run lint    # go vet + staticcheck + stylecheck
```
