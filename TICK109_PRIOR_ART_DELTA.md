# Tick 109 prior-art delta

## Run and handoff

- `run_timestamp_utc`: `2026-08-27T20:46:28Z`, captured with `date -u`.
- `report_base`: `ae8703bf5e0c4864d1af92ec6fb3e2d5eec8ed88`, the public `main` fetched and fast-forwarded after the live T113 mailbox re-anchor directive.
- `canonical_ledger_sha256`: `1e517b5cc1061c40a2a94f1dda04385a72661518a78c7b55bfe71535f988f513` — matched exactly.
- `predecessor_commit`: `98ae85385a328f58640ec262e7c189c1d821083d` — matched exactly.
- `predecessor_file_sha256`: `dd1f0d4aa2a10bd22123fb6e847b59c887247b73710f02701f3352726847b974` — matched exactly.
- `prior_watermark`: `20260827T032519Z` — matched the canonical ledger and predecessor.
- `reanchor_correction`: the original handoff value `0421e79487f9dab5a504e912a12098dc596ce3c3` was stale by the time of execution. The unread dedicated-mailbox directive dated `2026-08-27T20:41:03Z` named `ae8703bf5e0c4864d1af92ec6fb3e2d5eec8ed88`; `FETCH_HEAD`, `origin/main`, and this worker now all match that immutable SHA. The shared ledger was not edited.
- `scope`: product/report-only. No Go, issue, PR, scorecard, shared-ledger, or supervisor-cursor state was mutated. The required re-anchor evidence response was sent to the named supervisor mailbox before the dedicated cursor was advanced; no further acknowledgement will be sent before the final receipt.

## Predecessor score first

The T57 report and its T61 correction were scored before new searches. The T57 mailbox-cursor recommendation remains unadopted with score `0`:

- `PA-MAILBOX-CURSOR-TARGET-VALIDATE-001`: `unadopted`, score `0`; no independently adopted fix for malformed, nonexistent, future, or empty-mailbox targets was found.
- `PA-HTTP-IF-MATCH-CURSOR-CAS-001`: `rejected`; conceptual CAS comparator only, not local-helper adoption, score `0`.
- `PA-ACTION-CONCURRENCY-OWNER-SERIALIZE-001`: `rejected`; scheduler serialization is duplicate enrichment of existing cursor/lock families, score `0`.
- `PA-SD-NOTIFY-TERMINAL-STATE-001`: `rejected`; process-state notification is not durable application publication, score `0`.

The predecessor's totals remain Furiosa `+9`, Han `+2`, and Ozzy `+3`. No predecessor report-only branch, control message, red defect receipt, or comparator is adoption evidence.

## Current live problem census

- Generated hooks are POSIX `sh` templates in `internal/cli/setup.go`. Current main bakes an absolute executable path, falls back to `command -v`, and degrades to exit `0`; `internal/cli/hookresolve_test.go:80-130` covers off-PATH, spaces, missing-path fallback, and silent no-op. `codexhook_test.go` and `antigravityhook_test.go` cover target-specific JSON and invocation behavior. The remaining prior-art question is a black-box mutation matrix for generated identifier/path bytes under minimal hook environments, not a confirmed current product regression.
- Recurring background sync is bounded only at spawn cadence: `internal/archive/autosync.go:11-56` allows one token per five-minute window, while `internal/cli/autosync.go:63-90` starts a detached `setsid` child, redirects both streams to a rotating log, uses `--timeout 0`, and calls `Process.Release` without waiting. `cmd_archive_autosync_test.go` checks push/up-to-date/no-config text receipts and `autosync_test.go:101-138` checks eventual child-log output. There is still no durable per-run identity, terminal success/failure record, or later lease-reclamation proof that survives parent exit.
- Issue `#45` remains OPEN (`https://github.com/MoonCaves/rawclaw/issues/45`): current `internal/scopes/container.go:21-54` discovers and indexes every CWD before `FilterByPath` in `internal/cli/cli.go`, so the current main does not contain the path-pushdown candidate. Issue `#50` remains OPEN (`https://github.com/MoonCaves/rawclaw/issues/50`): current `internal/cli/cli.go:907-926` still calls Codex, Antigravity, and Goose scope discovery for an exact prefix; the catalog-first candidate is not on current main.

## Regrade since the watermark

The only post-watermark recommendations recorded in the canonical ledger are Tick 87's three refresh-cache recommendations. They were regraded against current public `main` at `ae8703bf5e0c4864d1af92ec6fb3e2d5eec8ed88`:

