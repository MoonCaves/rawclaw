# Hostile review: tag overlay and cancellable consolidation

Reviewed pins: `e6f22f1` (overlay), `047a6de` (cancellation), `cabab43`, and
integration `5eec12b`. Existing evidence was re-read from the pinned commits;
the exact focused tests were not treated as proof beyond the behavior they
actually control.

## Findings (ranked)

### 1. High — overlay resurrects deleted authoritative segments

`e6f22f1:internal/cli/tagrefresh.go:115-130` always retains consolidated-only
rows. That is safe for historical rows from a missing source, but it is wrong
for a live source session after `tag-write --retag-all` replaces `[A,B]` with
`[A]`: the overlay returns `[A(new),B(stale)]`. `runTagPrepWithTopics` then
sees B as already tagged and can suppress the untagged window while printing a
topic the authoritative DB deleted.

Exact reproducer: derived rows `(sid,A),(sid,B)`; authoritative rows `(sid,A)`;
call `overlayAuthoritativeTopics(derived, authoritative)`. Observed/pinned
implementation returns two rows. The merge needs a session-level replacement
signal (or authoritative replacement for all rows of a session), while still
preserving derived-only sessions. Deduction: **-4**.

### 2. High — cancellation stops at phase boundaries, not at SQL

`047a6de:internal/index/consolidated.go:699,820-907` uses `con.Exec` and
`tx.Exec` for ATTACH, migration, every merge, prune, and watermark commit.
Only `BeginTx` and one watermark lookup receive `ctx`. Cancellation during a
long merge therefore waits for that SQLite statement/transaction to return.
The pinned mutation audit (`cc3e088`) confirms the limitation: replacing
`BeginTx(ctx,nil)` or `QueryRowContext(ctx,...)` survived because its test only
cancels while waiting for the fence.

Exact reproducer: hold a phase-controlled merge on a large source, cancel the
context, and assert return before the SQL gate is released. The current code
cannot satisfy that assertion. Deduction: **-3**.

### 3. Medium — detached publication reports “queued” without durable work

`5eec12b:internal/cli/tagpublish.go:43-65` starts a detached process and
returns success after `Process.Release`. The only durable artifact is an
append-only log; there is no queue/job record or retry ownership. A child can
exit immediately from an invalid executable, timeout, or a later fold error
after `runTagWriteCmd` has printed `publication queued`; no caller observes or
retries that failure. The integration test controls `spawnTagPublish` and
therefore proves the seam, not real child survival or retry semantics.

Exact reproducer: make the detached child fail after `Start` (or point the
binary at an unavailable source), then inspect the authoritative DB and
consolidated DB. The command still reports queued and returns success. Deduction:
**-2**.

### 4. Medium — authoritative overlay adds a second foreground DB read

`e6f22f1:internal/cli/tagrefresh.go:49-54,100-107` opens the authoritative DB
in addition to the already-open `dbp` connection, then reads the consolidated
DB synchronously. On a large/contended SQLite store this adds another open and
read before the tag dump. It is bounded and fail-soft in the standalone
`readTopicSegments` helper, so this is a latency cost rather than data loss.
Deduction: **-1**.

## Explicit verdict

Reject `cabab43` standalone: its start-UUID-only map is cross-session unsafe;
the composite key in `e6f22f1` is required. Retain the overlay concept only
after fixing session deletion semantics. Retain `047a6de` as partial fence
cancellation progress, but do not call it end-to-end SQL cancellation.

## Gates and limitations

The pinned mutation audit observed the fence-cancellation test passing and the
BeginTx/QueryRowContext mutations surviving. No green result was inferred for
the unpinned SQL-phase reproducer. This review changed no Go files.
