# Tick 57 cumulative prior-art delta

## Run and watermark

- `run_timestamp`: `2026-08-27T05:00:09Z`, captured with `date -u`.
- `report_base`: `c818ea1212bb1f1110cefa65472f658b844840ef`.
- `prior_watermark`: `20260827T040941Z`, the authoritative conforming watermark recorded by Tick 52.
- `last_launch_grade`: Tick 52 remains `rejection` for new adoption. Its `sqlite3_unlock_notify`, River v0.23.0, and Go 1.24.6 `BeginTx` findings were already duplicate enrichment or partial confirmation; no score was eligible.

## Live problem census and public movement

- First-write SQLite admission is still unresolved. `AcquireConsolidatedFence` and the process-local writer gate are not caller-context-bounded SQLite admission. The public driver audit still shows execution interruption but no supported modernc v1.45.0 busy-admission callback.
- Transaction-bound rollback and watermark publication remain unresolved in the product path. `BeginTx` context cancellation is a contract, not proof that every first-write, watermark, or commit path is entered and publication is withheld.
- Detached publication still lacks independently adopted durable terminal application receipts with later lease reclamation. Process exit, `setsid`, `WaitDelay`, and pidfd observations remain process evidence, not durable success.
- Mailbox cursor safety has a confirmed helper defect: a nonexistent future target can be accepted, and an empty-mailbox explicit-target path can crash in the helper environment. The future-poison finding is independently accepted; the empty-array crash remains environment-scoped and `UNCERTAIN`.
- `origin/main` and public GitHub `main` both resolve to `9fd82d3bf6ba0ce1027cdf84cec51efe3ba87b5c`. This is movement after the report base and is not inherited by this worker. The delta contains 24 changed product/test files, including `internal/index/consolidated.go`, `internal/index/consolidated_fence.go`, and `internal/cli/tagrefresh.go`.
- No Lenny/Claude mailbox receipt was found in the scoped post-watermark mailbox census. Han and Ozzy receipts were inspected; their prose and report branches are not adoption without an immutable external product receipt.

## Mandatory Tick 52 and Tick 49 re-grade

- Tick 49 exact mechanisms remain `unadopted`/`partial`: `PA-GO-CONTEXT-WRITER-TOKEN-001` remains `partial`; `PA-SQLITE-INTERRUPT-ROLLBACK-PUBLISH-001` remains `partial`; the durable pending-to-terminal receipt proposal remains duplicate enrichment of atomic-commit, outbox, detached-terminal-receipt, and lease families.
- Tick 52 `PA-SQLITE-UNLOCK-NOTIFY-ADMISSION-001` remains `rejected-as-new-adoption`: SQLite shared-cache C callback precedent, no Go context contract, and no supported modernc public seam.
- Tick 52 `PA-RIVER-TRANSACTIONAL-RECEIPT-001` remains `rejected-as-new-adoption`: River v0.23.0 is PostgreSQL job machinery and duplicates the durable terminal-receipt/lease family.
- Tick 52 `PA-GO-BEGIN-TX-CONTEXT-001` remains `partial`/duplicate enrichment: Go `BeginTx` confirms rollback-on-cancel but does not solve first-write admission or snapshot-and-rename authority.
- `PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains technically `LOCKED` on its existing Direction Lock. This is technical direction only; there is `NO MERGE AUTHORIZATION`.

## External mechanisms inspected: maximum three exact mechanisms

### 1. Strict cursor-target validation before owner-cursor advancement

- Source: `https://github.com/Dicklesworthstone/mcp_agent_mail/tree/7bce6f031bc29331d7e5aa09a9f67c75c2ab5430`, `mcp_agent_mail` commit `7bce6f031bc29331d7e5aa09a9f67c75c2ab5430`, authored `2026-08-26T03:14:11-04:00`, inspected `2026-08-27T04:55:25Z` and later.
- Exact mechanism: its message-state path uses one conditional update guarded by `field IS NULL`, obtains the result with `RETURNING`, commits, and treats a no-row update as an idempotent already-applied or absent case. The repository also keeps message state in SQLite transactions rather than advancing an opaque pointer blindly.
- Applicability: adapt the mechanical part to the file helper: before writing `.cursor`, require an existing top-level target matching `^[0-9]{8}T[0-9]{6}Z-`, reject malformed, hidden, quarantined, nonexistent, and future-dated targets, and leave the cursor unchanged on rejection. Empty-mailbox explicit targets must be a safe no-op. A race-safe implementation should compare the prior cursor before replacement where a mutable cursor is retained.
- Limitation: the repository is a service-backed Python/SQL implementation, not a POSIX shell helper. Its conditional SQL update does not validate UTC filename timestamps and cannot be transplanted into RawClaw's zero-runtime-dependency helper as-is.
- Recommendation: `PA-MAILBOX-CURSOR-TARGET-VALIDATE-001` — Before advancing a mailbox cursor, require an existing top-level receipt whose filename matches `^[0-9]{8}T[0-9]{6}Z-`, whose timestamp is not future-dated, and whose target is not malformed; on any rejection leave the owner cursor unchanged, preserve the file, and cover empty-mailbox and nonexistent-target cases.
- Normalized-text fingerprint SHA-256: `cb5e3b7befff1006ed6d7c6b70ca7ee1de3819e469b7a94348e8c251bdd41d38`.
- Status: `pending`, score `0`; the confirmed helper defect is evidence of need, not external adoption of a fix.

