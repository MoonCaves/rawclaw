# Tick 30 semaphore proposal audit

run_timestamp: 2026-08-27T00:07:00Z (UTC; evidence freeze audit)
base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`
scope: `PA-GO-WEIGHTED-SEMAPHORE-WRITER-001` only; read-only product audit; no mailbox or cursor access.

## Verdict

**DUPLICATE.** The exact proposal is already recorded in the prior-art receipt
`3c31ccb` (`worker/furiosa-t29-external-mechanisms-20260827`) with fingerprint
`3be536e7d5aa2e34267b8b0b334b81165311f124ce38d5bfd45ac57676593c40`.
Re-recording it as a new mechanism would double-count the same candidate.

## Evidence

- Dependency diff: `golang.org/x/sync` is absent from `go.mod` and `go.sum`; the
  proposal would add a new dependency. No dependency change is authorized or
  needed for this audit.
- Existing writer admission: `internal/index/consolidated_fence.go:35`
  defines `AcquireConsolidatedFence(ctx)`, using `github.com/gofrs/flock` on
  `consolidated.lock`, polling with a context timeout, and reporting contention.
  `internal/index/consolidated.go:403,557` and `internal/index/rebuild.go:83`
  already call this fence around consolidated work. This is a cross-process
  durable file fence; a Go semaphore cannot replace it.
- Existing stdlib synchronization: `internal/source/sources.go:16`,
  `internal/store/connect_test.go:45`, and multiple index/CLI tests use
  `sync.Mutex` and channels. A weight-one in-process gate can be represented by
  a one-token channel and `select { case token <- struct{}{}: ...; case <-ctx.Done(): ... }`
  or by a mutex where cancellation is not required. No `semaphore.Weighted`
  symbol exists in the tree.
- SQLite boundary: `internal/store/store.go:340-345` sets WAL mode,
  `busy_timeout`, and `SetMaxOpenConns(1)` for a connection pool. A Go-level
  semaphore can reduce admission pressure before SQLite, but does not provide
  transaction serialization, busy-handler behavior, crash ownership, or
  cross-process exclusion.
- Proposal semantics from `3c31ccb`: `semaphore.Weighted.Acquire(ctx)` gives
  cancellable queued admission and `Release`; weight one is proposed for
  writers. This is an aggregate concurrency limit, not singleflight result
  sharing and not a durable writer fence.

## Minimalism and semantic ruling

- `x/sync` is **not already a dependency**. Adding it solely for a weight-one
  gate fails the dependency-minimality trap unless benchmark evidence proves a
  required fairness/cancellation contract unavailable in the stdlib.
- For one permit, `semaphore.Weighted` is semantically narrower than its API
  suggests. A buffered one-token channel plus `context.Context` preserves the
  required cancellable admission with zero dependencies. A `sync.Mutex` is
  smaller still when waiting need not be cancellable.
- `Weighted` is only justified if a future measured design needs multiple
  resource weights or its documented FIFO queue behavior is an explicit,
  tested contract. The current proposal says weight one and supplies no such
  measurement or fairness test.
- It remains distinct in *kind* from `PA-SQLITE-BEGIN-IMMEDIATE-001` (SQLite
  transaction admission), `PA-GO-SINGLEFLIGHT-FALLBACK-001` (coalescing
  identical calls), and `AcquireConsolidatedFence` (cross-process file fence),
  but that distinction does not make it a new candidate: the exact fingerprint
  is already recorded by `3c31ccb`.

## Verification and accounting

- Patch identity: no product patch; `git diff 0d1da19c4c21961b86cb3ca84ed047d941c83ed3`
  is empty for product files.
- Net lines: `0` product lines; no dependency delta.
- Exact test-list preflight: none run because there is no implementation or
  test mutation. Required future gate if implemented: targeted context-
  cancellation and fairness tests for the chosen primitive, then
  `CGO_ENABLED=0 go test -race -count=1 ./...` and `gofmt -l internal/`.
- Adoption evidence: none found; score remains `0`.
- Status: `DUPLICATE`, not score eligible, no merge authorization.
- Next lead: only revisit if a measured requirement demonstrates weighted
  permits or FIFO fairness that a stdlib token channel cannot preserve; otherwise
  reject the dependency and retain the existing fence.

