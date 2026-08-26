# Han harness audit

Audit basis: fixed base `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`, observed 2026-08-27.
This is a read-only audit. `RAWCLAW`, `RECOVERY_HARNESS`, `HAN_INBOX`, and
`FURIOSA_INBOX` are labels for local resources; no local paths, hostnames, tokens, or session
identifiers are reproduced here.

## Verdict

**FAIL: Han does not currently satisfy the launch contract.** The launch text and reference
documents describe the required behavior, but the active enforcement path does not prove it. In
particular, the mailbox lifecycle program is absent from the active hooks registry, no ten-minute
timer was observed, and only some worker worktrees carry require-report enforcement.

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
| State | 2/5 | `tick.count`, rotation log, scorecard, and changelog exist, but the live state files contain only templates/no receipts. No active tick progression was observed. Worker state is scattered across worktrees/mailboxes rather than one verified roster. |
| Verification | 2/5 | Worker contract names gates and independent supervisor verification. The shared pre-commit hook exists and is copied from the steering-kit guard, but there is no observed pre-push public scrub gate and no evidence that every worker has report enforcement. |
| Scope | 3/5 | Charter and worker prompt provide file fences, traps, bounded deliverables, rival read-only rules, and five-to-eight lane guidance. Actual roster evidence showed four Han-associated worktrees, one without require-report, and no observed live worker process. |
| Lifecycle | 1/5 | `lifecycle.py` implements PreToolUse/Stop/SubagentStop logic, silence patrol, cursor handling, and mapped inbox resolution, but active hooks configuration does not invoke it. No scheduler/ticker process or launchd/cron job was observed. |

Overall: **12/25**. The bottleneck is lifecycle activation, followed by durable state and uniform
verification enforcement.

## Active inventory: existence versus activation

| Mechanism | File/config evidence | Activation verdict |
|---|---|---|
| Launch document | `LAUNCH_NOW.md` contains the complete two-supervisor bootstrap and ten-minute run. | PRESENT; no evidence the exact launch prompt was executed in this audit. |
| Worker contract | `WORKER_PROMPT.md` requires Graphify, Mnemon, atomic commits, report, push, and clean finish. | PRESENT; prompt compliance is not runtime-enforced for every worker. |
| Rotation | `TEN_MINUTE_ROTATION.md` defines always-on duties and four phases. | PRESENT; `ROTATION_LOG.md` has only its template line, so no observed tick receipt. |
| Tick script | `supervisor-tick.sh` validates config, locks, sends phase mail, and advances `tick.count`. | PRESENT; no private config was found and no process, cron, or launchd registration was observed. Running it is therefore `UNCERTAIN`, not active. |
| Profiles | Han and Furiosa profiles define distinct personas/checksums. | PRESENT; roster messages show the Han supervisor identity was announced. |
| Mailbox lifecycle | `lifecycle.py` handles `PreToolUse`, `Stop`, and `SubagentStop`, unread blocking, and 600-second patrol. | PRESENT but INACTIVE: active hooks configuration has no command invoking this program. |
| Graphify enforcement | Active `PreToolUse` has a Bash matcher for `graphify hook-check`; launch/worker docs require orientation. | PARTIAL: a Bash hook-check exists, but no evidence it enforces MCP/CLI orientation before every code read or worker launch. |
| Git mailbox guard | Shared `pre-commit` is byte-identical to steering-kit `agent-mailbox-guard.sh` (SHA-256 `487f96c...223105`). It blocks unread mail and checks `.require-report` when present. | ACTIVE for commits using the shared hook, subject to fail-open behavior and per-worktree marker setup. |
| Require-report | Han Ozzy and Han prior-art worktrees contain `.require-report` pointing to `HAN_INBOX`; Han Flash candidate does not. | PARTIAL and uneven. A worker can commit without outbound report when the marker is absent. |
| Mail send/mark/report | Steering-kit scripts exist; report stamps `.last-report` only after successful send. | PRESENT; mailbox receipts prove messages were sent, but this does not establish lifecycle-hook activation. |
| Supervisor registry | `supervisors.tsv` maps the Han and Furiosa supervisors and a legacy entry. | PRESENT for mapping; no active hook consumed it because lifecycle is not registered. |
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
check, which prevents a blocked unread event from falsely refreshing the patrol timer. However,
the active hooks configuration contains no `lifecycle.py` command for any of these events. The
mapping is therefore **documented and implemented, but not activated**. The active configuration
does list `PreToolUse`, `Stop`, and `SubagentStop` sections, yet those sections call other tooling;
section existence is not lifecycle activation.