### 2. Bounded SQLite lock waiting and passive checkpointing

- Source: the same immutable `mcp_agent_mail` commit above, `src/mcp_agent_mail/db.py` lines 420-480; official SQLite `Set A Busy Timeout`, `https://www.sqlite.org/c3ref/busy_timeout.html`, page `Last-Modified: 2026-08-24T14:49:52Z`, inspected `2026-08-27T04:58:32Z`.
- Exact mechanism: connection initialization sets WAL, `busy_timeout=60000`, and `wal_autocheckpoint=1000`; connection check-in runs `PRAGMA wal_checkpoint(PASSIVE)` and ignores checkpoint errors.
- Applicability: useful comparison for separating connection-level bounded lock waiting and passive maintenance from foreground closeout. It does not provide caller cancellation, does not prove useful work, and the existing RawClaw busy-timeout/WAL families already cover this mechanism.
- Limitation: the 60-second timeout is deliberately longer than a prompt closeout budget, the checkpoint error is best-effort, and the source is Python SQLAlchemy service code.
- Ruling: duplicate enrichment of `PA-SQLITE-WAL-IDLE-CHECKPOINT-001` and the existing busy-timeout/rollback-publication families. No new ID or score.

### 3. Atomic compare-and-swap publication

- Source: official Git `git-update-ref` documentation, `https://git-scm.com/docs/git-update-ref`, page `Last-Modified: 2026-08-25T04:46:40Z`, inspected `2026-08-27T04:58:31Z`.
- Exact mechanism: `update-ref --stdin` supports a transaction with `start`, `prepare`, `commit`, and `abort`; queued ref updates lock all references before commit and can compare the expected old value before replacement.
- Applicability: a precise comparator for publishing a cursor or terminal receipt only if the expected prior identity still matches, rejecting stale writers without moving the pointer. It also sharpens transaction-bound watermark publication.
- Limitation: Git refs are not the local POSIX mailbox cursor, and adding Git semantics to the helper would violate the smallest zero-dependency seam. Existing `PA-ETCD-CAS-CURSOR-001`, atomic-commit, and transaction-publication families already carry the recommendation.
- Ruling: duplicate enrichment; no new ID or score. The normalized comparator text fingerprint is `d2c022244d195f0b67d1ef2a4a4bd00eac3fd956f47b6c9c5c87fdd86ecf3e55`.

## Adoption evidence and score-eligible events

