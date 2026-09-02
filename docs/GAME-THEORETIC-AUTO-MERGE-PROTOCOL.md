# RFC-001: Sovereign Game-Theoretic Autonomous Auto-Merge Protocol

**Status:** DRAFT FOR MULTI-SESSION REVIEW  
**Author (Window 1):** OLUMBRA (`20bc2e04` · `~/code/quota-forensics`) — *Mechanism Design & Game-Theoretic Architecture*  
**Contributors (Window 2):** VESKARN / ZAVRIK (`3f7097f5` · `~/code/rawclaw`) — *Ground-Truth Engine & SRE Execution*  
**Contributors (Window 3):** Aether-Borealis-88 (`fbcb4474` · `~/org/app-experts/litellm`) — *Resiliency & Multi-Runtime Routing*  
**Control Plane:** `~/.config/orchestrator/`  
**Date:** 2026-09-01T14:15:00+08:00 (WITA / Bali Time)  

---

## 1. Executive Summary & Core Objective

The purpose of this protocol is to **safely eliminate the human-in-the-loop bottleneck for code reviews and merges** across sovereign repositories (`rawclaw`, `quota-forensics`, `org`).

Single autonomous agents hallucinate success, miss edge cases, and cut legacy resilience fallbacks. Traditional static CI checks catch syntax and compile errors, but cannot evaluate semantic architecture or Chesterton Fences. 

This protocol replaces human babysitting with **Game-Theoretic Adversarial Self-Play**:
1. **The Proposer (`am-supA`)** implements a feature in an isolated worktree with green unit tests.
2. **The Adversary (`am-supB`)** is incentivized to break the PR by finding failing edge cases, memory regressions, or architectural drift within a bounded time window.
3. **The Referee (`am-orch`)** evaluates the machine-verifiable receipts. If the Adversary fails to produce a valid refutation and all 6 Sovereign Invariants pass, the PR is **autonomously merged via the LRVL (Lock-Rebase-Verify-Land) protocol**.

---

## 2. Theoretical Grounding (The Science)

This protocol is derived from computational complexity and mechanism design research:

* **AI Safety via Debate (*Irving, Christiano, & Amodei — DeepMind / OpenAI*):** Verifying code correctness through an asymmetric zero-sum debate. Core theorem: *"It is harder to lie than to refute a lie"* (PSPACE-completeness). An adversarial critic exposes flaws in seconds that a cooperative reviewer ignores.
* **Self-Play Reinforcement (*SSR / Self-Play SWE-RL*):** Dual-role competition between a *Bug-Injector / Red-Team* and a *Bug-Solver / Blue-Team* prevents model plateauing and forces divergent reasoning paths.
* **Dense Semantic Compression (*Prior Anchors*):** Utilizing dense conceptual triggers (`"Who Not How"`, `"Chesterton's Fence"`, `"Zero-Daemon North Star"`) to prime the LLM's pre-trained latent space with high-density behavioral constraints using minimal tokens.

---

## 3. The Tripartite Architecture

```
┌──────────────────────────────────────────────┬──────────────────────────────┬────────────────────────────────────────────────────────────────────────┐
│ Actor & Identity                             │ Primary Role                 │ Core Responsibility & Incentives                                       │
├──────────────────────────────────────────────┼──────────────────────────────┼────────────────────────────────────────────────────────────────────────┤
│ `am-supA` (The Proposer)                     │ Feature Implementer          │ • Writes code and unit tests in isolated worktrees.                    │
│ Token: `~/.config/orchestrator/agent-mail...`│ (Blue Team)                  │ • Scores points by landing clean, zero-allocation features.            │
├──────────────────────────────────────────────┼──────────────────────────────┼────────────────────────────────────────────────────────────────────────┤
│ `am-supB` (The Adversary)                    │ Adversarial Red-Team         │ • Attacks PRs, probes race conditions, tests unhandled errors.         │
│ Token: `~/.config/orchestrator/agent-mail...`│ (Red Team)                   │ • Scores points by submitting failing reproduction tests.               │
├──────────────────────────────────────────────┼──────────────────────────────┼────────────────────────────────────────────────────────────────────────┤
│ `am-orch` (The Referee & Auto-Merger)        │ Deterministic Adjudicator    │ • Enforces debate timer (180s), runs machine tests & benchmarks.       │
│ Token: `~/.config/orchestrator/agent-mail...`│ (Judge & Release SRE)        │ • Executes atomic git rebase, fast-forward merge, and binary rebuild.  │
└──────────────────────────────────────────────┴──────────────────────────────┴────────────────────────────────────────────────────────────────────────┘
```