- `PA-SQLITE-EVICTION-HOLD-LOCK-THROUGH-UNLINK-001`: `externally_adopted`, score `0` in this worker delta. Public PR `#51` commit `b41bb6b2d232f87d0d8ba211524fc788e3fc7e48` is an ancestor of current main. `internal/index/containers.go:69-87` obtains `BEGIN IMMEDIATE` before removing the database and `-wal`/`-shm` siblings; tests at `internal/index/containers_test.go:710-804` retain an active writer and delete after commit. No scorecard mutation is authorized here.
- `PA-SQLITE-EVICTION-FAIL-CLOSED-001`: `externally_adopted`, score `0` in this worker delta, by the same public PR #51 implementation. `evictStaleRefreshDB` returns on failed admission before unlinking, and `containers_test.go:806-817` proves a corrupt file is retained. No separate adoption credit is claimed for the same event.
- `PA-POSIX-RENAME-CACHE-QUARANTINE-001`: `unadopted`, score `0`; it remains a comparator-only alternative and is absent from current main. It is not superseded.

Timing evidence remains report-only and does not alter recommendation status. The exact five-run proof used baseline `029f60d`, #45 candidate `514817ed98850b3e251a6c9da629dafefe893559`, and #50 candidate `4923caafac7e1d1a00c9182587bd096db00ead8f`. #45 baseline/candidate both timed out under the 8-second cap, medians `8.52s`/`8.94s`, so it is inconclusive. #50 produced useful output in `5/5` candidate runs, median `0.76s`, while baseline timed out `5/5`, median `8.81s`; the candidate is not an ancestor of current main. No useful-output latency claim is current-main adoption.

## Exact mechanisms inspected: maximum three

### 1. Bats-core shell command capture for hook mutation matrices

- Source: `https://github.com/bats-core/bats-core/tree/eb7f42f8d608ac693d7a4b67474f6714ea68cfc5` and release `https://github.com/bats-core/bats-core/releases/tag/v1.14.0`.
- Immutable source: Bats-core v1.14.0 release commit `eb7f42f8d608ac693d7a4b67474f6714ea68cfc5`; released `2026-07-21T19:55:47Z`.
- Title: Bats-core v1.14.0; inspected `lib/bats-core/test_functions.bash` `run()` at the pinned commit.
- Inspected mechanism: `run()` executes a command, captures `status`, `output`, and split `lines`, supports expected return status, and can separate stderr. Bats test cases are shell commands whose nonzero status fails the test.
- Applicability: generate each RawClaw hook, invoke it explicitly through `sh` with a minimal `PATH`, and mutate baked identifiers/paths across empty, spaced, moved, missing, sibling, and quoted values. Assert exit status plus stdout/stderr and target-specific JSON. This is a usable harness pattern for the remaining hook matrix.
- Constraint: Bats itself requires Bash as the test runner and is not a RawClaw runtime dependency; it does not mutate source automatically, and its environment must be fenced so the test does not accidentally inherit the developer's PATH, HOME, or catalog markers. Current Go tests already provide much of this behavior, so this is duplicate enrichment rather than a new recommendation or adoption event.

### 2. GNU Parallel joblog, timeout, halt, and resume-failed controls

- Source: `https://git.savannah.gnu.org/cgit/parallel.git/tree/src/parallel.pod?id=4359ad0710c1b7465d7b83bb9b32b49688e93ca9` and pinned text `https://git.savannah.gnu.org/cgit/parallel.git/plain/src/parallel.pod?id=4359ad0710c1b7465d7b83bb9b32b49688e93ca9`.
- Immutable source: GNU Parallel version `20260722`, peeled tag commit `4359ad0710c1b7465d7b83bb9b32b49688e93ca9`; NEWS update `2026-07-22`.
- Title: GNU Parallel manual, 20260722; inspected the `--joblog`, `--timeout`, `--halt`, and `--resume-failed` mechanisms.
- Inspected mechanism: `--joblog` records sequence, host, start epoch, runtime, transfer sizes, exit status, signal, and command; `--halt` stops on configured failure conditions; `--resume-failed` supports retrying failed work. The pinned manual also states that job timeout and failure detection apply per chunk, not necessarily per individual job.
- Applicability: the schema is a useful comparator for a bounded recurring shell-run receipt: immutable run identity, start/runtime, exit/signal, command, and explicit retry/halting state. It can inform evidence collection around RawClaw's detached autosync or ticker child.
- Constraint: the chunk-level timeout and halt semantics are weaker than a per-child terminal receipt; command text can be truncated beyond 4 KB; GNU Parallel is an external Perl/runtime dependency. Its joblog records process execution, not a committed application result after a detached parent exits. It is therefore duplicate enrichment of the existing detached-terminal-receipt and lease families, with no new ID or score.

