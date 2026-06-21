# Contributing to RawClaw

Thanks for your interest in RawClaw. This guide gets you from a clean checkout to a
green build in a few minutes.

## Prerequisites

- **Go 1.24+** (the `go` directive in `go.mod` is the floor).
- No cgo, no system libraries: RawClaw is pure Go with `CGO_ENABLED=0`, so a stock
  Go toolchain is all you need.

## Build

```bash
go build ./cmd/rawclaw
```

This produces a `rawclaw` binary in the current directory.

## Test

```bash
go test ./...                 # the full suite
go test -race -count=1 ./...  # what CI runs: race detector, no test cache
```

Tests must pass with the race detector before a change is merged.

## Lint

```bash
golangci-lint run
```

The linter config is `.golangci.yml`. Keep new code lint-clean; if a warning is a
deliberate exception, suppress it narrowly with a `//nolint:<linter>` directive and a
one-line reason rather than loosening the global config.

## Pull requests

1. Branch from `main`.
2. Keep each commit focused and its message descriptive (what changed and why).
3. Make sure `go build ./...`, `go test -race -count=1 ./...`, and `golangci-lint run`
   are all green.
4. Update `CHANGELOG.md` (the `[Unreleased]` / current section) for any user-visible
   change, and the `README.md` if you change behavior or flags.
5. Open the PR against `main` with a short description of the change and its rationale.
