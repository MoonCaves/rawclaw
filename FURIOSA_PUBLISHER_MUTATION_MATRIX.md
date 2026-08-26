# Furiosa publisher hostile check

Base: `8c8216e25e22496b2e3e919fce836be49d692e25`.

The single hostile case `TestTagWriteDefaultScopeConsolidatedOnlyDoesNotBlock`
seeds a session only in `consolidated.db`, holds `consolidated.lock`, and calls
`runTagWriteCmd` with `scope=nil`. The winner blocks in
`AcquireConsolidatedFence` until the 300 ms timeout, so the test fails. This
survives the current coverage and is a real nil-scope publication defect, not a
mutation killed by an existing test.

Focused command:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^TestTagWriteDefaultScopeConsolidatedOnlyDoesNotBlock$'
FAIL (1.826s test process; 0.38s test)
```

No production files were changed. No broad matrix was added after supervisor
steer; this report contains the one permitted hostile case.