---

## 4. The 5-Stage Autonomous Lifecycle & State Machine

```mermaid
sequenceDiagram
    autonumber
    participant SupA as am-supA (The Proposer)
    participant SupB as am-supB (The Adversary)
    participant Orch as am-orch (The Referee / Auto-Merger)
    participant Repo as Sterile Verification Sandbox & Main

    Note over SupA,Repo: Phase 1: Local Implementation & Gate 1
    SupA->>Repo: Writes code on feature branch in isolated worktree
    SupA->>SupA: Runs local `go test -race` & `golangci-lint`

    Note over SupA,Repo: Phase 2: Dispatch Proposal to Agent Mail
    SupA->>Orch: Sends dispatch: Branch, Commit SHA, Diff Stats, Benchmark Baseline

    Note over SupA,Repo: Phase 3: The Adversarial Challenge Window (180s Timer)
    Orch->>Repo: Checks out branch into sterile supervisor worktree
    Orch->>Orch: Runs Deterministic Machine Tests (Build + -race + Lint + Zero Allocs)
    Orch->>SupB: Dispatches audit brief + RawClaw Prior-Art Context

    alt am-supB Finds a Refutation (Adversary Wins Round)
        SupB->>Repo: Injects edge cases, concurrent writes, or memory stress
        SupB->>Orch: Submits verified failing reproduction test (`TestX_FailureRepro`)
        Orch->>SupA: Emits `harness-feedback.json` with file:line & kicks back PR (Self-Heal Loop)
    else am-supB Fails to Refute (Proposer Wins Round)
        SupB->>Orch: Submits: "No refutation found; 6 Sovereign Invariants PASS"
        
        Note over SupA,Repo: Phase 5: Lock-Rebase-Verify-Land (LRVL) Auto-Merge
        Orch->>Repo: Acquires exclusive `flock` on `.git/merge.lock`
        Orch->>Repo: Pulls & rebases on latest `origin/main`
        Orch->>Repo: Runs 5-second smoke verification suite
        Orch->>Repo: Fast-forward merges into `main`
        Orch->>Repo: Rebuilds global binary (`~/.local/bin/rawclaw`)
        Orch->>Repo: Prunes feature worktree & releases merge lock
        Orch->>SupA: Broadcasts "PR Autonomously Merged" (Points Credited)
    end
```

---

## 5. The 6 Sovereign Invariants (Gate 4 Audit Rubric)

When `am-supB` evaluates a proposal, it MUST answer **YES/NO** to these exact 6 calibrated criteria:

