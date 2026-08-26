# Han periodic skill cadence

This is an adoption schedule for the Furiosa and Han adversarial supervisors. It is a rotation, not a command to run every skill on every tick. The four-phase clock is the unit of work:

1. **Claim spy** — inspect both mailboxes and rival claims before assigning credit.
2. **Prior art** — orient in Graphify and memory, then read the exact source or skill body.
3. **Mutation / duplicate** — attack the claim with the smallest useful reproduction, mutation, race, or simplification check.
4. **Harvest / integrate** — verify receipts, commit state, gates, adoption, and handoff.

The two supervisors should keep the method skills below always available in their launch prompts, but rotate the audits at the stated boundaries. A skill is not “covered” because another desk used it: skills apply to each desk’s own files; only file ownership is fenced.

## Clock and receipts

| Boundary | Supervisor action | Exact receipt |
|---|---|---|
| Every ~15-minute tick | Claim spy, mailbox cursor check, status of every lane | A dated mailbox note with CONFIRMED, REBUTTED, UNCERTAIN, or NO SCORE CLAIM; include commit, source path, and next action. A blocked command is not a green result. |
| Start of each phase | Use the phase’s 2–4 skills below; do not run unrelated audits | Phase note naming skill, command, input commit, and output path. |
| Every commit | Re-read status, run the requested focused gate, and record the commit SHA | git status --short --branch, exact gate output, SHA, upstream state, and mailbox receipt. |
| Daily boundary | Rotate the deeper architecture, harness, and performance audits | A dated scorecard listing findings, false positives, accepted deviations, and deferred lanes. |
| Final harvest | Both supervisors reconcile claims and acknowledge completion | Mutual written acknowledgment; Stop/SubagentStop remains UNCERTAIN unless independently observed. |

## Selected rotation (14 skills)

### 1. Graphify — always-on prior-art orientation; periodic saved outcome

Source: /Users/jay-m4/org/skills-upstream/shared/graphify/SKILL.md.

Trigger/frequency: every prior-art phase; revisit after a mutation or integration changes call structure. This is the strongest demonstrated superpower: query first, explain/path the mechanism, then feed the result back with save-result.

Exact method and receipt:

```sh
graphify reflect --if-stale
graphify query "runTagWriteCmd SyncConsolidatedFrom AcquireConsolidatedFence spawnIngestChild" --budget 4000
graphify path "runTagWriteCmd" "SyncConsolidatedFrom"
graphify explain "AcquireConsolidatedFence"
graphify save-result --question "<literal mechanism question>" --answer "<evidence-backed answer>" --outcome useful
```

Receipt must quote node/edge evidence, graph freshness, and whether the result was useful, dead_end, or corrected. The observed graph is 3,501 nodes and 10,364 edges; the current path is one inferred hop runTagWriteCmd() --calls--> SyncConsolidatedFrom(), while the query exposed extracted calls from SyncConsolidatedFrom to fencing-adjacent consolidation phases and spawnIngestChild to openIngestLog.

False-positive trap: the matcher is literal substring search, not semantic search. “No matching nodes” can be a vocabulary mistake or stale graph, and inferred edges are not extracted proof. Never rebuild or update shared graph files from a worker worktree; query the absolute base graph or the read-only MCP project path.

Adoption value: prevents grep-first tunnel vision and makes structural claims reproducible. The query → revisit → saved-outcome loop turns corrections into future orientation instead of repeated dead ends.

### 2. mnemon — always-on continuity and post-commit memory

Source: /Users/jay-m4/.codex/skills/mnemon/SKILL.md.

Trigger/frequency: recall before touching an area; remember after every meaningful commit and after a corrected claim; daily review of candidates only when causality is real.

Exact method and receipt:

```sh
mnemon --store rawclaw recall "supervisor cadence skills graphify ponytail mailbox mutation adoption" --limit 10
mnemon remember "<durable evidence>" --cat insight --imp 3 --entities "rawclaw,<mechanism>" --source agent
```

Receipt is the command output’s action (added, updated, or skipped) plus the returned memory id; review semantic/causal candidates before linking. False-positive trap: high similarity is not causality, and short-lived operational noise or secrets must not be stored. Adoption value: keeps claim rulings, known traps, and adoption evidence available across ticks without pretending chat history is durable.

### 3. Ponytail — always-on method gate

Source: /Users/jay-m4/.claude/plugins/cache/ponytail/ponytail/4.9.0/skills/ponytail/SKILL.md.

Trigger/frequency: every mutation/design/review, not as a separate periodic scan. Apply its ladder: YAGNI, reuse, stdlib, native feature, installed dependency, one line, then minimum code.

Exact receipt: in the phase note, state the first ladder rung that held, files touched, and net line delta; for non-trivial logic leave one runnable check. False-positive trap: a shorter diff in the wrong shared function can preserve the bug; understand all callers first, and do not simplify validation, error handling, security, or explicitly requested behavior. Adoption value: prevents speculative supervisor machinery and makes “simpler” auditable.

