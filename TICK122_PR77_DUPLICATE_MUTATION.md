# Tick 122 PR77 duplicate-token mutation

## Scope

PR77 repair head `daffc5f50c306e09f7008b295262a7ccedab6cd3`, descended from public
PR77 `e3ec1b86e4ea96d6e11016a63e65bda294d18b3f` (base `758aa44`). Stable prior
patch ID: `76a55136932d4039d2effdec4c972a5cf3c4d1b1`.

## Adversarial evidence

On original PR77, a 64-caller barrier against
`acquireIngestSpawnToken("same-session", now)` produced **3/64 winners** under
`CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run
'TestT122DuplicateToken$' -v` (PASSing test whose assertion was `winners > 1`).
This reproduces the `Stat` then `WriteFile` race.

On `daffc5f`, the repaired `acquireCloseoutToken("same-session")` barrier held
the first lock until all callers completed. The exact 64-caller gate produced
**1 winner**, PASS, under `CGO_ENABLED=0 go test -race -count=1 ./internal/cli
-run 'TestT122CloseoutTokenSingleWinner$' -v`.

The focused closeout preflight also passed:
`CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'Closeout' -v` — all
closeout, concurrent-parent, release, timeout-descendant, and incremental
closeout tests PASS.

## Verdict

**ACCEPT for this mutation target.** The original duplicate-token red is killed
by the current closeout-specific atomic `O_EXCL` token held through completion.
The ingest throttle remains intentionally separate and time-windowed; this
gate only validates closeout overlap protection.
