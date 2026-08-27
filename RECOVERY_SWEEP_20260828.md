# Recovery sweep — 2026-08-28

Status: HOLD / UNCERTAIN. This report is intentionally bounded and report-only.

## Snapshot

- CONFIRMED UTC: 2026-08-27T16:17:47Z.
- CONFIRMED WITA: 2026-08-28T00:17:47+0800.
- CONFIRMED public-main orientation SHA: `029f60d77e7e03192bc966de3a835a4a32a00fe2` (assignment base). Live `/Users/jay-m4/code/rawclaw` was on `integrate/tagwrite-closeout-wave1` at `0d1da19`, behind its upstream by 1, and dirty; therefore its checkout is not a public-main witness.
- CONFIRMED Graphify MCP: 3,712 nodes, 10,487 edges, 280 communities; 4 open main-targeting PRs.
- CONFIRMED handshake: `NONCE_FURIOSA_RECOVERY_5c2e9a71` echoed to Furiosa; supervisor cursor untouched.

## Active writers

| State | Evidence |
|---|---|
| COMPLETE_PRESERVE — CONFIRMED | `furiosa-t85-issue26-index-audit-luna`, session `01a043eb-d39d-73d3-a10b-34311eb112a1`, original worktree `/Users/jay-m4/code/rawclaw-furiosa-t85-issue26-index-audit`, commit `7185745`; no live Codex process now; clean, upstream 0/0. |
| COMPLETE_PRESERVE — CONFIRMED | `furiosa-t85-ozzy89c8-currentbase-luna`, session `01a043eb-d54e-72d3-a631-ac2bc6d6ab54`, original worktree `/Users/jay-m4/code/rawclaw-furiosa-t85-ozzy89c8-currentbase`, commit `bfac240`; no live Codex process now; clean, ahead 2 / behind 0. |
| COMPLETE_PRESERVE — CONFIRMED | `furiosa-t85-pr40-hostile-smoke-luna`, session `01a043eb-d502-7d60-ba2b-cf2b91795a68`, original worktree `/Users/jay-m4/code/rawclaw-furiosa-t85-pr40-hostile-smoke`, commit `0023160`; no live Codex process now; clean, upstream 0/0. |
| COMPLETE_PRESERVE — CONFIRMED | `furiosa-t85-pr43-toctou-luna`, session `01a043eb-d285-7e61-b534-52805467503b`, original worktree `/Users/jay-m4/code/rawclaw-furiosa-t85-pr43-toctou`, commit `2f1992a`; no live Codex process now; clean, upstream 0/0. |
| ACTIVE_WRITER — CONFIRMED | Rival `han-t87-prior-art`, session `01a043f8-4b39-7b00-9e5c-2ae2f8619f4d`, original worktree `/Users/jay-m4/code/rawclaw-han-t87-prior-art`, tmux pane and Codex PID 83814 live. Do not resume or alter. |
| ACTIVE_WRITER — CONFIRMED | Ozzy migration source thread `01a03ca0-d617-7c90-bfa4-6dc2d0316f7e`, shared root `/Users/jay-m4/code/rawclaw`, Codex PID 17963 live. Queue, do not resume; new route is `/Users/jay-m4/code/rawclaw-supervisor-ozzy-c`. |

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

## Second-pass mappings and recommendation

- CONFIRMED collision set: four T85 panes are completed-idle and preserved; only Han t87 and Ozzy’s shared-root thread are live writers.
- FIRST WAVE recommendation (6 max): preserve T85 issue26, T85 Ozzy, T85 PR40, T85 PR43, codex PR40 reconcile, and codex rescue; do not resume any. Queue Ozzy’s active thread before migration: `codex queue --thread 01a03ca0-d617-7c90-bfa4-6dc2d0316f7e --message 'Migrate this active thread to /Users/jay-m4/code/rawclaw-supervisor-ozzy-c; do not resume duplicate.'`
- Exact inactive resume form (only if later required): `cd <original-worktree> && codex resume <session-id>`; no current candidate meets resume criteria because preserved commits exist.
- Exact queue form once a thread ID is independently resolved: `codex queue --thread <id> --message 'Do not resume or duplicate this active writer; recovery sweep found live pane <pane>.'`
- Exact resume form only after proving inactivity: `cd <original-worktree> && codex resume <session-id>`.

## Evidence boundary

CONFIRMED: exact session-to-worktree mappings and current process/tmux state were independently inspected. UNCERTAIN: rollout freshness for older candidates outside the mapped sessions and remote tracking for Han (its worktree has no configured upstream). No claim is made beyond those limits.

## Final narrow candidate pass

- COMPLETE_PRESERVE — CONFIRMED: `/Users/jay-m4/code/rawclaw-codex-pr40-reconcile-20260827`, branch `codex/pr40-reconcile-20260827`, HEAD `8a87724d5e0e4bea4cb5bcf0be80019ab56fe319`, clean, origin tracking 0/0. No matching recent rollout `session_meta.cwd` was found; the durable branch/commit is sufficient and there is no resume action.
- COMPLETE_PRESERVE — CONFIRMED: `/Users/jay-m4/code/rawclaw-codex-rescue-12h-20260827`, branch `review/codex-rescue-12h-20260827`, HEAD `9939d239fdedd7175da23b12136d09dbdab38ec7`, clean, origin tracking 0/0. Session `01a0416b-fce6-7eb2-9146-ba96666651ba` maps to this worktree; no live process or tmux pane remains. No resume action.
- COMPLETE_PRESERVE — CONFIRMED: `/Users/jay-m4/code/rawclaw-furiosa-t57-prior-art-delta`, branch `worker/furiosa-t57-prior-art-delta-20260827`, HEAD `98ae85385a328f58640ec262e7c189c1d821083d`, clean, origin tracking 0/0. Sessions `01a04189-8a4b-7863-aa61-481d3730fec9`, `01a041ab-551e-7280-b80e-5d59287afe1f`, and `01a041b1-14f1-7d83-bba7-57cc29def130` map to this worktree; no live process or pane remains. No resume action.
- ACTIVE_WRITER — CONFIRMED: Han session `01a04403-d3d3-7611-b02b-479779ea55d5` remains live in tmux (`han-t87-prior-art`, PID 83814); queue only: `codex queue --thread 01a04403-d3d3-7611-b02b-479779ea55d5 --message 'Continue current Han t87 work; do not resume a duplicate.'`
- ACTIVE_WRITER — CONFIRMED: Ozzy source thread `01a03ca0-d617-7c90-bfa4-6dc2d0316f7e` remains live in shared root (PID 17963); queue only for migration: `codex queue --thread 01a03ca0-d617-7c90-bfa4-6dc2d0316f7e --message 'Migrate this active thread to /Users/jay-m4/code/rawclaw-supervisor-ozzy-c; do not resume duplicate.'`
