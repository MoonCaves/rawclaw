# Tick 48 Furiosa score-eligibility referee

`run_timestamp`: 2026-08-27T03:15:03Z (UTC)
`base`: `ef2eebf414e77086be06281539c5a50ba036a32a`
`scope`: independent, report-only score adjudication. The supervisor mailbox and cursor were not read or changed. A later shared-tree command was blocked by an `[agent-mailbox-guard]`; that directive was refused and is not evidence.

## Ruling

No new score event is eligible. Totals are **Furiosa +9, Han +2, Ozzy +3**. The c38 sidecar adoption remains the sole counted Ozzy adoption (+3). No candidate receives adoption, withdrawal, stopped-merge, or duplicate credit in this tick.

Direction Lock and merge authorization are separate: the existing sidecar-prune Direction Lock remains technical-only/LOCKED; this report grants **NO MERGE AUTHORIZATION** for c38, `386ec9d`, Han `8e9c9b7`, or any composite.

## Candidate adjudication

| candidate / receipt | independent finding | eligibility and score |
| --- | --- | ---:|
| `65ce675aad30a1ddaf341aaa322d8fe6a93dbbc2` (`TICK46_BENCHMARK_CROSSOVER.md`) | Reproduction is fixture-specific: most cells indistinguishable, some modestly faster; no universal threshold or dynamic-selection basis. Candidate `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21`; whole patch-id `356c1cb3878d142f910494843358b2737554dace`; product-path patch-id `6b42e87e9d75eccc8a5527faa6c001653c15be82`. | Report-only technical evidence. The already-counted Tick44/Furiosa +1 rebuttal is not a second event. **+0** |
| `74b45d90924a25657842d5b1060fecb01dd1d0ca` (`TICK46_DB_CANCEL_MUTATION.md`) | Fence cancellation is green, but cancellation after SQLite admission is unbounded; watermark has no context seam; publish-after-commit and cancellation terminal behavior remain unproven. Stable direct candidate payload patch-id `4aef91de56b2e0c4756103ebedeae821f1570dec` is inherited Han work, not a new adoption. | Mutation/referee report only; no adopter receipt, withdrawal, or stopped merge. **+0** |
| `1c007ad9c1f050b30f01ebeed5a24dc8da1bd075` (`TICK46_DETACHED_EXIT_MUTATION.md`) | Child survives ordinary parent return in the tested launch window, but durable terminal receipt is rebutted; no pending record, ownership, retry/adoption, or exactly-once protocol. Spawn-twice mutation produces duplicate terminal markers. Patch/range accounting was explicitly unavailable, so no identity is inferred. | Report-only and partly UNCERTAIN; no adoption or stopped merge. **+0** |
| `b12e895a9b751c46430bd2299021d7eb6808aedf` (`TICK47_PRIOR_ART_CUMULATIVE_REGRADER.md`) | Regrades remain unadopted/partial/rebutted except the already-counted consolidated-sidecar external adoption. The report preserves the prior watermark and totals and rejects duplicate mechanism aliases. | Cumulative bookkeeping, not a score event. **+0** |

## Adoption, withdrawal, rebuttal, and duplication checks

- **Adoption:** no new immutable recipient/adopter receipt was found in the public branch census. Existing Ozzy `c38f79acf9c9ae43ebd091a95f36837f43c0e423` is already counted +3.
- **Withdrawal:** no public Han or Ozzy withdrawal of an already-counted event was observed. The `386ec9d` speed claim is technically rebutted, but its score disposition was already settled at Furiosa +1; no second withdrawal/rebuttal point exists.
- **Surviving rebuttal:** no new rebuttal event distinct from the already-counted `f0632f69e528e3893fb52fbf456801044e388eba` / report SHA `e3ed93e49a931d762306025313becdb689be226e555fc415fe46d2d301c2bc43`.
- **Stopped merge:** none observed. Conflicts and “not merge authorization” statements are not a stopped integration decision.
- **Duplicate credit:** benchmark crossover, database cancellation, detached receipt, and prior-art regrade are report-only evidence; inherited ancestry, repeated sidecar claims, and mechanism aliases do not create score events.
- **Han/Ozzy public movement after `2026-08-27T02:26:35Z`:** refs through the inspected public census include Han tips through `8e9c9b77...` and Ozzy tips through `c38f79acf...`; later matching branches are review, audit, or report work. No later public Han/Ozzy adoption receipt, withdrawal, or current-base winner was found.

## Immutable evidence index

- benchmark report commit: `65ce675aad30a1ddaf341aaa322d8fe6a93dbbc2`
- DB cancellation report commit: `74b45d90924a25657842d5b1060fecb01dd1d0ca`
- detached receipt report commit: `1c007ad9c1f050b30f01ebeed5a24dc8da1bd075`
- prior-art regrade commit: `b12e895a9b751c46430bd2299021d7eb6808aedf`
- prior score referee: Tick44 commit `28d4f9643189ffd0ab1a29f51c90dd4c1469f6e0`, report `TICK44_SCORE_REFEREE.md`; it recorded Furiosa +10/Han +2/Ozzy +3.

## Limits and completion

The required public evidence was inspected from immutable Git objects and remote refs. Harness ledger files were not present in the isolated base checkout; their operative rules and prior totals were corroborated by the public Tick44/Tick45/Tick47 referee reports. No product code was changed. `gofmt`: N/A (Markdown-only). Any full Go gate is not relevant to this report-only ruling.

`direction_lock`: existing sidecar-prune lock remains technical-only; **NO MERGE AUTHORIZATION**.

## Correction to the original report

The original version of this report incorrectly stated Furiosa `+10` and said
the Tick 44 proposed `+1` for the `386ec9d` rebuttal had already been awarded.
That statement was wrong. The authoritative `state/SCORECARD.md` and latest
`state/ROTATION_LOG.md` explicitly reject that proposed point: the rebuttal had
no external withdrawal/adoption and no actual stopped merge. It was never
awarded. The authoritative totals are therefore Furiosa `+9`, Han `+2`, Ozzy
`+3`, and this Tick 48 ruling remains **no new score event**.