## Worker roster and live evidence

Observed Han-associated branches/worktrees from the shared Git worktree registry:

| Label | Branch role | HEAD | require-report | Live process observed |
|---|---|---|---|---|
| HAN_FLASH_CANDIDATE | Flash candidate | base | MISSING | No |
| HAN_OZZY_HARVEST | Luna harvest | base | PRESENT | No |
| HAN_PRIOR_ART | Luna prior art | base | PRESENT | No |
| HAN_HARNESS_AUDIT | Luna harness audit | base | PRESENT | This audit only |

The Han supervisor mailbox contains a roster claim of three Luna workers plus one Flash overlap,
but no committed worker output or independent gate receipt was present in the inspected state.
Accordingly, worker completion/adoption credit is **UNCERTAIN**, not green.

## Observed failures and missing enforcement

1. **Lifecycle dead path.** `lifecycle.py` is not referenced by the active hooks configuration, so
   unread mail and 600-second Stop/SubagentStop patrols are not proven to run.
2. **Cadence is aspirational.** The tick script is executable and logically advances state, but no
   config, timer, cron entry, launchd registration, running ticker, or tick receipts were observed.
3. **Require-report is opt-in but not universal.** The Flash candidate worktree lacks the marker;
   the commit guard therefore cannot enforce a report for that lane.
4. **Graphify is guidance plus a narrow hook.** The docs require orientation, while the active hook
   only checks Bash calls. Reads through other tools and launch-time compliance are not proven.
5. **Independent pull is unobserved.** The contract requires a watchdog, but the live inspection
   found no such process or durable pull ledger.
6. **Public hygiene is not built into the harness.** Example/reference documents carry local
   absolute paths. A public report can be scrubbed, but contributors can still copy unsafe paths
   from the launch materials.

## First three surgical changes

1. **Register the lifecycle hook in the active hooks configuration.** Add the exact `lifecycle.py`
   command under active `PreToolUse`, `Stop`, and `SubagentStop` entries, with a small fixture test
   that feeds each event and asserts deny/block behavior. Verify activation by a real unread-mail
   probe and a recorded `.hook-last-check`; do not score from config presence alone.
2. **Install one verifiable ten-minute trigger with durable receipts.** Create the private config,
   register one scheduler entry for `supervisor-tick.sh`, and make each tick write a receipt that
   includes tick, phases, and both send outcomes. Add an independent pull/watchdog that notices a
   missing receipt. Until a live tick advances `tick.count`, status remains `UNCERTAIN`.
3. **Make report enforcement universal at worker creation.** Every Han worker worktree must receive
   `.require-report` before launch, and the launcher must fail closed if the marker or supervisor
   inbox is missing. Verify with a disposable commit that fails before report, succeeds after report,
   and leaves a receipt in `HAN_INBOX`.

## Launch-contract checklist

| Contract item | Verdict |
|---|---|
| Supervisor role and persona checksum | PRESENT; Han profile and mailbox announcement observed |
| Worker-only execution boundary | PARTIAL; charter says supervisor is not primary implementer, but no runtime boundary prevents supervisor edits |
| Graphify-first orientation | PARTIAL; documented and first MCP call observed for this audit, not enforced for all workers/tools |
| Mailbox PreToolUse/Stop/SubagentStop | IMPLEMENTED in source, INACTIVE in active registration |
| Git pre-commit mailbox guard | ACTIVE in shared Git hook; fail-open and marker opt-in weaken coverage |
| Require-report every commit | FAIL; absent from at least one Han worker worktree |
| Ten-minute schedule/ticker | FAIL/UNCERTAIN; script exists, activation and receipts absent |
| Worker roster | PARTIAL; four worktrees named, no live worker processes or completed receipts observed |
| Independent pull/watchdog | FAIL; required in prose, not observed live |
| Public-path scrubbing | PARTIAL; this report scrubbed, source harness examples are not |

**Final verdict: FAIL launch contract.** The harness is a credible specification and a useful
starting point, but it is not yet an active, restartable enforcement system for Han.
