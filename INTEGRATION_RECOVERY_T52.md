# Han integration recovery T52

Base: `48661f403f880e2c1dac7615f39bbb8264eeafe7`

## Contract

`TestConsolidate_ContextCancellationDoesNotPublishAndRetryPublishesWatermark` uses two independently opened RW pools. One holds a real SQLite transaction on `consolidated.db`; the other runs `SyncConsolidatedFromContext` with cancellation at 200 ms while writer admission is occupied. The canceled operation must return `context.Canceled` by 250 ms, leave the new message and session state unpublished, and retain the prior sync watermark. After the lock and admission token are released, one retry must fold the message once and replace the sync watermark only through the committed transaction.

## Evidence

- Exact-one discovery: `CGO_ENABLED=0 go test ./internal/index -list '^TestConsolidate_ContextCancellationDoesNotPublishAndRetryPublishesWatermark$'` — one match.
- Focused red reproduction before the fix: the existing non-context writer path missed the 250 ms bound and returned only after lock release (`context canceled` observed after the timeout).
- Focused green gate after the fix: `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestConsolidate_ContextCancellationDoesNotPublishAndRetryPublishesWatermark$'` — PASS.
- Finder gates: `/Users/jay-m4/go/bin/dupl -t 100 internal/index/consolidated.go internal/index/consolidated_test.go` — zero clone groups; `/Users/jay-m4/go/bin/golangci-lint run ./internal/index` — zero issues.

## Mechanism

`consolidatedWriterGate` is a process-local, context-aware one-token admission gate placed before the existing cross-process `consolidated.lock`; this prevents RawClaw writers from entering the driver's unbounded SQLite busy-wait after cancellation becomes possible. The context-aware fold path uses `ExecContext` and `QueryRowContext` for its SQLite operations, while the sync watermark remains inside the fold transaction and is published only after `Commit` succeeds. No new dependency, global test hook, or second publication store was added.

## Routed method application

The testing rule supplied the observable timing/publication/retry assertions and exact filter preflight. The safety rule required rollback, resource cleanup, and no post-cancellation publication. The troubleshooting rule required a red real-lock reproduction before the one-hypothesis fix and kept untouched layers explicitly out of the claim.

Status: implementation evidence only; no score claim, merge authorization, or Direction Lock.
