# T56 ExecContext minimal cancellation comparator

Date: 2026-08-27 WITA

## Scope

- Base: `48661f403f880e2c1dac7615f39bbb8264eeafe7`
- Han candidate: `0cd0b9ce77e362b5bd4e973f948eb9981cdbf452`
- Correction checked: `7d1ca1c643795a145db1e33d657192993ff8fd78`
- Dedicated test: temporary `internal/index/t56_execcontext_comparator_test.go`, removed after runs
- Exact-one preflight: `go test ./internal/index -list '^TestT56ExecContextComparator$'` reported `exact-one count: 1`

The harness first completed an ordinary production sync, then changed the source, began an independent transaction on a separate `ConnectRW` pool, and acquired the real SQLite writer lock only after the production call completed `schema-migrate`. It never acquired or invoked `consolidatedWriterGate`. Cancellation followed 50 ms later. While held, the consolidated message count and sync watermark were checked; the new message was absent and the old watermark remained. The holder was then rolled back, and the blocked call was joined before one retry.

## Results

| Variant | One-variable patch | Result |
| --- | --- | --- |
| Untouched base | none | RED: call remained blocked `354.335292ms` after cancellation; test failed `canceled call exceeded 300ms` |
| Full Han | `c629814515d856bd5d93db83c19ca52838231fcb` | RED: same harness remained blocked `352.484375ms` |
| Full context, global gate/init and admission calls deleted | `1e37a069462cc5715a6720e21be360fa233302cd` | RED: remained blocked `351.222875ms` |
| Minimal transaction-only `tx.ExecContext(ctx, ...)` subset | `12cebe4d80999da9fe420d2e70b6918c38b98adb`, `17/17` lines | RED: remained blocked `352.125083ms` |

Observed commands for each mutant were the same:

```text
gofmt -w internal/index/consolidated.go internal/index/t56_execcontext_comparator_test.go
go test ./internal/index -list '^TestT56ExecContextComparator$'
go test -v ./internal/index -run '^TestT56ExecContextComparator$' -count=1 -timeout=20s
```

The full Han run and both reduced context variants reached the same production phases (`schema-migrate`, `source-migrate`, `attach`, `prepare`, `merge`) before the real SQLite writer boundary. The relevant first transaction call is the `modernc.org/sqlite` driver path behind `tx.Exec` / `tx.ExecContext`; changing the Go method did not make that busy wait return by the 300 ms bound. `BeginTx(ctx, nil)` still allowed eventual rollback/unwind after holder release.

## Verdict

`consolidatedWriterGate` is not necessary or sufficient for bounded cancellation at the SQLite writer boundary. The broad Han patch and the smallest compiling `tx.ExecContext` substitution are both behaviorally equivalent under this production-path lock harness: neither produced bounded `context.Canceled` before release. The candidate's green admission test therefore proves only process-local gate cancellation, not driver-level SQLite cancellation.

The harness directly observed no message publication and preservation of the old sync watermark while the holder was active, followed by release and a successful retry. Session-row publication was not independently varied because the test uses an existing session for the appended message; no claim beyond the observed message/watermark checks is made here.

No product integration is authorized. Product/test edits were restored; only this report and `FINDINGS.md` remain as permanent changes.
