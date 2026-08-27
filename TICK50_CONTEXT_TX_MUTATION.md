# Tick 50 context transaction mutation referee

Target base and candidate: `8e9c9b77e1eed984bfa847b9d613f263e2d46dd2`.

## Verdict

**CONFIRMED for the fold transaction seam.** A deterministic real SQLite
writer-lock probe on the untouched candidate showed that cancellation is not
observed at the first blocking fold write. The earlier SyncConsolidatedFromContext
referee measured 303.837125 ms against a 250 ms bound; the direct fold probe
also measured about 302–304 ms.

The smallest disposable mutation changed every fold transaction `tx.Exec` to
`tx.ExecContext(ctx, ...)` and added a test-only pre-merge barrier so the lock
was acquired exactly at transaction entry. The same probe then returned
`context.Canceled` within the bound. After releasing the lock, the session
count remained 1 and the existing `sync:<source identity>` watermark remained
unchanged, demonstrating rollback/no publication for this cancellation path.

This supports `PA-GO-CONTEXT-WRITER-TOKEN-001`. It is not a complete product
fix: schema migration, attach/detach, prune, and watermark publication still
use non-context APIs in the surrounding path. A transaction-bound,
context-aware `StampIngestWatermark` was **not tested** here; watermark
publication remains a separate unproven seam. Terminal success after commit
was not independently tested.

## Exact evidence

Focused preflight listed exactly one disposable test:

```text
CGO_ENABLED=0 go test -list '^TestTick50ContextTxMutation$' ./internal/index
mutated exit 0
output SHA-256: d5ef13fbb55bbf3e407bc44a275725cf86a84d933353d174f5f35c1d8a942777
```

Untouched candidate, with the disposable test present but no product mutation:

```text
CGO_ENABLED=0 go test -list '^TestTick50ContextTxMutation$' ./internal/index
exit 0
output SHA-256: 9d83cf22fa1f52df5ed50e3eb16d516be21e197e009241ef1087f2ae

CGO_ENABLED=0 go test -race -count=1 -timeout=15s -run '^TestTick50ContextTxMutation$' ./internal/index
exit 1 (expected adversarial red)
output SHA-256: 5e9a9eb6a7a2cb1e4016b2b2de76672226e158819d7a8f31122c860043ca428e
observed: sync error = <nil>, want context.Canceled
```

The direct lock-held variant on the untouched candidate timed out beyond the
250 ms bound at 302.336 ms; another run measured 303.228875 ms. It unwound
only after the lock was released.

Mutated candidate:

```text
CGO_ENABLED=0 go test -race -count=1 -timeout=15s -run '^TestTick50ContextTxMutation$' ./internal/index
exit 0
output SHA-256: dca64f9926edab12e0c0c4f3f50f5d71c9bc9d64cf38c54367f80a359ba3db42
observed: cancellation returned within 250 ms
observed: session count remained 1
observed: sync watermark remained unchanged
```

Disposable product patch was 19 insertions and 15 deletions in
`internal/index/consolidated.go`.

```text
direct patch SHA-256: e4e89af06b29613eb8556fac70c7d4af7451ef1a11484b99129029bf44047101
stable patch-id:      0e2186b33ec762a1cba96a64a3e5b30741cfe33a
test patch SHA-256:    d971a8d3b835e0fd9ff31c4de2efae1c575ef33638f571a58f6557b1356a7c22
```

The exact disposable diffs were preserved during the experiment at
`/tmp/t50-product.patch` and `/tmp/t50-test.patch` on the referee host. The
first contains the complete `consolidated.go` diff; the second contains the
complete disposable test file diff. Their hashes above are the reproducibility
anchors. They were not copied into the final branch because the branch is
report-only.

## Restoration and scope

All product and disposable test Go edits were restored. The final branch is
intended to contain only this report. No product fix or merge authorization is
included.

The post-restoration full package gate was started, but the local mailbox guard
interrupted shell verification before a result was returned; it is therefore
**UNCERTAIN**, not claimed green. Prior exact-candidate evidence recorded the
full race suite green in `b479a9061f27686a2179d28af9b97f83e3a65040`.
