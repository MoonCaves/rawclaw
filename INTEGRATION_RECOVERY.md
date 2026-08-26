# Integration and recovery ruling

Base under attack: `8c8216e25e22496b2e3e919fce836be49d692e25`.

Immutable production fence resolved from `8c8216e^..8c8216e`:

```text
internal/index/consolidated.go
```

## Decision

**DROP production change. DEFER rewrite.** The commit changes only the stale-topic cleanup in
`consolidateOneContext`. Its predicate deliberately has two independent contracts:

1. `session_sources` proves whether the source is the sole known contributor.
2. `(session_id,start_uuid)` proves whether the refreshed source still offers that topic anchor.

The pre-existing `temp.consolidation_affected_sessions` set cannot replace either contract. It tracks
session membership for message/session deletion, not sidecar ownership. Deleting or broadening the
predicate reintroduces data loss when two source databases share a session but one refreshes its tags.

## Personally observed evidence

- `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'TestConsolidate_(DeletesTopicsRemovedFromSource|OriginAuthorityWinsForConflictingTopicSegments|HonorsTombstones|DistinguishesSourcesWithTheSameBasename)|TestConsolidateFrom_PreservesLegacySourceWhenCoContributorSkipped'`
  passed: `ok`, 4.002s.
- Graphify on the current tree resolves `SyncConsolidatedFrom → consolidateOneContext` and the
  `TagFile → SyncConsolidatedFrom` path through `gatherTagFiles → ingestForeignTags`; no existing
  recovery primitive replaces the topic ownership check.
- `graphify update .` rebuilt the graph at this base: 3,484 nodes, 10,455 edges, 154 communities.
- The requested supervisor artifacts were not present in this checkout or its immediate parent;
  the available two-supervisor harness copy lacked `supervisor-b.md`, `SCORECARD.md`, and
  `PRIOR_ART_LOG.md`, so no watermark-based prior-art claim is made.
- Push is **UNCERTAIN/BLOCKED by environment**: SSH timed out after 10s against GitHub. The local
  review commit remains `89420a3`; remote publication was not observed.

## Recovery surface not changed

No transaction boundary, restart path, fence, watermark, or crash-recovery mechanism was altered.
The reviewed SQL runs inside the existing per-source transaction and is covered by the existing
topic deletion and co-contributor tests. Adding a second recovery layer would violate the file fence
and duplicate existing ownership.

## Next proof required before any production edit

Add a disposable mutation or equivalent isolated reproduction showing a strictly smaller predicate
that still preserves a co-contributor's topic, deletes a removed sole-source topic, and leaves the
transaction rollback-safe. Without that proof, the smaller candidate is unsupported.

Net production reduction proven: `0` lines. Safe production alternative: none.
