# Development

See [AGENTS.md](AGENTS.md) for the full architecture and design notes. Quick reference for building, testing, and linting:

```bash
mise run build   # build the frontend and the gh-copilot-usage binary
mise run test    # go test ./...
mise run lint    # go vet + staticcheck + stylecheck
```
