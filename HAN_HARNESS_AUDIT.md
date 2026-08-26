# Han harness audit

Audit basis: fixed base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`, initial evidence observed
2026-08-27; correction timestamp 2026-08-27.
This is a read-only audit. `RAWCLAW`, `RECOVERY_HARNESS`, `HAN_INBOX`, and
`FURIOSA_INBOX` are labels for local resources; no local paths, hostnames, tokens, or session
identifiers are reproduced here.

## Verdict

**FAIL: Han does not currently satisfy the launch contract.** The launch text and reference
documents describe the required behavior, and the registered Han supervisor session has since
provided direct live proof that mailbox enforcement is active. The contract still fails because
no ten-minute timer/watchdog activation is observed and one completed candidate has a gate
discrepancy.

## Graphify orientation evidence

The first codebase-orientation action was the Graphify MCP call:

```text
mcp__graphify__query_graph(project_path=RAWCLAW, question="setup hook catalog session", mode="bfs", depth=2, token_budget=2000)
```

Observed outcome: 36 nodes, including `Setup()`, `renderHookScript()`, `hookresolve_test.go`,
catalog-hook tests, and baked-path/fallback hook tests. This framed the later hook inspection.
Follow-up MCP query `question="mailbox lifecycle supervisor"`, BFS depth 3, budget 3000 returned
RawClaw lifecycle nodes but no harness-runtime registration node. MCP `get_node("Setup()")` and
`graph_stats(RAWCLAW)` were also attempted after mailbox directives were handled; the useful
result was the lifecycle query, while the graph did not expose external Codex hook configuration.
That boundary is why the active configuration was then checked directly.

## Five subsystem scores

Scores are out of 5 and measure observed, restartable behavior rather than prose quality.

| Subsystem | Score | Evidence-based assessment |
|---|---:|---|
| Instructions | 4/5 | `LAUNCH_NOW`, charter, rotation, profiles, and worker contract specify roles, phases, evidence, clean/push rules, and exact worker fences. The launch prompt still relies on the operator replacing placeholders and does not itself prove schedule creation. |
| State | 3/5 | `tick.count`, rotation log, scorecard, and changelog exist; completed pushed-clean receipts now exist for three report branches, but no active tick progression was observed. Worker state remains scattered across worktrees/mailboxes. |
| Verification | 3/5 | The shared pre-commit guard is active and require-report now exists on the reported Han candidate worktrees. However, independent supervisor checking found an EOF whitespace defect in candidate `418bfa7`, so one green receipt is disputed. |
| Scope | 4/5 | Charter and worker prompt provide file fences, traps, bounded deliverables, rival read-only rules, and five-to-eight lane guidance. One Luna candidate-stomp worker is live; the worker-only boundary remains behavioral rather than tool-enforced. |
| Lifecycle | 4/5 | Repeated live `MAILBOX BLOCK` denials on PreToolUse, Graphify MCP, collaboration, and spawn in the registered Han supervisor session prove unread-mail enforcement is active. Config discoverability remains incomplete because the inspected hooks file does not visibly reference `lifecycle.py`; no scheduler/ticker process or launchd/cron job was observed. |

Overall: **17/25**. The bottleneck is still cadence/watchdog activation, followed by explicit
configuration discoverability and resolving the `418bfa7` gate discrepancy.

## Active inventory: existence versus activation

| Mechanism | File/config evidence | Activation verdict |
|---|---|---|
| Launch document | `LAUNCH_NOW.md` contains the complete two-supervisor bootstrap and ten-minute run. | PRESENT; no evidence the exact launch prompt was executed in this audit. |
| Worker contract | `WORKER_PROMPT.md` requires Graphify, Mnemon, atomic commits, report, push, and clean finish. | PRESENT; prompt compliance is not runtime-enforced for every worker. |
| Rotation | `TEN_MINUTE_ROTATION.md` defines always-on duties and four phases. | PRESENT; `ROTATION_LOG.md` has only its template line, so no observed tick receipt. |
| Tick script | `supervisor-tick.sh` validates config, locks, sends phase mail, and advances `tick.count`. | PRESENT; no private config was found and no process, cron, or launchd registration was observed. Running it is therefore `UNCERTAIN`, not active. |
| Profiles | Han and Furiosa profiles define distinct personas/checksums. | PRESENT; roster messages show the Han supervisor identity was announced. |
| Mailbox lifecycle | `lifecycle.py` handles `PreToolUse`, `Stop`, and `SubagentStop`, unread blocking, and 600-second patrol. | ACTIVE for the registered Han supervisor session: repeated live `MAILBOX BLOCK` denials were observed on tool and collaboration attempts. The exact dispatch remains undiscoverable in the inspected hooks file. |
| Graphify enforcement | Active `PreToolUse` has a Bash matcher for `graphify hook-check`; launch/worker docs require orientation. | PARTIAL: a Bash hook-check exists, but no evidence it enforces MCP/CLI orientation before every code read or worker launch. |
| Git mailbox guard | Shared `pre-commit` is byte-identical to steering-kit `agent-mailbox-guard.sh` (SHA-256 `487f96c...223105`). It blocks unread mail and checks `.require-report` when present. | ACTIVE for commits using the shared hook, subject to fail-open behavior and per-worktree marker setup. |
| Require-report | Current evidence says `.require-report` exists in both the Han Flash candidate and Luna candidate-stomp worktrees; prior-art and audit branches also produced report receipts. | OBSERVED ACTIVE for the current candidate roster. Earlier missing-marker evidence is stale. |
| Mail send/mark/report | Steering-kit scripts exist; report stamps `.last-report` only after successful send. | PRESENT; mailbox receipts prove messages were sent, but this does not establish lifecycle-hook activation. |
| Supervisor registry | `supervisors.tsv` maps the Han and Furiosa supervisors and a legacy entry. | PRESENT for mapping; registered-session live denials prove that mapping or an equivalent dispatch is being consumed. |
| Independent pull/watchdog | The documents require one for long-running work. | NOT OBSERVED: no watchdog process, ticker, or worker process was found in the live process inspection. |
| Public-path scrubbing | The harness reference documents and example config contain local absolute paths and private-worktree placeholders. | WEAK: the committed audit is scrubbed, but the harness materials themselves are not public-safe by default. |

## Mailbox lifecycle mapping

The intended mapping is sound on paper:

```text
PreToolUse  -> allow mailbox reads; deny other tools while mail is unread
Stop        -> block unread mail; after 600s silence, demand patrol receipt
SubagentStop -> same unread/silence patrol for child completion
```

The observed implementation also writes `.hook-last-check` only after a successful no-unread
check, which prevents a blocked unread event from falsely refreshing the patrol timer. Subsequent
live evidence is decisive: this registered Han supervisor session repeatedly received exact
`MAILBOX BLOCK: N unread message(s)` denials on PreToolUse, Graphify MCP, collaboration send, and
spawn until mail was read and the cursor advanced. Therefore mailbox lifecycle enforcement is
**active in the registered session**. The inspected hooks configuration still does not make the
dispatch discoverable by visibly naming `lifecycle.py`; config absence is a discoverability gap,
not proof of inactive runtime behavior.

## Worker roster and live evidence

Observed Han-associated branches/worktrees from the shared Git worktree registry:

| Label | Branch role | HEAD | require-report | Live process observed |
|---|---|---|---|---|
| HAN_FLASH_CANDIDATE | Flash candidate | base | PRESENT (current evidence) | No |
| HAN_LUNA_CANDIDATE_STOMP | Luna candidate-stomp | base | PRESENT (current evidence) | Yes |
| HAN_OZZY_HARVEST | Luna harvest | base | PRESENT | No |
| HAN_PRIOR_ART | Luna prior art | base | PRESENT | No |
| HAN_HARNESS_AUDIT | Luna harness audit | base | PRESENT | This audit only |

The Han supervisor mailbox now contains completed pushed-clean receipts for `dd545719`, `4e53e9c`,
and `418bfa7`, plus a live Luna candidate-stomp worker. The `418bfa7` receipt is not fully green:
an independent `git diff --check HEAD^ HEAD` found an EOF whitespace defect, now assigned for
correction. Worker completion is therefore **PARTIAL**, not universally green.

## Observed failures and missing enforcement

1. **Config discoverability gap, not lifecycle dead path.** The active hooks file does not visibly
   reference `lifecycle.py`, but repeated registered-session `MAILBOX BLOCK` denials prove live
   unread-mail enforcement. The remaining risk is an undocumented/equivalent dispatch path and
   weaker restart auditability.
2. **Cadence is aspirational.** The tick script is executable and logically advances state, but no
   config, timer, cron entry, launchd registration, running ticker, or tick receipts were observed.
3. **Candidate gate discrepancy.** `418bfa7` has a pushed-clean receipt, but independent
   `git diff --check HEAD^ HEAD` found an EOF whitespace defect. It must be corrected before that
   receipt can count as fully green.
4. **Graphify is guidance plus a narrow hook.** The docs require orientation, while the active hook
   only checks Bash calls. Reads through other tools and launch-time compliance are not proven.
5. **Independent pull is unobserved.** The contract requires a watchdog, but the live inspection
   found no such process or durable pull ledger.
6. **Public hygiene is not built into the harness.** Example/reference documents carry local
   absolute paths. A public report can be scrubbed, but contributors can still copy unsafe paths
   from the launch materials.

## First three surgical changes

1. **Document and verify the lifecycle dispatch path.** Preserve the already-observed live mailbox
   blocking behavior, but make the active registration or equivalent dispatcher discoverable and
   add a fixture test for PreToolUse, Stop, and SubagentStop. Verify with an unread-mail probe and
   recorded `.hook-last-check`; score runtime evidence separately from config presence.
2. **Install one verifiable ten-minute trigger with durable receipts.** Create the private config,
   register one scheduler entry for `supervisor-tick.sh`, and make each tick write a receipt that
   includes tick, phases, and both send outcomes. Add an independent pull/watchdog that notices a
   missing receipt. Until a live tick advances `tick.count`, status remains `UNCERTAIN`.
3. **Close the `418bfa7` gate discrepancy and retain universal report enforcement.** Correct the
   EOF whitespace defect, rerun `git diff --check HEAD^ HEAD` plus the report-history range check,
   and keep `.require-report` provisioned before every candidate launch. A receipt counts only
   after the independent gate agrees.

## Launch-contract checklist

| Contract item | Verdict |
|---|---|
| Supervisor role and persona checksum | PRESENT; Han profile and mailbox announcement observed |
| Worker-only execution boundary | PARTIAL; charter says supervisor is not primary implementer, but no runtime boundary prevents supervisor edits |
| Graphify-first orientation | PARTIAL; documented and first MCP call observed for this audit, not enforced for all workers/tools |
| Mailbox PreToolUse/Stop/SubagentStop | ACTIVE in the registered Han session; config discoverability incomplete |
| Git pre-commit mailbox guard | ACTIVE in shared Git hook; fail-open behavior remains a boundary |
| Require-report every commit | PRESENT on current Han candidate roster; `418bfa7` still fails an independent whitespace gate |
| Ten-minute schedule/ticker | FAIL/UNCERTAIN; script exists, activation and receipts absent |
| Worker roster | PARTIAL; four worktrees named, no live worker processes or completed receipts observed |
| Independent pull/watchdog | FAIL; required in prose, not observed live |
| Public-path scrubbing | PARTIAL; this report scrubbed, source harness examples are not |

**Final verdict: FAIL launch contract.** Mailbox lifecycle enforcement is active for the registered
Han session, and report markers now cover the current candidate roster. The harness still lacks
observed ten-minute scheduler/watchdog activation, has a configuration discoverability gap, and
has not cleared the `418bfa7` gate discrepancy.
