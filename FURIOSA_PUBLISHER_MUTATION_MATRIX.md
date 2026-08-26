# Furiosa publisher hostile check

Base: `8c8216e25e22496b2e3e919fce836be49d692e25`.

The single hostile case `TestTagWriteDefaultScopeConsolidatedOnlyWaitsForFence`
seeds a session only in `consolidated.db`, holds `consolidated.lock`, and calls
`runTagWriteCmd` with `scope=nil`. The winner correctly remains blocked while
the fence is held, then completes after release. This is required because
consolidation snapshot-and-renames the store; an unfenced direct write could be
discarded. The initial "must not block" assertion was an invalid expectation,
not a production defect.

Focused command:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^TestTagWriteDefaultScopeConsolidatedOnlyWaitsForFence$'
PASS (expected fence wait, then completion)
```

No production files were changed. No broad matrix was added after supervisor
steer; this report contains the one permitted hostile case. Verdict: nil-scope
consolidated fence behavior is sound; no harvest recommendation.
