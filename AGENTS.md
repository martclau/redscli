# Repository Guidelines

## Project Structure & Module Organization

This repository contains a dependency-free Go CLI for the REDS API. The entrypoint is `cmd/redscli/main.go`. Most implementation lives in `internal/redscli/`, split by concern: command routing in `app.go`, HTTP client logic in `client.go`, endpoint command groups in `commands_*.go`, config handling in `config.go`, output formatting in `output.go`, and pagination helpers in `pagination.go`. Tests live beside package code as `*_test.go`. The API reference is kept in `docs/apispec.txt`.

## Build, Test, and Development Commands

Use the Makefile for common workflows:

```sh
make build      # builds ./redscli with -buildvcs=false
make test       # runs go test ./...
make fmt        # runs go fmt ./...
make vet        # runs go vet ./...
make check      # verifies formatting, then runs vet and tests
make run ARGS="quota"
make clean
```

The `-buildvcs=false` flag is intentional because this environment has previously exposed incomplete Git metadata during builds.

## Coding Style & Naming Conventions

Write idiomatic Go formatted by `go fmt`. Use tabs for indentation as produced by the formatter. Keep package names lowercase and short. Prefer clear command names that mirror API groups, for example `runStringsSearch`, `runBulkSearch`, and `runIOCSummary`. Keep endpoint-specific code in the matching `commands_*.go` file. Avoid external dependencies unless they provide a concrete benefit over the standard library.

## Testing Guidelines

Use Go’s standard `testing` package. Name tests as `Test<Behavior>`, and prefer local `httptest.Server` instances for API client behavior. Add tests for command parsing, request construction, error handling, and pagination behavior when changing those areas. Run `make test` before submitting changes; run `make check` for broader validation.

## Commit & Pull Request Guidelines

There is no existing commit history yet, so use concise imperative commit subjects such as `Add bulk search command` or `Fix config profile selection`. Keep commits focused and avoid mixing unrelated refactors with behavior changes.

Pull requests should include a short summary, the commands run for verification, and any API behavior assumptions. Link related issues when available. Include terminal examples for user-facing CLI changes.

## Security & Configuration Tips

Do not commit API tokens, `.env` files, downloaded malware samples, or private config. Authentication should use `REDS_API_TOKEN`, `--token`, or local config created by `redscli config`.