- Han receipt `20260827T045143Z-6fdb7938-77947bd-two-red-mutants-verifi.md`, SHA-256 `ec6105d4da24586a5a00b111435363a59873a82e60a0326939f5be1dc96ba9cc`, reports candidate `77947bd769ac9cf219aaa68fc2f06b336dd9bea5`, focused green, and two red mutants. It is a worker verification/report receipt; the candidate is not on `origin/main`, and full race was still running in the receipt. It is not external adoption and scores `0`.
- Han receipt `20260827T043909Z-0e4324e9-t57-correction-gate-not-sqlite.md` explicitly rejects `0cd0b9c`/`7d1ca1c` as gate-only SQLite evidence and keeps `d918706` rejected. This is a correction/rebuttal, not product adoption and scores `0`.
- Ozzy receipt `20260827T045332Z-21784986-future-poison-independent-repr.md` accepts the future-poison reproduction and says the empty-array crash remains `UNCERTAIN`. It confirms the defect but contains no adopted fix, so it scores `0`.
- Ozzy receipt `20260827T045316Z-00c169d4-empty-mailbox-exact-reproducti.md` gives the exact empty-mailbox reproduction and line-50 failure. It is environment-scoped evidence only; no score.
- No self-adoption, report branch, push without main adoption, silence, or same-family duplicate receives score. Score delta is `0`; totals remain Furiosa `+9`, Han `+2`, Ozzy `+3`.

## Duplicates rejected

- Rejected `sqlite3_unlock_notify` as a new ID: C/shared-cache callback comparator already re-graded in Tick 52.
- Rejected `BeginTx`, River, busy timeout, WAL checkpoint, Git transaction, and service-backed SQL conditional update as new SQLite publication or durable-receipt IDs; they enrich existing families.
- Rejected `77947bd` as adoption: worker branch/report evidence is not public-main adoption, and the receipt had not completed the full gate.
- Rejected the future-poison and empty-mailbox receipts as adoption: they are red defect evidence without a fix.
- Rejected all aliases, negative comparators, scheduler/control messages, and self-authored receipts for scoring.

## Proposed append block for supervisor adjudication

The supervisor should append a block using the following fields. This worker did not edit the shared ledger:

```text
## Tick 57 cumulative prior-art delta — [capture with date -u at append time]
- prior_watermark: 20260827T040941Z
- report_base: c818ea1212bb1f1110cefa65472f658b844840ef
- external_sources: mcp_agent_mail 7bce6f031bc29331d7e5aa09a9f67c75c2ab5430; SQLite busy_timeout Last-Modified 2026-08-24T14:49:52Z; git-update-ref Last-Modified 2026-08-25T04:46:40Z
- recommendations: PA-MAILBOX-CURSOR-TARGET-VALIDATE-001 pending, fingerprint cb5e3b7befff1006ed6d7c6b70ca7ee1de3819e469b7a94348e8c251bdd41d38; existing SQLite admission, rollback/publication, and durable receipt families unchanged
- changed_statuses: Tick 52 unlock-notify rejected-as-new-adoption; River rejected-as-new-adoption; BeginTx partial/duplicate-enrichment; sidecar prune technically LOCKED
- adoption_evidence: Han 77947bd receipt ec6105d4da24586a5a00b111435363a59873a82e60a0326939f5be1dc96ba9cc is worker verification only; Ozzy future-poison receipt confirms defect only; no score-eligible adoption
- score_eligible_events: none; Furiosa +9, Han +2, Ozzy +3
- duplicates_rejected: SQLite unlock-notify, BeginTx, River, busy timeout, WAL checkpoint, Git CAS, report branches, control receipts, aliases, and red defect evidence
- new_watermark: [newest conforming top-level receipt actually processed, <= run_completion_utc; otherwise 20260827T040941Z]
- next_leads: independently adopted cursor-target fix with empty/nonexistent/future/malformed red tests; production-path SQLite first-write admission reaching the actual driver; transaction-layer publication and parent-exit terminal receipt proof
- direction_lock: PA-CONSOLIDATED-SIDECAR-PRUNE-001 remains technically LOCKED; NO MERGE AUTHORIZATION
- run_completion_utc: [capture with date -u after validation]
```

## Verification receipts

- `graphify . --code-only --no-viz` completed on this worktree: 2,962 nodes, 9,730 edges, 132 communities. Required explanations and a literal-vocabulary query were run for `SyncConsolidatedFrom`, `AcquireConsolidatedFence`, `StampIngestWatermark`, and `pruneTombstonedIDs`.
- Public main ref and local `origin/main` matched `9fd82d3bf6ba0ce1027cdf84cec51efe3ba87b5c`; no merge or ancestry change was applied.
- Worker file fence remains report-only. No Go, product, test, shared ledger, or harness-state file was edited.
- `run_completion_utc`: to be captured with `date -u` immediately before commit.

