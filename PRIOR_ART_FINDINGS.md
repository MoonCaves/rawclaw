# Tick 29 live prior-art census

## Watermarks and mandatory re-grade

The last authoritative prior-art ledger watermark is `20260826T232815Z`.
The Tick 28 live evidence freeze is `20260826T235330Z`; it is a candidate
watermark for this report, not a claim that the prohibited parent mailbox was
processed. The three Tick 25 recommendations were re-graded from the ledger
after the corrected watermark:

| recommendation | fingerprint | current status | score |
|---|---|---|---:|
| `PA-SQLITE-BEGIN-IMMEDIATE-001` | `7e0c7cf321c3b845baf8518173f636fdf86eda2d81b69cd8b00b635cff80b214` | pending; no external adoption | 0 |
| `PA-FTS5-DELETEMERGE-001` | `21ae4bb81dff3c0531f08b62290258442aaea11b75949aeb2e7bca2996e240a2` | pending; no external adoption | 0 |
| `PA-GO-SINGLEFLIGHT-FALLBACK-001` | `1532e53cf1b582d958f6fec89bcb723cf2da7681bc696a5b7cfbc0fe4bf3465a` | pending; no external adoption | 0 |

No adoption receipt, immutable adopter SHA, or score-eligible event was found
for any of the three. The sidecar Direction Lock remains technical-only and
does not authorize a merge.

## Current live census and deduplication

Current refs and receipts add no distinct implementation effect beyond the
locked sidecar direction and previously recorded rival work:

- `c38f79a` is the source-without-sidecar-tables implementation; `0cd00e4` is
  its current-base adaptation. Their whole stable patch IDs are respectively
  `6a62ff59b1b20a5873006b17ce72cd64229f65a6` and
  `57bdcd672364438b3b898f35d6f60c7cc178f5ca`; the differing IDs reflect
  parents/tests, not a second mechanism.
- `a78b39b` is the rejected sidecar variant. Its production path ID
  `ac2dbbbf06cdc226ade47b39b1e636a48f3cbdab` is shared by the duplicate
  `96aa522`/`a62ab05` family. Treat these as one effect, not separate wins.
- Ozzy `bc8af914` is docs-only (`docs/design/tombstone-consolidation-contract.md`,
  +12/-10; stable ID `1b46d699f573efb107f8825f983771b4c9161d61`). Its inherited
  code is ancestry, not new payload.
- Han's latest implementation refs remain older tag-overlay/detached-publisher
  experiments (`8e9c9b7`, `cabab43`, `d2315cb`, `daaf973`). They are distinct
  effects from sidecar pruning, but none supersedes `0cd00e4`; the latest Han
  Tick 28 claim-spy `b2aa76fde37da4d4ab2f4906caf5af10a3707ec3` is report-only.
- Ozzy Tick 28 claim-spy `b799c65bdbad268fe2dc90986ecff2f61826ffc0` is
  report-only. It confirms `bc8af914` is docs-only and keeps `386ec9d`'s speed
  claim UNCERTAIN: whole ID `356c1cb3878d142f910494843358b2737554dace`,
  `internal/index/consolidated.go` ID
  `6b42e87e9d75eccc8a5527faa6c001653c15be82`, with no fair old/new baseline.
- Post-freeze audit refs (`81e51b83`, `b2aa76fd`, `b799c65b`) contain only
  findings. Stable patch identity and `git range-diff` therefore cannot turn
  them into product novelty.

The locked base remains `878f631b74e68aa76302f382e28096dc3d60b545`; source
winner `c38f79acf9c9ae43ebd091a95f36837f43c0e423`; adaptation
`0cd00e44c7eb87e30fcf72f8ae790e7060635b09`; loser
`a78b39b3d87c82a4f83878359afc98e2b8fde2d4`. No current candidate changes the
locked ancestry or applicability. Invalidation triggers remain: base change,
production patch change, mutation failure, focused/full gate regression, or
adoption-receipt invalidation.

## Three unresolved external-mechanism questions

Ordered by integration value, not novelty score:

1. Can SQLite `BEGIN IMMEDIATE` be applied at every RawClaw rebuild/tag/vector
   writer boundary without holding a transaction across scans, and with
   retryable `SQLITE_BUSY` semantics?
2. Can FTS5 `deletemerge` plus bounded merge work reduce closeout latency while
   preserving source-scoped sidecar eligibility and observable incomplete work?
3. What key/generation must `singleflight.Group.Do` include so duplicate
   fallback writers coalesce without sharing stale freshness results, while
   durable cross-process ownership remains separate?

These are unresolved adaptation questions only. They remain score `0` absent
external adoption and are not implementation authorization.

## Final ruling

**NO SCORE CLAIM.** The live census finds no novel score-eligible mechanism;
all current additions are documentation, audit evidence, inherited ancestry, or
already-deduplicated effects. No product files were changed.
