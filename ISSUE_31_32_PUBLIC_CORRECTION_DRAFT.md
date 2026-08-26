# Public correction draft for Issues #31 and #32

Prepared from the live public issue state on 2026-08-26. This is draft text
only; no GitHub comments were posted.

## Issue #31

Suggested correction comment:

> Correction to the closeout history: `d5d036b` is deletion-only. It removes
> the 57-line duplicate timing test in
> `internal/index/consolidated_logging_test.go`; it does not supersede the
> accepted phase-log contract in `2ee9950`. The canonical #31 implementation
> and contract test remain `2ee9950` (with its required start markers and typed
> durations, including failed fence acquisition). `d5d036b` should be read only
> as a later test cleanup, not as the implementation or replacement for
> `2ee9950`.

The live issue is **CLOSED**. Its current close comment names `2dca4d9` and
`2ee9950`, so the correction should preserve that wording and clarify the role
of `d5d036b`; no reopening is needed.

## Issue #32

Suggested correction comment:

> Correction to the reproduction evidence: `cece0a5` is the corrected
> same-store negative reproduction. It mutates the source after the forced
> post-merge exit so the retry performs real merge work, while restoring the
> same isolated HOME/store in the child and parent. The race/shuffle gate ran
> five repetitions and **PASS**ed: wall time `3.99s`, package time `3.484s`,
> and observed retry duration `143.494833ms`. The multi-second stall did **not**
> reproduce. This result is distinct from the full baseline `0d1da19`, which
> also passed its repository gate; the negative reproduction is evidence about
> the suspected stall, not a claim that the baseline was changed or fixed.

The live issue is **CLOSED**. Its existing comments first cite `c14e806`, then
correct the same-store test to `fd01a92` and `479d14c`; those pointers are stale
for the final corrected reproduction and should be replaced or superseded by
the `cece0a5` pointer and the exact measurements above.

## Stale claims requiring correction

- #31: any wording that treats `d5d036b` as the implementation or as a
  replacement for `2ee9950` is incorrect. It is deletion-only.
- #32: the first `c14e806` comment is explicitly invalid for same-store
  recovery; the later `fd01a92`/`479d14c` pointers are superseded by the
  corrected `cece0a5` evidence requested here.
- #32: comments that omit the second-message mutation and exact timing should
  state that the retry performed real merge work and did not reproduce the
  multi-second stall.
- #32: the negative reproduction and the full-baseline `0d1da19` pass are
  separate facts and must not be merged into one performance or fix claim.

## Live-state verification

`gh issue view 31 --comments --json number,title,state,body,labels,comments`
reported #31 **CLOSED**, titled “Add phase-level timing instrumentation to the
post-merge fold tail,” with the current close comment naming `2dca4d9` and
`2ee9950`.

`gh issue view 32 --comments --json number,title,state,body,labels,comments`
reported #32 **CLOSED**, titled “Fault-injection repro test for the Problem B
single-source closeout stall,” with the stale `c14e806`, `fd01a92`, and
`479d14c` history described above.

