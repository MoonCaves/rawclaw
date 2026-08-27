# Tick 36 cross-score and claim-identity referee

Run completion: `2026-08-27T01:11:02Z` (captured with `date -u`; WITA `09:11:02`).
Audit worker: `worker/furiosa-t36-score-referee-20260827`, base `ef2eebf414e77086be06281539c5a50ba036a32a`.

## Evidence boundary

Mnemon recall and Graphify CLI orientation preceded inspection. Graphify used the canonical graph at `/Users/jay-m4/code/rawclaw/graphify-out/graph.json`: `reflect --if-stale`, `query "score referee claim receipt patch ancestry chronology" --budget 4000`, `explain`, and `path`. The Tick 35 cumulative ledger was read at `two-supervisor-harness/state/PRIOR_ART_LOG.md`; its authoritative watermark is `20260827T010052Z`, with post-Tick-32 mutation receipts and score outcomes recorded there. The shared scorecard remains Furiosa `+9`, Han `+2`, Ozzy `+3`.

The Furiosa parent mailbox and cursor were not read, sent, acknowledged, or advanced. Rival inspection was read-only: Han supervisor `supervisor/han-mechanism-20260827` is `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`, clean except pre-existing receipt artifacts, upstream `0/0`; the Tick 36 Han/Ozzy claim-spy worktrees both point at inherited `ef2eebf414e77086be06281539c5a50ba036a32a` and contain no committed Tick 36 product payload. Their shared reports repeat the same three Ozzy candidate tips and the same red default-scope finding.

## Exact score verdicts

| Claim/fingerprint or immutable payload | Adopter / evidence | Identity and chronology ruling | Score |
|---|---|---|---:|
| `PA-CONSOLIDATED-SIDECAR-PRUNE-001`, `d07f69f8...` | Ozzy external adoption receipt `fb8147aa46baf4668699e6c610e8b6f60c4ddafa87abdd9b3a0d5d27c3016316`; source `c38f79acf9c9ae43ebd091a95f36837f43c0e423`; adaptation `0cd00e44c7eb87e30fcf72f8ae790e7060635b09` | One already-counted adoption event (Ozzy +3). Furiosa `0cd` transplant is the same effect and was correctly rejected as duplicate; no post-Tick-32 score. | 0 new |
| `PA-SQLITE-BEGIN-IMMEDIATE-001`, `7e0c7cf3...` | Furiosa mutation `47b1c2b9e1ed1fce5df46fc7f6ec66d64d960831` | `NARROWED`, not adopted: `_txlock=immediate` moved contention to Begin but 10s busy timeout still ignored a 200ms context (~10.207s). Parent-mailbox interference gives that lane no process credit. | 0 |
| `PA-SQLITE-PROGRESS-BUDGET-001`, `6d296d6f...` | Furiosa applicability report `bd65393044b51f98cab6faba2bf92235a3d30337` | `NARROWED`, report-only: modernc v1.45.0 has context interruption but no supported progress-handler API. No product adoption or score. | 0 |
| `PA-SQLITE-BUSY-TIMEOUT-001`, `69634bee...` | Furiosa applicability report `bd653930...` | `DUPLICATE`: RawClaw already sets RO 5000ms and RW 10000ms `_pragma=busy_timeout`. No new event. | 0 |
| `PA-SEMANTIC-BENCH-COUNTER-001`, `c0bb5901...` | Furiosa mutation report `bd653930...` | `CONFIRMED` validity rule but unadopted: `ReportMetric` alone prints zero and exits green; assertion is required. Report-only, no score. | 0 |
| Ozzy benchmark `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` | mutation receipt `5878c48064a797314986a884e10163b086a84c5c` | `UNCERTAIN`: no semantic work assertion, fair paired baseline, or external adoption. | 0 |
| Han/Ozzy Tick 36 claim-spy payloads | worktrees at inherited `ef2eebf`; reports only | No new branch movement, immutable adoption receipt, or distinct recommendation fingerprint. Unsupported prose/control mail cannot score. | 0 |

Authoritative totals therefore remain Furiosa `+9`, Han `+2`, Ozzy `+3`; no Direction Lock changes and no merge authorization.

## Duplicate, ancestry, and chronology checks

The repeated Ozzy tips are commits `497818fa1e02a6b799bd862f2585fe1a9618cc42`, `cb339acf8db4043775cc512b9926e76b5526aa16`, and `857dc62414426b540b57a497609122721982a367`. They are descendants of earlier candidate histories (`47c582e8`, `740101c3`, and `cd6efe39` respectively), not new post-Tick-32 mechanism families. Their report claims patch IDs `2d84aa38...`, `7411fcf4...`, and `d2181e52...` identify prior candidate effects; a different commit SHA does not make the effect novel. No independent adopter/receipt pair was found.

The Tick 35 mutation receipts are all report-only and clean/upstream `0/0`: semantic `c0c8bcf...` (report SHA `5b4e4dbf...`), BEGIN IMMEDIATE `47b1c2b...` (report SHA `c0450124...`), and prior-art applicability `bd65393...` (report SHA `92f5d180...`). They cannot score merely because their SHAs differ. Scheduler messages, self-adoption proposals, pending prior-art entries, inherited ancestry, duplicate patches, and unsupported prose all score zero.

## Furiosa Tick 35 cursor attribution

No immutable evidence permits assigning Tick 35 cursor advancement to a named owned worker. The cumulative ledger says the supervisor-owned Tick 35 preliminary receipt `20260827T010052Z-4c067884...` was processed after repair of an unauthorized cursor advance, but it does not identify the actor. The claim-spy branches have no cursor commit, and the parent mailbox/cursor is forbidden to this referee. Verdict: `UNATTRIBUTED / DO NOT GUESS`; do not award or remove process credit from a worker on this record alone.

## Next challenge

Require a current-base, independently adopted payload with one stable recommendation fingerprint, one immutable receipt SHA/path, exact patch-id and ancestry proof, and personally observed gates. For BEGIN IMMEDIATE, challenge the remaining 200ms-context versus 10s-busy-timeout gap; for benchmark claims, require a failing zero-work mutation plus a restored semantic assertion. Do not treat the technical sidecar lock as merge authorization.

Validation: only this report is intended to be committed; no product, rival, scorecard, rotation log, mailbox, cursor, or graph state was modified.
