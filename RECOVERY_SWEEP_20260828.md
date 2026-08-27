# Recovery sweep — 2026-08-28

Status: HOLD / UNCERTAIN. This report is intentionally bounded and report-only.

## Snapshot

- CONFIRMED UTC: 2026-08-27T16:17:47Z.
- CONFIRMED WITA: 2026-08-28T00:17:47+0800.
- CONFIRMED public-main orientation SHA: `029f60d77e7e03192bc966de3a835a4a32a00fe2` (assignment base). Live `/Users/jay-m4/code/rawclaw` was on `integrate/tagwrite-closeout-wave1` at `0d1da19`, behind its upstream by 1, and dirty; therefore its checkout is not a public-main witness.
- CONFIRMED Graphify MCP: 3,712 nodes, 10,487 edges, 280 communities; 4 open main-targeting PRs.
- HOLD: mailbox handshake nonce was not received. Supervisor cursor must not be advanced by this worker.

## Active writers

| State | Evidence |
|---|---|
| ACTIVE_WRITER — CONFIRMED | `furiosa-t85-issue26-index-audit-luna`, Codex PID 89842, tmux pane live. Do not resume. Queue thread ID is not present in the captured pane listing; Furiosa must resolve it from her registry. |
| ACTIVE_WRITER — CONFIRMED | `furiosa-t85-ozzy89c8-currentbase-luna`, Codex PID 89822, tmux pane live. Do not resume. Resolve thread ID from Furiosa registry. |
| ACTIVE_WRITER — CONFIRMED | `furiosa-t85-pr40-hostile-smoke-luna`, Codex PID 89832, tmux pane live. Do not resume. Resolve thread ID from Furiosa registry. |
| ACTIVE_WRITER — CONFIRMED | `furiosa-t85-pr43-toctou-luna`, Codex PID 89819, tmux pane live. Do not resume. Resolve thread ID from Furiosa registry. |
| ACTIVE_WRITER — CONFIRMED | Rival `han-t87-prior-art`, Codex PID 64007, tmux pane live. Do not resume or alter. |

## Ranked recovery candidates

The following are candidates only; no resume or queue action was taken.

| Rank | Classification | Candidate / evidence | Safe command |
|---:|---|---|---|
| 1 | COMPLETE_PRESERVE — CONFIRMED | `codex/pr40-reconcile-20260827` at `8a87724`, tracking origin; public PR #40 worktree exists. | None; preserve branch and receipt evidence. |
| 2 | COMPLETE_PRESERVE — CONFIRMED | `codex/rawclaw-closeout-hook-copy` at `03451f3`, tracking origin; durable commit exists. | None; preserve branch. |
| 3 | COMPLETE_PRESERVE — CONFIRMED | `codex/remove-cross-session-tagging` at `be797bf`, tracking origin; durable commit exists. | None; preserve branch. |
| 4 | COMPLETE_PRESERVE — CONFIRMED | `codex/tag-closeout-instant-spec-20260826` at `ead38b9`; durable branch exists, current PR relevance not independently established. | None; preserve; verify against public main before reuse. |
| 5 | UNCERTAIN — HOLD | `/Users/jay-m4/code/rawclaw-codex-pr40-reconcile-20260827` exists, but no session-to-worktree mapping was safely read before mailbox guard blocked commands. | Do not resume until liveness and session mapping are proven. |
| 6 | UNCERTAIN — HOLD | `/Users/jay-m4/code/rawclaw-codex-rescue-12h-20260827` exists; session freshness, dirty state, and writer ownership remain unverified. | Do not resume until liveness and session mapping are proven. |
| 7 | STALE_OR_DUPLICATE — HOLD | Numerous `conor/claim-spy-*` branches point at `0d1da19` or older and are not current public-main recovery targets. | Do not resume; compare only if a receipt names unique payload. |
| 8 | UNCERTAIN — HOLD | Other `rawclaw-*` worktrees from the 48-hour census exist, but the mailbox guard prevented the required per-worktree status and session correlation. | Do not resume until per-worktree evidence is collected. |

## Collision protection and recommendation

- CONFIRMED collision set: four live Furiosa t85 panes plus rival Han t87 pane. Any candidate whose worktree or thread overlaps these must remain protected as ACTIVE_WRITER.
- FIRST WAVE recommendation: HOLD all resumptions. Furiosa should first harvest the five live panes, resolve thread IDs, and clear the supervisor-owned mailbox guard. Only then inspect candidates 5, 6, and 8.
- Exact queue form once a thread ID is independently resolved: `codex queue --thread <id> --message 'Do not resume or duplicate this active writer; recovery sweep found live pane <pane>.'`
- Exact resume form only after proving inactivity: `cd <original-worktree> && codex resume <session-id>`.

## Blocker

UNCERTAIN: the local mailbox pre-tool guard rejects every further shell command because `/Users/jay-m4/code/rawclaw-supervisor-furiosa-a/.agent-mailbox` contains my introduction and another unread message. This worker is forbidden to advance Furiosa’s cursor. Consequently no claim is made about unverified dirty/upstream state, rollout freshness, completion receipts, or exact session/worktree mappings.