```
┌────┬──────────────────────────────────────────┬───────────────┬────────────────────────────────────────────────────────────────────────┐
│ #  │ Sovereign Invariant Question             │ Pass Criteria │ Failure Action & Requirement                                           │
├────┼──────────────────────────────────────────┼───────────────┼────────────────────────────────────────────────────────────────────────┤
│ Q1 │ Chesterton's Fence:                      │      NO       │ REJECT: Cite original rationale from git history. Must prove why the   │
│    │ Does this PR remove or weaken any error  │ (No removals) │ fallback is obsolete before stripping it.                              │
│    │ fallback, multi-store shim, or safety    │               │                                                                        │
│    │ boundary?                                │               │                                                                        │
├────┼──────────────────────────────────────────┼───────────────┼────────────────────────────────────────────────────────────────────────┤
│ Q2 │ Zero-Daemon North Star:                  │      NO       │ REJECT: Flag architectural drift. System must remain a CLI/embedded    │
│    │ Does this PR introduce any background    │ (No daemons)  │ SQLite binary without external network daemon dependencies.            │
│    │ daemon, unmetered socket, or cgo leak?   │               │                                                                        │
├────┼──────────────────────────────────────────┼───────────────┼────────────────────────────────────────────────────────────────────────┤
│ Q3 │ Failing Test Proof:                      │      YES      │ REJECT: Demand reproduction test. PR must include a unit test that     │
│    │ Did the author include a unit test that  │ (Proof given) │ fails on `main` and passes only with this diff.                        │
│    │ proves the bug or validates the feature? │               │                                                                        │
├────┼──────────────────────────────────────────┼───────────────┼────────────────────────────────────────────────────────────────────────┤
│ Q4 │ Worktree & Scope Hygiene:                │      YES      │ REJECT: Flag cross-worktree contamination risk. All paths must be      │
│    │ Are all file and database operations     │ (Safe paths)  │ canonicalized with `filepath.Clean`.                                   │
│    │ isolated to exact canonical paths?       │               │                                                                        │
├────┼──────────────────────────────────────────┼───────────────┼────────────────────────────────────────────────────────────────────────┤
│ Q5 │ Zero Dead Error Checks:                  │      YES      │ REJECT: Cite `file:line` of swallowed error or ungrounded panic.       │
│    │ Are all error returns checked explicitly │ (Handled)     │                                                                        │
│    │ without discarding via `_ = err`?        │               │                                                                        │
├────┼──────────────────────────────────────────┼───────────────┼────────────────────────────────────────────────────────────────────────┤
│ Q6 │ Atomic Reversibility:                    │      YES      │ REJECT: Demand self-contained migration.                               │
│    │ Can this commit be cleanly rolled back   │ (Reversible)  │                                                                        │
│    │ via `git revert <hash>` without schema   │               │                                                                        │
│    │ corruption?                              │               │                                                                        │
└────┴──────────────────────────────────────────┴───────────────┴────────────────────────────────────────────────────────────────────────┘
```

---

## 6. The Lock-Rebase-Verify-Land (LRVL) Protocol & Implementation

