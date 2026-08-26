# Furiosa Tick 19 harvest

Date: 2026-08-27 WITA
Exact comparison base: `8c8216e25e22496b2e3e919fce836be49d692e25`

## Method constraints applied

Before source inspection I read the required `golang-how-to`, `golang-testing`,
`golang-safety`, `golang-troubleshooting`, `golang-cli`, `golang-documentation`,
`golang-concurrency`, `golang-context`, and `graphify` skills completely. They changed
this review in concrete ways: treat tests as observable contracts; require race and
mutation evidence rather than self-reported green; inspect nil, aliasing, resource,
context, channel, and cancellation hazards; preserve CLI stdout/stderr and exit-code
contracts; keep the report concise and evidence-backed; and use Graphify for orientation
before literal Git/source inspection. Graphify was run against the canonical graph after
`reflect --if-stale`; `query 'consolidate overlay sidecar nil scope'`, `explain
'ConsolidateFrom'`, and `path 'ConsolidateFrom' 'runTagWriteCmd'` were useful orientation.
Graph relationships below are not proof; Git ancestry, patch IDs, and receipts are.

## Candidate census and immutable Git evidence

| Candidate | ancestry against base | whole-commit patch-id | disposition |
|---|---|---|---|
| `34d2fb05161b1be7819b80804fca2e3a576243cf` | exact chain `8c8216e -> 88e73b5 -> 878f631 -> 34d2fb0`; `34d2fb0` is a docs receipt atop the two product commits | `21194acb3fe3836226a00e72273de0a5bf47ac0f` | **CURRENT INTEGRATION CANDIDATE** |
| `5eb3a38309a5319befaa434483bbf97004312129` | separate same-base product implementation via `b0126f2` | `aabc9e67b3c8adc28561be43f735b64a8c67eb03` | product payload is semantically represented by `88e73b5`; duplicate for integration |
| `3b641ce9582541a60a7b37c8456bedaa9d86d29c` | separate branch via `b2280fc`; not the composite ancestry | `108af27e33932a0d77585f7631de76dcfd2f451` | contract variant; no need to replace the composite |
| `96aa522611fdcb78e281db31634144e40222de91` | descends from `fb99037`, not from the exact current candidate chain | `d54fa75907a2cb2b5bb823d101fe3d385ac6c775` | sidecar candidate, stale/overlap-heavy and incomplete without absent-table handling |
| `c38f79acf9c9ae43ebd091a95f36837f43c0e423` | descends from `96aa522` | `6a62ff59b1b20a5873006b17ce72cd64229f65a6` | newest sidecar refinement; unique but not externally adopted or independently current-base gated here |
| `d91870634ff42b11165811111442acab26244d39` | separate nil-scope branch, not current candidate ancestry | `5133121d630d549c255f82606b13c1012c6c748f` | **REJECTED**: removing the consolidated fence risks snapshot-replacement data loss |
| `ef9f6ab31530bc689329e8058936473b6ae27601` | separate nil-scope branch, not current candidate ancestry | `086cb5a32567201d650ddee0405b3a60d7372803` | **REJECTED** for the same fence-safety reason |
| `e43127edc1d35e111c8b0fa5bcb19a8cb59b26ce` | descends from `fb99037`, not current candidate ancestry | `ce9dc82e0c222cc51d3424581dc811c6b6814ea6` | **REJECTED** by hostile missing-catalog/arbitrary-dir/symlink/held-fence evidence |

The shrink product patch is genuinely the same implementation as the composite's
`88e73b5`: both have stable patch ID `73f5dd69a25ee9f6e39bcd2036397b46661d741b`.
Likewise, the best-effort contract implementation has stable patch ID
`b2d5b3e2afbfef8ef404e95e356b38f5d8d35bcc` in both `878f631` and `3b641ce`'s product
path. Chronology is binding: `fb99037` is timestamped 05:07:29 WITA and `5eb3a38`
05:27:22 WITA, so any claim that the latter independently implemented the former is
convergence/prior implementation and scores zero.

## Current-base and upstream state

The composite branch `worker/furiosa-composite-t17b-20260827` points at
`34d2fb0` and is equal to its origin branch. The best-effort branch
`worker/furiosa-best-effort-contract-20260827` points at `3b641ce` and is origin-equal.
The sidecar and nil-scope branches have pushed refs, but their payloads are separate
lines and are not descendants of the composite candidate. The current checkout is a
clean exact-base worktree before this report; no product files were modified.

The supervisor supplied immutable receipts for the composite focused race (`PASS`,
7.32s) and full race (`exit 0`, 113.16s). Those receipts are corroborated as state
claims, but this referee did **not** run those gates and does not claim to have observed
their execution. The integration-winner mailbox receipt independently identifies
`8c8216e` as clean/upstream-equal and selected/publishable while explicitly retaining
the terminal-receipt limitation.

## Han/Ozzy adoption and Direction Lock

Newer Han/Ozzy receipts do not satisfy Direction Lock for `34d2fb0`:

- Han's `8e9c9b7` is a separate stale-lineage overlay/consolidation candidate, not an
  implementation of the composite payload.
- Ozzy's `96aa522`/`c38f79a` sidecar work is a distinct mechanism family. The Han duplicate
  audit explicitly withdraws the five older Ozzy product adoption claims; `c38f79a` is
  only a plausible unique follow-up pending its own current-base gates.
- Ozzy's `e43127e` was explicitly rejected by the hostile missing-catalog, arbitrary-dir,
  symlink, and held-fence checks.
- `d918706` and `ef9f6ab` contradict the retained consolidated-fence safety contract and
  are not adoptable.
- The earlier external overlay adoption receipt for Han's `5eec12b` concerns the older
  `e6f22f1` composite-key mechanism, not this current detached-publication composite.

Therefore there is no qualifying external adopter receipt for this exact recommendation
with the required decisive focused and full gates. **Direction Lock: NOT ELIGIBLE.** No
score change and no merge authorization are claimed.

## Final ruling

`34d2fb05161b1be7819b80804fca2e3a576243cf` is the current integration candidate because
it is the exact-base composite and preserves the best-effort publication contract plus
the dead-overlay shrink. `5eb3a38` and `3b641ce` are duplicate/alternate representations
of payload already in that candidate. `96aa522`/`c38f79a` remain separate, unadopted
sidecar work; `d918706`/`ef9f6ab` and `e43127e` are rejected. The terminal publication
receipt gap remains open and is an integration invalidation risk.

No merge or product edit was performed.
