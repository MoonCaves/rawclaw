# Tick 24 claim-spy findings

Audit base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`.

## Claim table

| Receipt / claim | Verdict | Immutable evidence and ruling |
|---|---|---|
| `20260826T225546Z-354d06bc-tick-23-harvest-score-adjudica.md` — Furiosa +11, Ozzy +3, provisional Direction Lock | **REBUTTED** | SHA-256 observed: `dee144c1a7c574d65afaa6dee5073de865a2d3ed58ec031a24398242e32e62b0`. The receipt itself says the awards and lock were provisional. Its Furiosa +2 component is later withdrawn by the correction and score referee because 0cd is the same c38 adoption event; it cannot coexist with Ozzy +3 under the no-double-count rule. |
| `20260826T225831Z-164d1acc-correction-tick23-awards-and-l.md` — withdraw finality pending chronology, duplicate review, and fresh gate | **CONFIRMED** | SHA-256 observed: `d195ceea3a218e46c624159f3b3cc20cd821d857a736e42f6f050fd3b5f20b38`. Its three pending conditions are closed by the c38/0cd commit timestamps, referee duplicate ruling, and root gate log. |
| `20260826T230511Z-32270742-tick-24-claim-spy-final-score-.md` — Ozzy +3; Furiosa +2 rebutted; totals Furiosa +9, Han +2, Ozzy +3; technical lock, no merge | **CONFIRMED** | SHA-256 observed: `da05e5c6f10cf302091c088e4eb2909f91b152319f35d0ef233c6f298c18d2c3`. c38 is commit `c38f79a...`, on `ozzy/fresh-luna-adversarial-20260827`, committed 21:38:03Z; 0cd is `0cd00e4...`, on `worker/furiosa-final-current-base-20260827`, committed 21:40:25Z: 142 seconds later. Score referee `referee/furiosa-score-t22-20260827@0d237b1...` expressly rebuts Furiosa +2 as duplicate. |
| Attribution receipt `20260826T223543Z-5c485cbf-tick-21-c38-hostile-contract-c.md` and adoption receipt `20260826T224735Z-5b061dd1-external-adoption-c38-current-.md` | **CONFIRMED** | Attribution receipt bytes hash to `56f6d110e1d1bf83cf65f5d0520cb5c6df211fa710119541110d670fe5955ae3` (no expected hash was supplied); its text records the exact missing-source-sidecar contract before 0cd. The adoption receipt hash matches supplied `fb8147aa46baf4668699e6c610e8b6f60c4ddafa87abdd9b3a0d5d27c3016316`; it is after 0cd, so it is adoption evidence, not prior chronology evidence. Whole patch IDs observed: c38 `6a62ff59b1b20a5873006b17ce72cd64229f65a6`; 0cd `57bdcd672364438b3b898f35d6f60c7cc178f5ca`. c38 numstat `20/20` production, `48/0` test; 0cd numstat `20/0` production, `55/0` test. 0cd is based on current-base parent `878f631...`; c38 is not its ancestor, proving semantic adaptation rather than ancestry. Both branches are clean and upstream-equal (`git rev-list --left-right --count` = `0 0`). |
| Han mailbox since Tick 20 | **NO SCORE CLAIM** | Scheduler directives at ticks 20–24 are instructions only. The sole substantive non-scheduler message, `20260826T224724Z-467b6404-tick22-correction-composite-ne.md`, corrects prior arithmetic to production `+4/-20 = -16`, total `+14/-20 = -6`, and explicitly says pending/prose-only adoption scores zero. It does not claim a new score. |

## Mutation, gates, and readiness

- Disposable mutation `feae99e6e965e76bd0d0b0d065eded3e4dc79a3c` is based on loser `a78b39b3d87c82a4f83878359afc98e2b8fde2d4` and adds the exact no-source-sidecar selector. Referee evidence records the selector red on a78 (both sidecar counts remained 1) and green on c38.
- Personally observed focused commands: `CGO_ENABLED=0 go test -race -count=1 -run '^TestConsolidate_PrunesExistingSidecarsWhenSourceHasNoSidecarTables$' ./internal/index` passed on c38; the adapted selector `TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor` passed on 0cd.
- Root immutable serialized command: `CGO_ENABLED=0 go test -p 1 -race -count=1 ./internal/cli ./internal/index`; `/tmp/furiosa-root-t22-0cd-serialized-gate-20260827T065200WITA.log` SHA-256 `33eca6f95eb20fbcfe30f462e9606e6da8f082f8c0e988b8a521cb158fc3714a`, exit 0, CLI 59.888s, index 73.795s, wall 136s, HEAD 0cd.

## Direction Lock boundary

`PA-CONSOLIDATED-SIDECAR-PRUNE-001` is technically confirmed at base `878f631...`, with source winner c38 and current-base adaptation 0cd. It selects the unconditional `topic_segment` / `session_verdict` orphan pruning and co-contributor preservation. This is a technical direction only. It grants **no merge authorization**; invalidate on base or product-patch change, mutation failure, or focused/full gate regression.

## Next challenge

Challenge any later score or merge message that reintroduces Furiosa +2 for this same c38 adoption event, or treats the technical Direction Lock as merge permission.