### 4. Ponytail-review — periodic mutation/duplicate simplification review

Source: /Users/jay-m4/.claude/plugins/cache/ponytail/ponytail/4.9.0/skills/ponytail-review/SKILL.md.

Trigger/frequency: after a candidate commit and once per daily mutation sweep, scoped to that diff. Do not run on every fifteen-minute tick.

Exact receipt:

```text
<file>:L<line>: <delete|stdlib|native|yagni|shrink> <what to cut>. <replacement>.
net: -N lines possible.
```

False-positive trap: this skill deliberately excludes correctness, security, and performance; route those findings elsewhere. “Lean already. Ship.” is a valid result. Adoption value: catches duplicated scanners, wrappers, and one-implementation abstractions before they become supervisor folklore.

### 5. Ponytail-audit — daily whole-tree bloat scan

Source: /Users/jay-m4/.claude/plugins/cache/ponytail/ponytail/4.9.0/skills/ponytail-audit/SKILL.md.

Trigger/frequency: daily boundary or before a broad integration, never every ten minutes.

Exact receipt: ranked one-line findings in the required tag format, ending net: -N lines, -M deps possible. Commit the report before any later fix if the audit is delegated.

False-positive trap: it is report-only and complexity-only; it must not “fix” a race or security issue. Adoption value: gives both supervisors an independent bloat corpus and prevents one desk’s claimed simplification from becoming accepted without a file-grounded finding.

### 6. Multi-reviewer patterns — harvest consolidation

Source: /Users/jay-m4/org/skills-upstream/shared/multi-reviewer-patterns/SKILL.md.

Trigger/frequency: harvest/integrate when two or more reviewers or claim spies produce overlapping findings; daily scorecard for severity calibration.

Exact receipt: one consolidated report with deduplicated findings, each at file:line, severity, evidence, and recommendation, followed by counts and merge recommendation. Apply its rule that missing tests for critical paths are at least Medium and hot-path performance issues at least Medium.

False-positive trap: similar wording is not duplicate evidence; merge only the same root cause/location. Adoption value: prevents double-counting rival claims and makes Furiosa/Han scoring comparable.

### 7. Parallel debugging — mutation phase for competing causes

Source: /Users/jay-m4/org/skills-upstream/shared/parallel-debugging/SKILL.md.

Trigger/frequency: when a timeout, race, stale state, or data-loss claim has multiple plausible causes; revisit at mutation and harvest, not on healthy trivial lanes.

Exact receipt: hypothesis table with Confirmed, Plausible, Falsified, or Inconclusive; each row cites direct evidence and the reproduction/gate. End with the dominant root cause or explicitly generate the next hypothesis.

False-positive trap: correlation after a commit is not causation, and an inconclusive experiment is not a pass. Adoption value: keeps both adversarial desks from converging on the first attractive explanation, especially around SyncConsolidatedFrom and AcquireConsolidatedFence.

### 8. Code-review-excellence — correctness review at commit harvest

Source: /Users/jay-m4/org/skills-upstream/shared/code-review-excellence/SKILL.md.

Trigger/frequency: every candidate commit before credit; full-diff review at daily integration. Use it for behavior, tests, and maintainability; pair with Ponytail for complexity.

Exact receipt: ordered findings with severity, file:line, concrete failure scenario, and requested fix; explicitly state “no findings” only after inspecting the relevant diff and tests. False-positive trap: style-only comments and speculative future concerns do not outrank a reproducible data-loss or cancellation failure. Adoption value: supplies the correctness half of a fair adversarial score instead of rewarding green unit tests alone.

### 9. Task-coordination-strategies — claim-spy and launch design

Source: /Users/jay-m4/org/skills-upstream/shared/task-coordination-strategies/SKILL.md.

Trigger/frequency: whenever assigning or rebalancing lanes; daily queue review. Use file ownership, dependency edges, acceptance criteria, and explicit out-of-scope boundaries.

Exact receipt: task record containing owned files, blockedBy/blocks, interface contract, acceptance criteria, out of scope, and current status. False-positive trap: coordination metadata cannot substitute for an observed gate or receipt. Adoption value: prevents overlapping writes while allowing both supervisors to apply the same skills independently.

### 10. Team-communication-protocols — mailbox and lifecycle enforcement

Source: /Users/jay-m4/org/skills-upstream/shared/team-communication-protocols/SKILL.md.

Trigger/frequency: every tick for direct messages and mailbox cursors; at handoff/shutdown boundaries for acknowledgments. Broadcast sparingly.

Exact receipt: message filename, sender/recipient mailbox, subject, timestamp, and the acknowledged evidence. The local PreToolUse mailbox enforcement is concrete evidence: it denied repository inspection until the unread Furiosa directive was replied to and marked read. This is a hook mechanism, separate from a periodic skill audit.

False-positive trap: a sent message is not an acknowledgment, and a shutdown request is not proof of stopped work. Stop/SubagentStop status is UNCERTAIN here unless independently observed. Adoption value: makes the claim-spy clock enforceable and prevents unread steering directives from being silently skipped.

