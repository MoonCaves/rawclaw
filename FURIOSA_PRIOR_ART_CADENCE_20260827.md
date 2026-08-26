# Furiosa prior-art cadence — 2026-08-26T21:17:28Z

## Run receipt

- `prior_watermark`: `20260826T202709Z`, taken from the last valid processed top-level receipt in `PRIOR_ART_LOG.md`.
- `run_completion_utc`: `2026-08-26T21:17:28Z` (captured with `date -u`).
- `new_watermark`: `20260826T211556Z`; this is the newest conforming receipt actually inspected in the Furiosa inbox. It is control/ack text, not product adoption evidence.
- Scope: research only. No RawClaw code or shared ledger was edited. The cumulative ledger is proposed to receive this delta append-only after supervisor review.

## Previous-launch grade and adoption regrade

The previous launch’s three recommendations remain `pending` and score zero. No immutable adopter receipt after `20260826T202709Z` proves that any recommendation was adopted. The scheduler/control receipts at `203440Z`, `204440Z`, `205440Z`, `210440Z`, and `211440Z` contain instructions or acknowledgements, not adoption.

- `PA-PG-MERGE-SCOPED-001`: `pending`. Furiosa `8c8216e` (commit time `2026-08-27T04:15:58+08:00`) independently implements source-scoped topic pruning, but predates the PostgreSQL recommendation’s recorded launch and has no causal adopter receipt. It is corroborating product evidence, not score-eligible external adoption.
- `PA-S3-TERMINAL-RECEIPT-001`: `pending`. `987c6a3` (commit time `2026-08-27T05:00:32+08:00`) documents the detached publisher’s receipt gap and confirms the foreground `Start()` message is not a terminal result. It is a report-only red, not adoption of S3 semantics.
- `PA-TEMPORAL-DURABLE-PUBLISH-001`: `pending`. No immutable green receipt, durable execution record, or independently observed terminal-result implementation was found.
- Existing `PA-K8S-SCOPED-RECONCILIATION-001`, `PA-ETCD-CAS-CURSOR-001`, and `PA-DEBEZIUM-OUTBOX-001`: unchanged `pending`; no new adopter or green gate.
- Default generated `tag-write <sid>` remains the unresolved `scope=nil` path. The current red `2789d6f` is a mutation/reproduction, not a fix. Explicit `{Project,TDir}` evidence does not generalize to the default command.

No score change: Furiosa `+9`, Han `+2`, Ozzy `0`. Duplicate rejection: do not score `8c8216e`, `987c6a3`, scheduler acknowledgements, or any comparator as external adoption.

## New high-authority mechanisms

### `PA-AWS-STEPFUNCTIONS-TERMINAL-001`

Normalized recommendation: `Use AWS Step Functions execution records: persist executionArn and expose RUNNING, SUCCEEDED, FAILED, TIMED_OUT, or ABORTED plus stopDate; detached publication is successful only on SUCCEEDED.`

- Fingerprint SHA-256: `5c286f472a6259921c99cea137a63ad5c17267c72ce337bdc4b61d1aec14b801`.
- Source: https://docs.aws.amazon.com/step-functions/latest/apireference/API_DescribeExecution.html — *DescribeExecution*, publication date not stated, accessed 2026-08-26.
- Exact inspected text: “The current status of the execution. … Valid Values: `RUNNING | SUCCEEDED | FAILED | TIMED_OUT | ABORTED | PENDING_REDRIVE`”; the same response includes `executionArn` and `stopDate` (“If the execution ended…”).
- Mapping: RawClaw needs a durable publication identity and terminal state separate from the foreground process. `SUCCEEDED` must mean the child completed the scoped fold and receipt was persisted; process disappearance after `Start()` is not success. This directly addresses the detached-publisher receipt gap, without importing Step Functions into the sovereign binary.
- Status/strength: `pending`, strong (4/5) external mechanism; no RawClaw adoption claimed.

### `PA-AZURE-DURABLE-INSTANCE-TERMINAL-001`

Normalized recommendation: `Use Azure Durable Functions instance records: persist instanceId and expose Pending, Running, Completed, Failed, or Terminated with last checkpoint and output; detached publication is successful only on Completed.`

