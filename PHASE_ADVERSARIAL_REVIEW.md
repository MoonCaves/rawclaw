# Adversarial review: phase-timing consolidation

Reviewed immutable implementation commit `33c742137376ee3bf7ff38497167f19476ec1195`, parent `e7bf12f11c64bb6f7ca90f280f64ec71dda5467d`, against integration tip `2ee995096db544be1ba8c889e4c68e3eb7ef24d1`.

## Verdict

**CHANGES REQUESTED.** Names, message names, source basename placement, typed duration attributes, and exercised completion paths remain stable. One duration-semantic regression is confirmed below.

## Finding 1 — duration starts after the start marker

**Confirmed, low severity but contract-relevant.** `internal/index/consolidated.go:20-32` calls `slog.Info(... event=start)` at line 25 and captures `started := time.Now()` only at line 26. The old closures captured the timestamp before emitting the start record. Any time spent in the start handler is therefore excluded from the reported duration, so completion no longer measures the wall interval beginning at the advertised start marker. Capture the timestamp before the start log.

The default handler makes this latent; the recorder-backed test only checks that a typed duration exists and cannot catch ordering or interval semantics.

## Contract checks

- Immutable source has fold phases `schema-migrate`, `source-migrate`, `attach`, `prepare`, `merge`, `detach`, `tombstone-prune`, `watermark-stamp`, `connection-close`; fence phases remain `acquire` and `release`.
- Start records retain `event=start`; completions retain typed `duration`; source-bearing phases retain `source=filepath.Base(src)`.
- Attach errors complete `attach`; successful attaches complete `detach` through its defer; merge completion is deferred and was observed before forced child exit.
- No race-visible shared production state was introduced by the helper.
- The new test aggregates by message+phase, so it does not pin ordering, cardinality, source values, or positive durations. That is a coverage gap; the timestamp regression is independently visible in the immutable diff.

## Exact commands and observed output

Required memory lookup before touching this report area: `mnemon --store rawclaw recall "phase timing consolidated fold logging" --limit 5 --verbose`. Observed five relevant memories, including prior phase instrumentation and a focused race pass.

Graph orientation: `graphify reflect --if-stale`, `cat graphify-out/reflections/LESSONS.md`, `graphify explain "consolidateOne"`, and `graphify query "fence-acquire schema-heal-migrate tombstone-prune watermark-stamp"`. Reflection observed `0 useful, 0 dead ends, 0 corrected`; `LESSONS.md` had no lessons. The worktree has no graph JSON, so queries reported `graph file not found`. No session-scoped MCP was used.

Focused race command, five iterations: `for i in 1 2 3 4 5; do CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestConsolidate_PhaseLogsHaveStartsAndDurations$' -v || exit; done`. Observed: all five passed; durations were 0.35s, 0.43s, 0.48s, 0.35s, and 0.39s.

Retry fault command: `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestConsolidate_RetryAfterAbruptPostMergeExit$' -v`. Observed `PASS`; child emitted `phase=merge duration=...` and no detach record before exit; parent retry succeeded.

Package gate: `CGO_ENABLED=0 go test -race -count=1 ./internal/index/...`. Observed `ok github.com/MoonCaves/rawclaw/internal/index 137.344s`.

Hygiene gates: `gofmt -l internal/` and `git diff --check`. Observed no output; both exited 0. No production or test Go files were edited.

