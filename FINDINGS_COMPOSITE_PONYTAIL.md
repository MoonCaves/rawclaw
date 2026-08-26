# Ponytail review: composite `9788d21`

Scope: deletion/reuse only. The composite is approximately `+844/-60` versus
`5eec12b` (`+784` net), so these are concrete cuts that preserve the red
contracts for freshness, cancellation, authority, deletion, and eventual
publication.

## Ranked findings

### 1. `yagni`: global detached-session handoff

`internal/cli/tagpublish.go:27-29` and `internal/cli/cmd_tag.go:525-533`
introduce mutable package globals `tagPublishSessionID` and `tagPublishMu` only
to smuggle `fullSID` through the one-argument `spawnTagPublish` seam. Capture
the value in the call instead: make the seam
`func(dbp, sessionID string) error`, call `spawnTagPublish(dbp, fullSID)`, and
make `spawnTagPublishChild` take two required strings. This removes shared
mutable state, the lock, the variadic argument branches, and the reset dance;
the detached child and test seams remain intact. Estimated production net:
**-14 lines**. Deduction opportunity: **-3**.

### 2. `shrink`: duplicated optional-argument validation

`internal/cli/tagpublish.go:49-74,79-94` uses variadic `sessionID ...string`
and `sessionIDs ...string`, then manually checks length/empty values. Every
production caller already has exactly one session ID after the composite
change. Required parameters (`spawnTagPublishChild(dbp, sid)` and
`runTagPublishChild(ctx, w, dbp, sid)`) make the interface truthful and delete
roughly 8 lines of argument plumbing. Keep one explicit empty-ID validation at
the command boundary if the hidden command can be invoked directly. Estimated
net: **-8 lines** (partly overlapping finding 1; do not double-count). Deduction
opportunity: **-1**.

### 3. `yagni`: duplicate `consolidateOne` compatibility wrapper if callers can migrate

`internal/index/consolidated.go:678-682` keeps `consolidateOne` solely as a
background-context wrapper around `consolidateOneContext`. Current production
callers in this composite route through the context-aware path; if the remaining
test-only callers are migrated, delete the wrapper and call
`consolidateOneContext(context.Background(), ...)` at that one boundary. Verify
the full call graph before applying: if external/package tests rely on the
unexported helper, retain it. Estimated net: **-4 lines**, conditional. Deduction
opportunity: **-1**.

### 4. `shrink`: repeated topic/verdict snapshot row decoding

`internal/cli/tagpublish.go:137-185` manually decodes nullable topic and
verdict columns. The existing `store.TopicsForSession` and `store.VerdictFor`
already own the same row shapes, but cannot be called directly because this
path deliberately needs one read transaction for a consistent snapshot. The
smallest safe reuse is a store-level transaction-aware helper that accepts a
transaction/query interface and shares the scanners; do not replace these two
queries with separate existing helpers, or the mixed-snapshot red contract
returns. Estimated eventual net: **-20 lines**, but requires a new store seam;
do not attempt as an incidental cleanup. Deduction opportunity: **-1 advisory**.

### 5. `shrink`: repeated authoritative replacement SQL

`internal/cli/tagpublish.go:187-254` duplicates the shape of
`store.ReplaceSessionSegments` (delete the session, insert every segment), then
adds publisher authority/revision and context handling. The authority checks
make direct reuse unsafe today. If a transaction-aware store primitive is
introduced, move the delete/insert loop behind it and retain authority in the
caller; otherwise keep the duplication. Estimated eventual net: **-10 lines**;
not a safe standalone cut. Deduction opportunity: **-1 advisory**.

## Deliberately not findings

- `locateTagWriteFast` is not dead duplication: it is the source-only freshness
  seam that avoids consolidated locking and has a dedicated fence-held test.
- `isConsolidatedSource` cannot be replaced by basename-only
  `index.IsConsolidatedDB`; hard-link/symlink identity is a distinct contract.
- The reconciliation SQL in `consolidated.go` is source-scoped and protects
  co-contributors; whole-session deletion would regress the deletion red proof.
- `readTagSnapshot` must remain one transaction; two existing helper calls would
  reintroduce the mixed topic/verdict snapshot race.

## Verdict

The clearest immediate cut is the global session handoff and its variadic
plumbing (finding 1, with finding 2 counted only once): approximately **14
production lines deleted** while preserving all tested contracts. The scanner
and replacement-loop reductions need a deliberate store seam and should not be
smuggled into this composite.