- Fingerprint SHA-256: `8d417e9288ee88cbda35f8163a0409a629f7d3288ab4314d49620df1bc728203`.
- Source: https://learn.microsoft.com/en-us/azure/azure-functions/durable/durable-functions-instance-management — *Durable Functions instance management*, accessed 2026-08-26.
- Exact inspected text: “Query instances … returns the status of an orchestration instance”; statuses include “`Pending`”, “`Running`”, “`Completed`”, “`Failed`”, and “`Terminated`”. The page also states: “An orchestrator isn't marked as Completed until all of its scheduled tasks finish and the orchestrator returns.”
- Mapping: A RawClaw detached publisher needs an instance/attempt identity, persisted checkpoint or result, and a terminal receipt whose success waits for all scheduled work. This is a precise rebuttal to the current `cmd.Start()`-then-“publication queued” ambiguity. It does not solve source-scoped deletion; pair only with `PA-PG-MERGE-SCOPED-001`.
- Status/strength: `pending`, strong (4/5) external mechanism; no RawClaw adoption claimed.

### `PA-FOUNDATIONDB-CONFLICT-RANGES-001`

Normalized recommendation: `Use FoundationDB strict-serializable conflict ranges: bind the authoritative source scope to a transaction read/write conflict range, retry conflicts, and treat commit_unknown_result as requiring idempotence; never claim publication success without a durable commit receipt.`

- Fingerprint SHA-256: `7bfb9801877c76649995a03b510f9976c59f0137c714710d51332840a314c400`.
- Source: https://apple.github.io/foundationdb/developer-guide.html#conflict-ranges — *FoundationDB Developer Guide: Conflict ranges*, publication date not stated, accessed 2026-08-26.
- Exact inspected text: “FoundationDB transactions guarantee strict serializability”; conflicting transactions “will fail at commit time and will usually be retried by the client.” The guide further warns that `commit_unknown_result` means the client cannot determine whether the transaction succeeded and must consider transaction idempotence.
- Mapping: This is relevant to the unresolved `scope=nil` authority boundary: the read set that establishes the authoritative source scope and the writes that replace derived rows must conflict atomically, or concurrent publishers can publish against a stale scope. The unknown-result warning directly reinforces idempotent attempt keys and a terminal receipt. It is not proof that RawClaw should adopt FoundationDB or that a conflict range alone performs deletion.
- Status/strength: `pending`, medium-strong (3/5); exact scope binding is an adaptation requiring a RawClaw SQLite design decision.

## Graphify receipts

- `graphify reflect --if-stale`: lessons already up to date; read `graphify-out/reflections/LESSONS.md`.
- `graphify update .`: rebuilt the current graph (`3467` nodes, `10362` edges, `174` communities).
- `graphify query "scope publisher receipt" --budget 1200 --context call`: surfaced `Scope` and the detached-publication finding.
- `graphify explain "runTagWriteCmd"`: showed calls to `AcquireConsolidatedFence`, `SyncConsolidatedFrom`, and the `Scope` reference.
- `graphify path "runTagWriteCmd" "SyncConsolidatedFrom"`: one-hop inferred call path.
- Revisit conclusion: the graph supports treating `scope` and `SyncConsolidatedFrom` as separate edges from the terminal-receipt problem; it does not prove that an external terminal-state mechanism fixes source-scoped deletion.

## Proposed append-only ledger delta

```text
run_timestamp: 2026-08-26T21:17:28Z
prior_watermark: 20260826T202709Z
external_sources: AWS Step Functions DescribeExecution; Azure Durable Functions instance management; FoundationDB conflict ranges (URLs and quotes above)
recommendations: PA-AWS-STEPFUNCTIONS-TERMINAL-001=5c286f... pending; PA-AZURE-DURABLE-INSTANCE-TERMINAL-001=8d417e... pending; PA-FOUNDATIONDB-CONFLICT-RANGES-001=7bfb98... pending
adoption_evidence: none after watermark; 8c8216e is independent source-scoped product evidence; 987c6a3 is detached-receipt red evidence
score_eligible_events: none; totals Furiosa +9, Han +2, Ozzy 0
duplicates_rejected: control receipts, 8c8216e, 987c6a3, and prior comparator claims are not external adoption
new_watermark: 20260826T211556Z
next_leads: obtain a green default scope=nil mutation packet; design an idempotent durable receipt keyed by publication attempt; preserve source-scoped deletion and terminal status as separate invariants
```

Direction Lock remains `NO LOCK`: no exact-base red/green packet plus independent immutable adopter receipt exists.
