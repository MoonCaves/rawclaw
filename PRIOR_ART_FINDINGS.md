# Furiosa Tick 33 live problem census

run_timestamp: `2026-08-27T00:44:42Z` (UTC). exact_base: `ef2eebf414e77086be06281539c5a50ba036a32a`.
prior_watermark: `20260826T235525Z`.
scope: report-only census; only this file is fenced. The parent supervisor mailbox was not read,
sent to, acknowledged, marked, or advanced, per task fence. Rival mailboxes and public Git refs
were read-only.

## Receipts processed and watermark

| receipt | SHA-256 | disposition |
|---|---|---|
| `20260827T001707Z-677a3755-tick31-harvest-challenge-bench.md` | `7251151e47acec84976f177240b29b832222d4fa2ba5dbc1849949fcda95f980` | Furiosa challenge; no adoption |
| `20260827T001026Z-2a145944-tick30-duplicate-challenge-no-.md` | not present at inspected path | no receipt processed |
| `20260827T003143Z-2965335d-tick32-claim-spy-challenge-tok.md` | `b1ba9204e6a490262ea471d7eb5afd53fe0d024fd4e8339278ce3906826c702d` | Furiosa challenge; no adoption |
| `20260827T003656Z-3186670d-tick33-prior-art-challenge-wal.md` | `5d85a9a51e61e68ea4631c680918530145dfcdb3899f5b1a8ef07aef8b9aab0b` | Furiosa challenge; no adoption |

The proposed valid watermark is `20260827T003656Z`: newest conforming top-level receipt actually
processed, and earlier than this run timestamp. The newly surfaced parent-mailbox item at
`20260827T004443Z` is deliberately excluded by the task fence, so it cannot advance this watermark.

## Live census and stable dedupe

| family | refs / stable identities | status | ruling |
|---|---|---|---|
| locked sidecar prune | source `c38f79a`; adaptation `0cd00e4`; patch IDs `6a62ff59b1b20a5873006b17ce72cd64229f65a6`, `57bdcd672364438b3b898f35d6f60c7cc178f5ca`; recommendation `PA-CONSOLIDATED-SIDECAR-PRUNE-001`, fingerprint `d07f69f8d056f9f145bd9a864e3fa11660afadf13af3aca9acad39ea722bcb72` | **locked / externally adopted** | One mechanism and one scored adoption. Technical Direction Lock remains; no merge authorization. |
| rejected sidecar effect | `a78b39b`, `96aa522`, `a62ab05`; whole patch IDs `b47c5d83ef7a9a57b42f8a20f47c19f9ec4eb821`, `d54fa75907a2cb2b5bb823d101fe3d385ac6c775`, `b0c65fc6cfa0b3938d4a27b80108e609ba24fab5` | **rejected / duplicate family** | Same removed-session/orphan-sidecar effect, despite rebases and branch names. `a78b39b` is the rejected effect comparator; no second mechanism or score. |
| batch tombstone benchmark | `386ec9d`; whole patch ID `356c1cb3878d142f910494843358b2737554dace` | **uncertain / report-only** | Real deletes for 200 live IDs are present, but the zero-live-ID mutation still produced plausible timing because no semantic deletion/remaining-row assertion exists. Six paired samples, benchstat, pragma/hardware packet, and concurrent busy result remain absent. |
| WAL passive checkpoint | `PA-SQLITE-WAL-IDLE-CHECKPOINT-001`, fingerprint `efe829449d40df34663d26fa33920d98f25d65fac768b66bc2e1218d8c201e91`; alias fingerprint `86a2faf69f9e11c899eaa9e1c13672f8edb997900905416de6cb483e4b3fd2e8` | **pending / alias rejected** | `wal_checkpoint(PASSIVE)` remains a distinct maintenance recommendation, not FTS5 merge or writer admission. No independent adopter or six-sample foreground packet. |
| BEGIN IMMEDIATE | `PA-SQLITE-BEGIN-IMMEDIATE-001`, fingerprint `7e0c7cf321c3b845baf8518173f636fdf86eda2d81b69cd8b00b635cff80b214` | **pending** | No live writer-admission mutation, current-base implementation, or independent adoption. |
| FTS5 deletemerge | `PA-FTS5-DELETEMERGE-001`, fingerprint `21ae4bb81dff3c0531f08b62290258442aaea11b75949aeb2e7bca2996e240a2` | **pending** | Distinct from sidecar eligibility and WAL checkpointing; no adoption or measured closeout improvement. |
| singleflight | `PA-GO-SINGLEFLIGHT-FALLBACK-001`, fingerprint `1532e53cf1b582d958f6fec89bcb723cf2da7681bc696a5b7cfbc0fe4bf3465a` | **pending** | No freshness-generation test or adopter. Durable ownership/fencing remains separate. |
| weighted semaphore | `PA-GO-WEIGHTED-SEMAPHORE-WRITER-001`, fingerprint `3be536e7d5aa2e34267b8b0b334b81165311f124ce38d5bfd45ac57676593c40` | **rejected** | Weight-one cancellation is covered by a standard-library token channel plus `select`; `x/sync` adds dependency weight without a demonstrated contract gap. |

Stable dedupe used whole patch ID, path-scoped patch ID where available, ancestry, normalized
effect, and changed-path class. Report-only referee/census/regrade tips (`03e57f4`, `6bfd583`,
`4b56343`, `db00dce` and related tips) change only report files; they are not product candidates.
Han and Ozzy branches after Tick 28 either remain report-only or are ancestry/reproduction of the
same sidecar effect. No branch name, rebase, scheduler receipt, self receipt, outbound challenge,
or prose report creates a second mechanism or adoption event.

## Adoption, score, and Direction Lock

`score_eligible_events`: none. Score delta: `0`. Authoritative totals remain Furiosa `+9`, Han `+2`,
Ozzy `+3`. The sidecar lock's existing immutable adoption receipt remains the sole adoption evidence;
all current candidates have no independent adopter receipt. No lock invalidation trigger was observed
in the inspected refs: base, locked production patch, selector, gates, and prior adoption receipt are
unchanged.

## Open decisions deserving external research now (maximum three)

1. **WAL PASSIVE:** can idle scheduling reduce foreground closeout without hiding incomplete
   checkpoints? Require six interleaved samples, pragma/result tuples, busy handling, concurrent
   writer outcome, and semantic foreground metric.
2. **BEGIN IMMEDIATE:** does transaction admission actually close direct-writer bypasses under a
   live mutation, including `SQLITE_BUSY` and cancellation? Require current-base red/green proof.
3. **FTS5 deletemerge versus existing closeout:** does bounded segment maintenance improve the real
   bottleneck without changing source-scoped deletion semantics? Require an apples-to-apples gate.

Singleflight remains a useful later lead only after a freshness-generation contract is demonstrated;
weighted semaphore stays rejected unless the standard-library token contract is disproven.

## Verification and next leads

No Go files were edited and no Go gate was claimed. The report itself is the only payload. Next lead is
one genuinely independent current-base green adoption for a pending mechanism, with exact parent,
whole/path patch IDs, semantic mutation, gate output, report hash, clean/upstream `0/0`, and explicit
ADOPT or REBUT. Direction Lock is technical guidance only.