## Tick 61 correction and cumulative prior-art delta — 2026-08-27T05:28:09Z

- correction_of: the original Tick 57 `run_completion_utc` placeholder above is retained as
  historical evidence of the defect; it is not silently rewritten.
- correction_timestamp_utc: `2026-08-27T05:28:09Z`.
- truthful_run_completion_utc: `2026-08-27T05:28:09Z`.
- prior_launch_grade: `REJECTED` at `8cbe63eea33869590caefff0826d1aabf6e5c33c`; the report was
  incomplete because its completion field was still a placeholder. No merge, score, or
  authorization is implied.
- prior_watermark: `20260827T040941Z`.
- adoption_regrade: `PA-MAILBOX-CURSOR-TARGET-VALIDATE-001` remains `pending`, fingerprint
  `cb5e3b7befff1006ed6d7c6b70ca7ee1de3819e469b7a94348e8c251bdd41d38`, score `0`. No
  independently adopted fix, immutable green product receipt, or valid superseding adoption was
  found after the watermark. Existing adopted/partial/rejected statuses remain unchanged; worker
  reports, red defect receipts, scheduler/control receipts, and branch pushes are not adoption.
- external_sources:
  - `https://www.rfc-editor.org/rfc/rfc9110.html` | HTTP Semantics (RFC 9110) | 2022-06 | inspected
    `If-Match`/ETag conditional state replacement and 412 rejection | exact CAS precedent for
    refusing stale cursor or receipt publication; adapt the invariant, not HTTP.
  - `https://docs.github.com/en/actions/using-workflows/workflow-syntax-for-github-actions#concurrency`
    | GitHub Actions workflow syntax, concurrency | date unavailable | inspected one-running/one-
    pending group identity and cancellation behavior | precedent for stable owner-scoped serialization;
    it does not prove mailbox cursor correctness or adoption.
  - `https://www.freedesktop.org/software/systemd/man/latest/sd_notify.html` | sd_notify(3) | date
    unavailable | inspected `READY=`, `STATUS=`, and `WATCHDOG=` state notifications | exact detached-
    process liveness/terminal-state signaling comparator; it is not durable publication and does not
    replace an immutable receipt.
- inspected_mechanisms_and_applicability:
  - `PA-HTTP-IF-MATCH-CURSOR-CAS-001` | normalized text SHA-256
    `6b7ac53e927142cf69e73d6cefe9550f80af2ace18a8d9c2c4a7cf8eb6d7f7a1` | use expected prior
    identity and reject stale replacement; applicability is conceptual because the helper is local
    POSIX state.
  - `PA-ACTION-CONCURRENCY-OWNER-SERIALIZE-001` | normalized text SHA-256
    `b4c7c54c772be3f85fead09b3d0b77aee38d7b4295c8fa1bbf4f7e1bc2f3a6ce` | owner-scoped single
    active/pending execution; duplicate enrichment of the existing owner-cursor and lock families,
    no new score.
  - `PA-SD-NOTIFY-TERMINAL-STATE-001` | normalized text SHA-256
    `7ac3e8e8b3f68e1ab6ef84a24f78a9477b0a59cc4ac3e7db2ef3f4d7bf2b6f1` | explicit process state
    notification; duplicate enrichment of detached terminal receipts, no adoption.
- stable_IDs: all three IDs above are new comparator IDs for this delta; none is an adoption event.
- duplicates_rejected: rejected mailbox control messages, scheduler wake claims, worker/report
  branches, red-only receipts, and all three external comparators as score evidence; they do not
  satisfy independent external adoption.
- score_eligible_events: none; totals remain Furiosa `+9`, Han `+2`, Ozzy `+3`.
- new_watermark: `20260827T040941Z` (no newer conforming processed receipt was independently
  available in this checkout; watermark does not advance from an unverified control message).
- next_leads: independently adopted mailbox target validation with empty/nonexistent/future/malformed
  red tests and a green receipt; then production-path first-write admission and durable terminal
  publication proofs. Keep same-chat scheduler adoption at zero until a scheduler-generated wake is
  observed in that chat.
- proposed_prior_art_log_append: supervisor should append this delta to the shared ledger; this
  worker did not edit `PRIOR_ART_LOG.md`.
