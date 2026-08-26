# Lenny modularity raid

Base: `0d60b4c81a3fbdb29e63d3a55869214d2686cdeb`.

## Rival audit

- `dd57060` extracts the repeated consolidation phase closure into `beginConsolidatePhase`, preserving phase names and start/duration logs. Its helper branches on `src == ""` and duplicates the entire start/finish logging pair for root and source-backed phases.
- `fc1a075` extracts `resolveSegmentRange` and adopts `slices.Backward`, which is already the stdlib-minimal shape for the range conversion. It also swaps `refreshTagSession` to `LocateConsolidatedSession`; that removes `LocateSession`'s documented per-project fallback when the consolidated store is missing, empty, or has no matching row. I am not touching this seam: the behavior risk is larger than its deletion opportunity, and its `cmd_tag.go` change also crosses a separate concern.

## Selected seam

Take over only `dd57060`'s consolidation phase logger. Keep the public phase names, source attribute presence/absence, start event, duration event, close/detach timing, and deferred error paths. Replace the helper's duplicated logging branches with one logger enriched by `slog.Logger.With("source", ...)` when a source exists.

This is a concrete deep module with one small interface: callers still receive only a `func()` completion hook, while source enrichment and event formatting stay local. No new interface or adapter is justified.

## Expected result

- Baseline `consolidated.go`: 1,601 lines.
- Lenny's `dd57060` source delta: 39 additions, 48 deletions (net `-9`; the receipt reported `-10` including surrounding accounting).
- Proposed helper: remove the duplicated `if src != ""` log bodies while preserving attributes and call sites; expected net at least one line better than `dd57060`.
- If source or test receipts show any logging-contract drift, reject this takeover and report NO TAKEOVER.