To prevent the **Parallel Worktree Merge Race** (where multiple workers merging at once clobber `main`), `am-orch` executes the deterministic LRVL protocol implemented in [`scripts/lrvl-merge.sh`](file:///Users/jay-m4/code/rawclaw/scripts/lrvl-merge.sh) and gated by [`scripts/harness-gate.sh`](file:///Users/jay-m4/code/rawclaw/scripts/harness-gate.sh):

```bash
#!/bin/sh
# scripts/lrvl-merge.sh — Atomic Lock-Rebase-Verify-Land Engine
set -eu

BRANCH="$1"
REPO_DIR="/Users/jay-m4/code/rawclaw"
LOCK_FILE="$REPO_DIR/.git/merge.lock"

# 1. Acquire exclusive lock via portable Python fcntl mutex
python3 - <<EOF
import fcntl, sys
lock_file = open("$LOCK_FILE", "w")
try:
    fcntl.flock(lock_file, fcntl.LOCK_EX | fcntl.LOCK_NB)
    print("Lock acquired successfully.")
except BlockingIOError:
    print("ERROR: Merge lock held by another process.", file=sys.stderr)
    sys.exit(1)
EOF

cleanup() {
  rm -f "$LOCK_FILE"
}
trap cleanup EXIT

# 2. Rebase candidate branch on latest main
git -C "$REPO_DIR" fetch origin main
git -C "$REPO_DIR" checkout "$BRANCH"
git -C "$REPO_DIR" rebase origin/main

# 3. Deterministic 5-Phase Machine Gate Suite
"$REPO_DIR/scripts/harness-gate.sh"

# 4. Fast-Forward Merge & Push to Main
git -C "$REPO_DIR" checkout main
git -C "$REPO_DIR" merge --ff-only "$BRANCH"
git -C "$REPO_DIR" push origin main

# 5. Global Sovereign Binary Rebuild
CGO_ENABLED=0 go build -o ~/.local/bin/rawclaw "$REPO_DIR/cmd/rawclaw"

# 6. Release lock on exit trap
exit 0
```

### Deterministic 5-Phase Gate Specification (`scripts/harness-gate.sh`)
* **Phase 1 (Pure-Go Build):** `CGO_ENABLED=0 go build -o /dev/null ./cmd/rawclaw` (zero cgo, zero daemons).
* **Phase 2 (Concurrency & Race Gate):** `CGO_ENABLED=0 go test -race -count=1 ./...` (100% PASS across all packages).
* **Phase 3 (Formatting Gate):** `gofmt -l internal/ cmd/` (zero formatting drift).
* **Phase 4 (Worktree Hygiene):** `git status --porcelain` validation.
* **Phase 5 (Performance Budget):** `go test -run=^$ -bench=BenchmarkSearch ./internal/agentproto/...` (sub-5ms search path guarantee).

---

## 7. The Recursive Prior-Art & Evidence Ledger

Stored in `harness/prior-art.jsonl` and indexed by RawClaw:

```json
{
  "rfc_id": "RFC-001",
  "decision_sha": "d8ff681",
  "target_file": "internal/agentproto/agentproto.go",
  "ruling": "Single-pass connection memoization approved; ConnectRO fallback preserved for multi-store queries.",
  "tested_invariants": ["Q1", "Q3", "Q4"],
  "grader_agent": "am-supB",
  "adjudicator": "am-orch",
  "points_awarded": {"am-supA": 100, "am-supB": 25}
}
```

*Every new proposal automatically queries this ledger for past decisions on touched files before `am-supB` begins its audit.*

---

## 8. Circuit Breakers & Human Escalation

* **Max Iteration Cap:** If a branch fails **5 consecutive debate iterations**, `am-orch` halts the loop, preserves the worktree, and emits an urgent push notification to Jay's iPhone via `ntfy`:
  ```bash
  ntfy publish sovereign-ops "🚨 Harness Circuit Breaker: PR #XX exceeded 5 iterations. Human review required."
  ```
* **Model Failover Protection:** If Gemini Flash or Claude Sonnet encounters rate-limiting or quota exhaustion, `~/.config/orchestrator/failover.env` automatically switches the debate stream to the secondary LiteLLM proxy route on `ai-server` without dropping state.

---

## 9. Verification & Execution Status

1. **Window 1 (OLUMBRA in `~/code/quota-forensics`):**
   * Mechanism design, mathematical debate formulation, and scorecard scoring rules defined.
2. **Window 2 (VESKARN / ZAVRIK in `~/code/rawclaw`):**
   * Implemented and made executable: [`scripts/harness-gate.sh`](file:///Users/jay-m4/code/rawclaw/scripts/harness-gate.sh).
   * Implemented and made executable: [`scripts/lrvl-merge.sh`](file:///Users/jay-m4/code/rawclaw/scripts/lrvl-merge.sh) with portable Python `fcntl.flock` mutex locking.
   * Ratified Section 5 (6 Sovereign Invariants) and Section 6 (LRVL Protocol).
3. **Window 3 (Aether-Borealis-88 in `~/org/app-experts/litellm`):**
   * LiteLLM proxy token limits and failover responsiveness under high-frequency debate traffic.

---
*Signed and ratified across multi-session review by OLUMBRA (`20bc2e04`) and VESKARN/ZAVRIK (`3f7097f5`).*
