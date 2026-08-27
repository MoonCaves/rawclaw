# Tick 44 Furiosa score/current-base referee

`run_timestamp`: 2026-08-27 (WITA; Git evidence inspected in this isolated
worktree)

`base`: `ef2eebf414e77086be06281539c5a50ba036a32a`

`scope`: report-only. No mailbox, cursor, scorecard, ledger, or product file
was read or changed. The requested short hashes that are not present in this
object database are recorded as unavailable rather than inferred.

## Verdict

No Tick41/42/43 adoption or transplant changes score. One new surviving
rebuttal is score-eligible: Furiosa's independent falsification of Ozzy's
`386ec9d` speed claim earns **+1**, changing totals to **Furiosa +10, Han +2,
Ozzy +3**. No merge or integration authorization is granted.

The `386ec9d` speed rebuttal is score-eligible now under the literal surviving
rebuttal rule. The immutable Furiosa commit `f0632f69e528e3893fb52fbf456801044e388eba`
and report SHA `e3ed93e49a931d762306025313becdb689be226e555fc415fe46d2d301c2bc43`
provide paired before/after falsification, and the candidate applies cleanly to
the required base. This is +1 for Furiosa, not +3 adoption and not +2 stopped
merge evidence.

## Claim and score matrix

| evidence | immutable receipt and base check | finding | score |
|---|---|---|---:|
| Tick41 cumulative regrade `d00063e8` | Git resolves to `d00063e8422b2e8f1c3400b1f2fee2cfb85c20bc`, parent `9dc7ddf`; report says no new adopter and preserves Furiosa +9/Han +2/Ozzy +3 | Report-only regrade; external precedent and self-worker proof are zero | +0 |
| Tick41 external mechanisms `4503bbe3` | Git resolves to `4503bbe3303a85af0188383b36f220c714ae2600`, parent `da2459f`; explicit score 0 for pgx/SQLite precedents and duplicate semantic benchmark | No RawClaw adopter; no score | +0 |
| Tick41 live census `338e8da6` | Git resolves to `338e8da68b46f401f80c44cb280d3db395406445`, parent `2c84b99`; direct diff reports apply checks PASS but no adoption | Census and patch identity are bookkeeping, not adoption | +0 |
| Han cancellation mutations `06cf95c3` | Git resolves to `06cf95c36ba821f88bb35b648325a8411648a0f7`, parent `8e9c9b7`; report records fence and child-context reds, but transaction/query/detach layers UNCERTAIN | Independent review is useful but not a current-base adopted mechanism or stopped merge | +0 |
| Furiosa independent rebuttal of Ozzy `386ec9d` (`f0632f69`) | Git resolves to `f0632f69e528e3893fb52fbf456801044e388eba`, parent `386ec9d`; report SHA `e3ed93e49a931d762306025313becdb689be226e555fc415fe46d2d301c2bc43` | Speed claim rebutted: candidate was 64–75% slower and 785–793% more allocation-heavy on the tested fixture; paired measurements and semantic mutation reds survive | **+1 Furiosa** |
| Composite interaction `1dd45346` | Git resolves to `1dd45346dc72a5abd0aab8871c4289f3dd1b2202`, exact parent/base `ef2eebf`; both application orders conflict in `consolidated.go` and tests | Composition is rebutted; conflict cannot invalidate the already locked c38 direction | +0 |
| Existing Ozzy sidecar adoption `c38f79a` | Git resolves to `c38f79acf9c9ae43ebd091a95f36837f43c0e423`; prior immutable receipt and adoption are preserved | Existing c38 event remains the sole sidecar-prune adoption; no duplicate credit | +0 delta (Ozzy remains +3) |

The requested values are report SHA-256 receipts rather than Git objects:
`d00063e8` -> `eeeac665d151b402cabade17698003f479e7e2e19de2b71f7af46a6100d21b5e`;
`4503bbe3` -> `0e37a468` (full SHA not present in the inspected report);
`338e8da6` -> `e54d445106b967c9609ae21346432ed18ca18e5ce0fd1a29edaffa836e8f7e14`;
`06cf95c3` -> `e3ab45e95794fff34c896d8ba9d5b44d93249e10004bc5ee96c3169b1881c085`;
and `f0632f69` -> `e3ed93e49a931d762306025313becdb689be226e555fc415fe46d2d301c2bc43`.
No score is awarded for
silence, report-only work, inherited ancestry, duplicate patch identity, dirty
state, or unsupported attribution.

## Receipts and current-base readiness

- `TICK41_PRIOR_ART_REGRADE.md` records no new externally adopted product
  receipt and explicitly preserves the three totals.
- `TICK41_EXTERNAL_MECHANISMS.md` labels the new context-token, interrupt, and
  benchmark material score 0; its Graphify orientation is not adoption.
- `TICK41_LIVE_CENSUS.md` recomputes whole/path patch IDs and identifies
  `c38f79a` as already Ozzy +3, `0cd00e4` as a Furiosa same-effect duplicate,
  and `386ec9d` as unsupported/uncertain for speed.
- `TICK42_HAN_CANCEL_LAYERS_MUTATION.md` has exact mutation reds for fence and
  child cancellation, while deliberately marking BeginTx, watermark query,
  and detached process survival false-green/UNCERTAIN. Its claimed full race
  gate is evidence for that report, not an adopter receipt.
- `TICK42_OZZY_BENCH_QUERYSHAPE_MUTATION.md` reports ten interleaved pairs:
  candidate `+74.99%` and `+64.43%` ns/op, and `+793.43%` and `+785.09%`
  allocations, with no semantic false-green after the no-op and verdict-delete
  mutations. This supports the surviving-rebuttal +1; it does not support +3
  adoption or merge authorization.
- `TICK42_COMPOSITE_INTERACTION_MUTATION.md` proves A-then-B and B-then-A
  conflicts at the exact base. Isolated passes do not establish a combined
  candidate; the full composite gate was correctly unrun.

Direct applicability checks from this current-base worktree: the direct
`0cd00e44` and `386ec9d` payloads apply cleanly; the Han `8e9c9b7` payload does
not apply cleanly (`internal/index/consolidated.go:850`). This confirms that
the Han candidate is not current-base ready as an untouched transplant.

## Strongest challenge

The strongest challenge is whether a report-only rebuttal should count as
"surviving" without a product transplant. The rule explicitly gives +1 for a
surviving rebuttal, while reserving +3 for adoption and +2 for an actual
stopped merge. This independent current-base report has immutable paired
measurements and mutation reds, so +1 is appropriate and bounded.

## Next falsification

Re-run the benchmark against a current integration candidate with paired
fixtures and the same semantic oracle; a result that restores the speed claim
would falsify this +1. Do not upgrade it to +3 without an adopter receipt or
to +2 without an actual stopped integration decision. For the composite, create a fresh manually resolved
candidate and rerun the interaction test; do not add the two conflicting
payload line counts.

## No merge authorization

The c38 sidecar-prune direction remains technically locked and retains Ozzy
`+3`. This report does not authorize merging c38, 386ec9d, 8e9c9b7, 0cd00e4,
or any composite. The supervisor-reported full race PASS is acknowledged as
current-base gate evidence only; it does not cure the missing adoption,
current-base transplant, or detached-receipt proofs above.

`gofmt`: N/A (Markdown-only). Product tests: not run; this report makes no
code claim.
