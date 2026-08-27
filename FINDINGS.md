# T52 integration recovery findings

Base: `48661f403f880e2c1dac7615f39bbb8264eeafe7`

## Ponytail Review

- `internal/index/consolidated.go:402-645`: `shrink:` keep one writer path and reuse the existing `AcquireConsolidatedFence`; add only the smallest process-local context gate needed before SQLite admission.
- `internal/index/consolidated.go:807-948`: `delete:` do not add a test-only production hook or a second publication mechanism; the existing transaction and meta table are sufficient primitives.
- `internal/index/consolidated.go:1374-1390`: `shrink:` watermark publication must use the transaction already owning the fold; do not introduce a new receipt type or abstraction.

Net target: minimum necessary delta; no new dependency, interface, or test hook.

## Routed method rules that changed this method

### `golang-testing`

- Treat the test as an observable contract: cancellation must return `context.Canceled` within 250 ms, leave session and sync-watermark state unpublished, and allow one successful retry after release.
- Use deterministic isolated fixtures with two independently opened database pools; do not rely on test ordering or a production-only hook.
- Run an anchored exact-one `go test -list` preflight before the focused `-run`, and use the race detector for focused and package gates.

### `golang-safety`

- Keep transaction ownership explicit: every failed or canceled path must roll back and close resources, with no partial session or watermark publication.
- Preserve context and ownership through API calls; do not allow a canceled operation to publish a success watermark after its transaction ends.
- Avoid introducing shared mutable state without a clear zero-value-safe, synchronized ownership model; the admission primitive must be safe under concurrent writers.

### `golang-troubleshooting`

- Reproduce the real SQLite writer-lock symptom on the exact base before changing code.
- Test one hypothesis at a time: first transaction statement cancellation, then process-local admission, then commit-bound watermark publication.
- Trace the complete caller/data flow and report unentered layers as uncertain; do not treat a green test hook or a package-wide green as proof of the lock contract.

## Decision boundary

The current test only proves cancellation while waiting for the file fence. It does not enter SQLite first-write admission, transaction statement cancellation, or transaction-bound watermark publication. Production changes are justified only if the new two-pool test observes all three layers and the retry proves exactly-once publication.