### 3. Hyperfine repeatable A/B timing and exported measurements

- Source: `https://github.com/sharkdp/hyperfine/tree/975fe108c4ee7bd2600d10758207b44ca3dae738` and release `https://github.com/sharkdp/hyperfine/releases/tag/v1.20.0`.
- Immutable source: Hyperfine v1.20.0 release commit `975fe108c4ee7bd2600d10758207b44ca3dae738`; released `2025-11-18T08:38:43Z`.
- Title: Hyperfine v1.20.0 README and CLI; inspected `--runs`, `--warmup`, `--prepare`, parameter scans, and JSON/Markdown export.
- Inspected mechanism: repeatable command A/B runs with warmup or a preparation command before each sample, plus machine-readable result export. This supplies reproducible timing packets for current-base #45/#50 gates.
- Applicability: wrap each RawClaw invocation with an assertion that the exit status is acceptable and the output contains the required useful result/receipt; record timeout as no useful output; compare candidate and exact-base binaries under the same corpus and cap. The existing timing proof already follows the important semantic part of this pattern.
- Constraint: Hyperfine measures wall time and does not decide whether output is useful, complete, or truthful. It is an external Rust binary and cannot establish adoption when the measured candidate is not current main. This is duplicate enrichment of `PA-SEMANTIC-BENCH-COUNTER-001`, not a new recommendation.

## Deduplication and score ruling

- No new stable recommendation ID or fingerprint is minted. Bats maps to existing generated-hook/path-safety and mutation-test families; GNU Parallel maps to detached terminal receipts, job identity, and lease-reclamation families; Hyperfine maps to the semantic useful-output benchmark family.
- The two PR #51 statuses above are externally adopted by one immutable public-main event, not two score events. The comparator-only quarantine idea and all pending ideas score `0` here. Report branches, timing candidates, worker receipts, process logs, and scheduler/control messages are not adoption.
- `score_eligible_events`: none; score delta `0`; this report does not mutate any scorecard or shared ledger.
- `new_watermark`: `20260827T032519Z`; no newer conforming dedicated-mailbox receipt was processed. The T113 re-anchor directive is control input, not a prior-art receipt.
- `direction_lock`: `PA-CONSOLIDATED-SIDECAR-PRUNE-001` remains technically `LOCKED`; `NO MERGE AUTHORIZATION`.

## Exact proposed append bytes

The exact ledger preimage is the current canonical ledger bytes followed immediately by the bytes between the markers below, including the final newline and excluding the marker lines and Markdown fence. The current ledger already ends in a newline. The expected hash fields are report metadata and are not part of the proposed append bytes.

`proposed_delta_sha256`: `2e2a556593741a50e03941f05ee7731ebedc5753982e6e71bbf8b9dd5abe75f0`

`expected_ledger_sha256`: `76452fb4a8fe1cc1de634bed64744da674eb370a151b180beb1f11092305719b`