### 11. Parallel feature development — worktree and integration boundary

Source: /Users/jay-m4/org/skills-upstream/shared/parallel-feature-development/SKILL.md.

Trigger/frequency: launch and harvest phases; daily integration review when branches diverge. Apply the cardinal rule of explicit file ownership and interface contracts.

Exact receipt: branch/worktree path, base SHA, owned-file list, integration strategy, commit SHAs, and clean/upstream-equal status. Furiosa’s explicit adoption of the direct-collaboration Luna launch mechanism is concrete evidence that this boundary can be used operationally; record it as adoption, not as a claim of product correctness.

False-positive trap: separate worktrees do not prevent overlapping ownership of generated/index files or uncommitted changes. Adoption value: makes races cheap and recoverable while protecting the shared checkout and keeping integration auditable.

### 12. golang-how-to — always-on Go skill router

Source: /Users/jay-m4/org/skills-upstream/shared/golang-how-to/SKILL.md.

Trigger/frequency: every Go review, debug, test, or setup lane. Use it to load the narrow secondary skill instead of dumping the entire Go catalog into every tick.

Exact receipt: name the selected secondary skill and the boundary it covers (for example, concurrency + context for cancellation, testing for a test change, benchmark for measurement). False-positive trap: it is an orchestrator, not evidence that a gate passed. Adoption value: keeps RawClaw’s Go work aligned with CGO_ENABLED=0, race testing, and area-specific guidance without skill sprawl.

### 13. golang-testing — mutation and commit gate

Source: /Users/jay-m4/org/skills-upstream/shared/golang-testing/SKILL.md.

Trigger/frequency: every Go mutation or candidate commit; focused tests during mutation, full race gate at harvest/integration.

Exact receipt: exact command and output, including the test name/package and whether it ran with -race; the standing repository gate is CGO_ENABLED=0 go test -race -count=1 ./.... False-positive trap: a green test that does not exercise the claimed interleaving, cancellation, ordering, or deletion path is not evidence for that claim. Adoption value: turns adversarial hypotheses into reproducible contracts.

### 14. golang-concurrency — mutation audit for goroutine/fence paths

Source: /Users/jay-m4/org/skills-upstream/shared/golang-concurrency/SKILL.md.

Trigger/frequency: whenever touching goroutines, channels, locks, cancellation, ingest children, or consolidated writers; focused mutation audit then daily race review.

Exact receipt: inventory goroutine spawns and exit mechanisms, channel ownership, wait/error propagation, ctx.Done() coverage, bounded spawning, and exact race-test output. Inspect spawnIngestChild, AcquireConsolidatedFence, and any direct tag/vector writer path when relevant.

False-positive trap: presence of a mutex or a green race run does not prove every writer shares the fence; unobserved cancellation and delayed children remain open hypotheses. Adoption value: targets RawClaw’s highest-risk silent data-loss and liveness boundaries.

## Evaluated but lower-frequency candidates

These bodies were evaluated, but are intentionally not in the 14-skill rotating clock:

- /Users/jay-m4/org/skills-upstream/shared/golang-benchmark/SKILL.md — run only for a measured performance question, with go test -bench=. -benchmem -count=10 and (when comparing) benchstat; a ten-minute benchmark ritual creates noise and false regressions.
- /Users/jay-m4/org/skills-upstream/shared/harness-creator/SKILL.md — run at daily or release-boundary harness audits. Its useful receipt is an audit of instruction/state/verification/lifecycle artifacts; it should not be confused with live scheduler evidence. Current Stop/SubagentStop behavior remains UNCERTAIN.
- /Users/jay-m4/org/skills-upstream/shared/git-advanced-workflows/SKILL.md — apply at branch divergence, bisect, recovery, or release preparation, with explicit git worktree list, base SHA, and upstream-equal receipt. Running rebase/bisect guidance every tick adds no evidence.
- /Users/jay-m4/org/skills-upstream/shared/repomix-explorer/SKILL.md — use for broad unfamiliar-repository exploration or remote packs, not known-symbol lookups; its own body says local single-symbol questions should use direct search. Graphify is the stronger RawClaw prior-art entry point.

## Adoption scorecard

At each daily boundary, score each selected skill 0–2:

- **0** — not triggered or no receipt.
- **1** — method named, but evidence or false-positive handling incomplete.
- **2** — exact command/output or artifact receipt, claim disposition, and next action recorded.

Report the total per supervisor, the three weakest skills, and one concrete change for the next clock. A high score never overrides a REBUTTED claim, an UNCERTAIN experiment, a dirty worktree, or missing mutual acknowledgment.

## Evidence boundary

Graphify orientation plus saved-outcome feedback is the largest demonstrated adoption win in this cadence. The mailbox PreToolUse denial demonstrates that enforcement can make a required read/action happen, but it is not evidence that every hook or scheduler is healthy. Furiosa’s direct-collaboration Luna launch adoption demonstrates mechanism uptake, but not product correctness. Stop/SubagentStop remains UNCERTAIN. No product files are changed by this document.
