# Furiosa Tick 33 cumulative prior-art regrade

run_timestamp: `2026-08-27T00:41:05Z` (UTC). Exact base: `ef2eebf414e77086be06281539c5a50ba036a32a`.
Prior authoritative watermark: `20260826T235525Z`.

## Evidence boundary and receipts

The prohibited parent mailbox `/Users/jay-m4/code/rawclaw-supervisor-furiosa-a/.agent-mailbox`
was not read, acknowledged, marked, or advanced. I inspected conforming Furiosa-authored
challenge receipts available in rival worktree mailboxes and public Git refs. Hidden,
quarantined, malformed, future-dated, scheduler-only, self, and outbound challenge items are
not adoption evidence.

Receipts processed after the prior watermark:

| UTC receipt | SHA-256 | path | disposition |
|---|---|---|---|
| `20260827T001707Z-677a3755-tick31-harvest-challenge-bench.md` | `7251151e47acec84976f177240b29b832222d4fa2ba5dbc1849949fcda95f980` | `/Users/jay-m4/code/rawclaw-wt-instant-closeout-spec/.agent-mailbox/20260827T001707Z-677a3755-tick31-harvest-challenge-bench.md` | Furiosa challenge; no adoption |
| `20260827T001026Z-2a145944-tick30-duplicate-challenge-no-xsync-for-weight-one.md` | unavailable (path absent in inspected Han mailbox) | rival Han mailbox path; not present | no receipt processed |
| `20260827T003143Z-2965335d-tick32-claim-spy-challenge-tok.md` | `b1ba9204e6a490262ea471d7eb5afd53fe0d024fd4e8339278ce3906826c702d` | `/Users/jay-m4/code/rawclaw-supervisor-han-b/.agent-mailbox/20260827T003143Z-2965335d-tick32-claim-spy-challenge-tok.md` | Furiosa challenge; no adoption |
| `20260827T003656Z-3186670d-tick33-prior-art-challenge-wal.md` | `5d85a9a51e61e68ea4631c680918530145dfcdb3899f5b1a8ef07aef8b9aab0b` | `/Users/jay-m4/code/rawclaw-wt-instant-closeout-spec/.agent-mailbox/20260827T003656Z-3186670d-tick33-prior-art-challenge-wal.md` | Furiosa challenge; no adoption |

The newest valid processed top-level Furiosa receipt is `20260827T003656Z`; therefore the new
watermark is `20260827T003656Z`. It is earlier than run completion and does not claim processing
of the prohibited parent mailbox.

## Cumulative re-grade

| recommendation | fingerprint | status | adoption evidence / score |
|---|---|---|---|
| `PA-SQLITE-BEGIN-IMMEDIATE-001` | `7e0c7cf321c3b845baf8518173f636fdf86eda2d81b69cd8b00b635cff80b214` | `unadopted` / pending | No independent adopter, immutable green receipt, or current-base product implementation. `0`. |
| `PA-FTS5-DELETEMERGE-001` | `21ae4bb81dff3c0531f08b62290258442aaea11b75949aeb2e7bca2996e240a2` | `unadopted` / pending | No independent adopter, immutable green receipt, or measured closeout result. `0`. |
| `PA-GO-SINGLEFLIGHT-FALLBACK-001` | `1532e53cf1b582d958f6fec89bcb723cf2da7681bc696a5b7cfbc0fe4bf3465a` | `unadopted` / pending | No independent adopter, immutable green receipt, or current-base implementation. `0`. |
| `PA-SQLITE-WAL-IDLE-CHECKPOINT-001` | `efe829449d40df34663d26fa33920d98f25d65fac768b66bc2e1218d8c201e91` | `unadopted` / pending | Tick 33 challenge requests a six-sample foreground-latency packet; no response or adoption exists. `0`. |
| `PA-GO-WEIGHTED-SEMAPHORE-WRITER-001` | `3be536e7d5aa2e34267b8b0b334b81165311f124ce38d5bfd45ac57676593c40` | `rejected` | Weight-one cancellable admission is covered by a stdlib token channel and `select`; `x/sync` is absent. Local ruling is not external adoption. `0`. |
| `PA-CONSOLIDATED-SIDECAR-PRUNE-001` | `d07f69f8d056f9f145bd9a864e3fa11660afadf13af3aca9acad39ea722bcb72` | `locked` / externally adopted previously | Existing immutable adoption receipt SHA `fb8147aa46baf4668699e6c610e8b6f60c4ddafa87abdd9b3a0d5d27c3016316`; technical lock remains, with no merge authorization. No new score. |

Post-watermark Git movement (`03e57f4`, `6bfd583`, `4b56343`, and related refs) is report-only
referee/audit work. Product commits that reproduce or adapt sidecar/topic-prune behavior are
duplicate mechanism activity and have no new adopter receipt in this evidence window. Furiosa
self-receipts, scheduler controls, outbound rival challenges, and report-only branches cannot
establish external adoption.

## Prior research and duplicate rulings

Prior source research remains applicable: SQLite Transactions and FTS5 documentation, SQLite WAL
documentation, Go `x/sync` source, Go context cancellation guidance, Kubernetes terminal Job
conditions, Kafka delivery semantics, and Git content-addressed object identity. These are mature
precedents, not RawClaw adoption. `BEGIN IMMEDIATE`, WAL checkpoint scheduling, FTS5 `deletemerge`,
singleflight, and weighted admission remain separate mechanisms; none earns duplicate credit for
the sidecar lock or for another mechanism's receipt.

Duplicates rejected this run:

- `PA-SQLITE-WAL-PASSIVE-CHECKPOINT-001` remains an alias of canonical WAL ID
  `PA-SQLITE-WAL-IDLE-CHECKPOINT-001`, fingerprint `86a2faf69f9e11c899eaa9e1c13672f8edb997900905416de6cb483e4b3fd2e8`.
- The weighted-semaphore report and all token-channel challenges are local research/rulings, not
  a second adoption event.
- Sidecar adaptations/reproductions (`c38f79a`, `0cd00e44`, and later duplicate product tips) do
  not score again under fingerprint + adopter + immutable receipt SHA/path.
- Scheduler receipts, self-receipts, outbound rival challenges, and report-only branches are
  rejected as score evidence.

## Score and next leads

`score_eligible_events`: none. Score delta: `0`. Authoritative totals remain Furiosa `+9`, Han
`+2`, Ozzy `+3`. No Direction Lock invalidation trigger was evidenced: the locked base,
production patch, decisive selector, gates, and prior adoption receipt remain unchanged.

Next leads: obtain an independent current-base green adoption for one pending mechanism; for WAL,
require the challenged six interleaved closeout samples, pragma/result tuple, busy handling,
concurrent-writer outcome, and semantic foreground metric; for BEGIN IMMEDIATE, require a live
writer-admission mutation; for singleflight, require a freshness-generation test; keep the
weighted semaphore rejected unless stdlib cannot satisfy a demonstrated contract.

Verification: no Go files edited; no Go gate claimed. This report is the only file in the fence.
