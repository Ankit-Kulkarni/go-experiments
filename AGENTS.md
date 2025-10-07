# Repository Guidelines

## Project Structure & Module Organization
The repo collects standalone Go experiments. Each top-level directory such as `transparentProxy/`, `notification/`, `tcpqueue/`, or `webs/` contains its own `go.mod` and houses source and tests for that experiment only. Shared scratch utilities live in `test/`, while the root `README.md` tracks the catalogue. When adding a new idea, create a sibling directory, keep binaries and generated data out of git, and document any external services it touches.

## Build, Test, and Development Commands
Work with Go 1.21+. Typical loops:
- `cd notification && go run .` to execute a module’s main package.
- `cd tcpqueue && go build ./...` before committing to ensure it compiles cleanly.
- `for d in */go.mod; do (cd "${d%/go.mod}" && go test ./...); done` to exercise every module.
Prefer module-local `go test ./...` when iterating to keep output focused, and run `go mod tidy` after updating dependencies.

## Coding Style & Naming Conventions
Rely on `gofmt`/`goimports` (tabs for indentation, trailing newline). Name packages after their responsibility, keep filenames lowercase with optional underscores, and export only what a peer module needs. Use short comments to clarify non-obvious concurrency, networking, or AWS usage, and keep main packages minimal by pushing logic into small packages.

## Testing Guidelines
Place `_test.go` files beside the code under test and favour table-driven cases. Mock network or AWS calls via interfaces so tests stay offline. Measure with `go test -cover ./...` and aim to cover the primary code paths before opening a PR. Use `go test -run TestName ./...` for quick, focused feedback.

## Commit & Pull Request Guidelines
History uses short, imperative summaries (`Add graceful restart experiment`). Keep one experiment per commit, include `go.sum` updates, and note any manual setup in the body. PRs should describe the experiment’s intent, list validation commands, flag breaking changes, and link issues or docs when available. Include screenshots or sample output if behavior is user-visible.

## Security & Configuration Tips
Never commit credentials; modules that call AWS expect keys via environment variables or profiles. Default to local `.env` files ignored by git, and scrub logs before sharing. For listeners or proxies, state default ports and ensure shutdown paths close connections to avoid leaking resources.