```text
## Tick 109 exact-mechanism prior-art delta — 2026-08-27T20:46:28Z
- prior_watermark: 20260827T032519Z
- report_base: ae8703bf5e0c4864d1af92ec6fb3e2d5eec8ed88
- handoff_reanchor: original 0421e79487f9dab5a504e912a12098dc596ce3c3 was superseded by the live T113 directive; public main and this report are anchored at ae8703bf5e0c4864d1af92ec6fb3e2d5eec8ed88
- predecessor_score: PA-MAILBOX-CURSOR-TARGET-VALIDATE-001 unadopted score 0; PA-HTTP-IF-MATCH-CURSOR-CAS-001, PA-ACTION-CONCURRENCY-OWNER-SERIALIZE-001, and PA-SD-NOTIFY-TERMINAL-STATE-001 rejected as comparator-only or duplicate enrichment; predecessor totals Furiosa +9, Han +2, Ozzy +3
- live_problem_census: generated POSIX sh hook identifier/path mutation coverage remains a test gap; detached autosync is spawn-cadence bounded but lacks durable per-run terminal identity and lease reclamation; Issues 45 and 50 remain OPEN and their useful-output timing candidates are not on current main
- external_sources:
  - Bats-core v1.14.0 | commit eb7f42f8d608ac693d7a4b67474f6714ea68cfc5 | released 2026-07-21T19:55:47Z | https://github.com/bats-core/bats-core/tree/eb7f42f8d608ac693d7a4b67474f6714ea68cfc5 | run captures status/output/lines and optional stderr for shell-command assertions | hook mutation-matrix comparator; duplicate existing hook/path test families; Bash runner is not core runtime
  - GNU Parallel 20260722 | commit 4359ad0710c1b7465d7b83bb9b32b49688e93ca9 | NEWS update 2026-07-22 | https://git.savannah.gnu.org/cgit/parallel.git/tree/src/parallel.pod?id=4359ad0710c1b7465d7b83bb9b32b49688e93ca9 | joblog records timing/exit/signal/command and controls halt/timeout/resume-failed | bounded recurring subprocess receipt comparator; chunk-level limits and external runtime do not prove application terminal success; duplicate existing receipt/lease families
  - Hyperfine v1.20.0 | commit 975fe108c4ee7bd2600d10758207b44ca3dae738 | released 2025-11-18T08:38:43Z | https://github.com/sharkdp/hyperfine/tree/975fe108c4ee7bd2600d10758207b44ca3dae738 | repeatable runs/warmups/preparation and JSON/Markdown export | current-base useful-output latency gate comparator; semantic assertion must be supplied by wrapper; duplicate PA-SEMANTIC-BENCH-COUNTER-001
- recommendations:
  - PA-SQLITE-EVICTION-HOLD-LOCK-THROUGH-UNLINK-001 externally_adopted via public PR #51 commit b41bb6b2d232f87d0d8ba211524fc788e3fc7e48; score 0 in this worker delta; no separate score event
  - PA-SQLITE-EVICTION-FAIL-CLOSED-001 externally_adopted via the same public PR #51 commit; score 0 in this worker delta; no separate score event
  - PA-POSIX-RENAME-CACHE-QUARANTINE-001 unadopted comparator-only; score 0; not superseded
- timing_gate_regrade: #45 baseline 029f60d versus candidate 514817ed98850b3e251a6c9da629dafefe893559 timed out 5/5 under 8s, medians 8.52s versus 8.94s, inconclusive; #50 candidate 4923caafac7e1d1a00c9182587bd096db00ead8f produced useful output 5/5 at median 0.76s versus baseline 029f60d median 8.81s with 5/5 timeouts, but candidate is not current-main adoption
- changed_statuses: Tick 87 hold-lock and fail-closed recommendations are externally_adopted on current public main; POSIX rename quarantine remains unadopted; no recommendation is superseded
- adoption_evidence: b41bb6b2d232f87d0d8ba211524fc788e3fc7e48 is current-main PR #51 adoption for both SQLite eviction recommendations; no second credit; timing branches and report receipts are not adoption
- score_eligible_events: none; score delta 0; no scorecard mutation
- duplicates_rejected: Bats as a new runtime dependency; GNU Parallel as durable terminal proof; Hyperfine without semantic output assertion; all report branches, process logs, timing candidates, aliases, and control receipts as adoption
- new_watermark: 20260827T032519Z
- next_leads: current-base mutation matrix for every generated hook identifier/path and minimal PATH; structured detached-child run identity plus terminal commit and later lease reclamation; re-run #45/#50 useful-output gates only on exact current main or an explicitly authorized candidate
- direction_lock: PA-CONSOLIDATED-SIDECAR-PRUNE-001 remains technically LOCKED; NO MERGE AUTHORIZATION
- delta_timestamp_utc: 2026-08-27T20:46:28Z
- correction_timestamp_utc: 2026-08-27T20:46:28Z; correction records the stale original main anchor and the T113 re-anchor
- run_completion_utc: 2026-08-27T20:50:28Z
```

## Verification receipts

- `git status --short` was clean before report creation; this file is the only permitted worktree edit.
- The dedicated mailbox was advanced only through `20260827T204636Z-18211e67-t113-final-only-directive-plus.md`; no supervisor cursor was read or advanced.
- `run_completion_utc`: `2026-08-27T20:50:28Z`, captured with `date -u` after the first hash validation and before the report commit; the proposed append hash is recalculated below after this timestamp was inserted.
