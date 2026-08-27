# Canonical RawClaw Agent Registry

Snapshot and run completion: **2026-08-27T04:45:27Z UTC / 2026-08-27T12:45:27+0800 WITA**. This reconciliation is read-only with respect to accepted source checkouts. It contains **198 identity/session rows**: **196 exact mailbox identity records**, plus **2 live-only identities** added only from independent process and CWD evidence. It grants no score, merge, cleanup, or product authorization.

## Accepted inputs and integrity

- Live checkout `2c306f4039060ef53f4962c49bba66d6ab394995`; report SHA-256 `2989a9a1229c20c1b70b4e798544117d5d45999765d7ed2afefa50c0a3f0c284`; CSV SHA-256 `b71964c292dfd616c643e0d9e1a8f9c982ebaa44761bf030329a66d57a08ca66f`; 7 rows.
- Mailbox checkout `2cfd07e3b27d0f0ff041e9f82db1c16ad2fc9a58`; report SHA-256 `bf5a0f5db7742c65ea13dc05920d2ae97e1a945b63df40e8735cef82f5a6fd0a`; CSV SHA-256 `ecd25733e3d3e44aed6dd3392ee568fc2fa1732231eff2241ac5bb7c9f3ffcd9`; 196 identity rows, 4,761 messages, 192 mailbox directories; completion/reference `2026-08-27T04:45:27Z`.
- Git checkout `b9fe72481249a518237c3294e950c6e650bb6861`; report SHA-256 `6ff6d12225e2c368238248de92ec73328e149213b89cd240de56e81b21e4c97f`; CSV SHA-256 `2a0dda1d402ee83edc0e36eb70b7b47647df28df22f449fc11cabd499c88964f`; 715 artifact rows: 410 attached worktrees, 27 detached worktrees, 32 branch-only records, 246 origin refs.

## Counts

Status counts: `reported-running=30, confirmed historical=59, completed=76, ambiguous=31, stale=1, confirmed-running=1`.

Confidence counts: `medium=51, high=115, low=32`.

## Current live and terminal roster

- Sarah Connor: terminal receipt `d81b6de`, own-upstream 0/0 clean; patch-identical to adopted `0152683`; candidate baseline and two hostile mutants independently verified. Requested lint v2.13.1 is UNCERTAIN because only v2.12.2 was installed.
- Ada Lovelace: terminal receipt `15bae63`; four independent real SQLite-writer variants remained blocked 351–354 ms after cancellation. Gate and `ExecContext` substitutions were insufficient; no product integration is authorized.
- Margaret Hamilton: terminal receipt `020b1eb`; independently reproduced future/nonexistent explicit cursor poisoning and the clean-empty-mailbox Bash 3.2 nounset crash. Helper fix is not implemented or authorized.
- Han supervisor: TUI alive at tmux `han-recovery-20260827-2113`, PID 63333, exact CWD and branch@HEAD verified. Branch/result claims remain UNCERTAIN. Supervisor mailbox was not inspected.
- Hypatia T57 prior-art candidate: proposed process was not observed and is excluded from the current live roster.

## Identity join rules

Mailbox records are preserved one-for-one; aliases are never merged from name similarity, shared mailbox paths, shared branches, shared commits, or artifact proximity. A row gains a session ID, tmux/PID, worktree, branch@HEAD, mailbox, or immutable receipt only from exact textual or process/CWD evidence. The two live-only rows satisfy the independent process+CWD rule. Self-reports remain labeled until corroborated.

## Git artifact coverage and unresolved mappings

The 715 Git records are coverage, not 715 agents: 410 attached worktrees, 27 detached worktrees, 32 branch-only records, and 246 origin refs. Relevance is 406 same-base, 252 diverged, and 57 ancestor. Artifact-to-identity mappings remain unresolved unless an immutable exact join exists; the CSV does not manufacture agent rows from Git artifacts.

## Heartbeat and wake status

Furiosa’s exact thread heartbeat mechanism B was verified: ticker targets tmux `rawclaw-supervisor-furiosa-wake-relay`, and relay queued self-test message `01a04187-d060-7ad3-b30f-9ac0a215b1e6` into thread `01a03fdb-2bb0-70a3-966e-4163be3ab394` at `2026-08-27T04:43:34Z`. This is infrastructure evidence, not an agent identity. The two-supervisor ticker/watchdog and Codex rescue/pull watcher are likewise infrastructure automation and excluded from the identity count.

## Mailbox and cursor failures

The accepted mailbox census is textual evidence: one-way/partial nonce states, missing explicit identity fields, aliases, and stale or ambiguous senders remain visible in CSV. Margaret’s audit shows that the mark-read helper accepts future explicit targets, can suppress later real receipts, does not verify an explicit target exists, and that a clean empty mailbox crashes Bash 3.2 nounset at `MESSAGE_FILES[@]`. No helper repair was made here.

## Collaboration-CWD failure

A prior collaboration reconciler shared the supervisor CWD. Its mailbox guard surfaced supervisor receipts despite the prompt fence; it did not advance its dedicated cursor or send acknowledgments and moved the supervisor cursor backward and then to a future receipt. Its empty worktree and claims are excluded from registry evidence except as a control-plane incident. Supervisor mailbox contents were not read or advanced in this run.

## Refresh procedure

1. Re-run the live, mailbox, and Git collectors at pinned commits and verify report/CSV SHA-256 values before reading rows.
2. Process only the reconciler’s dedicated mailbox; complete the nonce handshake and preserve its cursor boundary.
3. Rebuild identity rows from the mailbox CSV without alias merging; add live-only identities only with independent process+CWD evidence.
4. Join worktree, branch, PID, tmux, session, and receipt data only on exact immutable evidence. Keep Git artifacts as coverage counts.
5. Re-run Ruby CSV parse/count and `git diff --check`; inspect the diff for forbidden product files before commit and push.
6. Record updated snapshot UTC and WITA times, source hashes, unresolved gaps, and mailbox/cursor incidents.

## Index

- `AGENTS.md` — governing RawClaw constraints, worktree discipline, and output fence.
- `AGENT_REGISTRY.md` — this reconciliation’s source hashes, identity rules, evidence limits, and refresh procedure.
- `agent-registry.csv` — one row per evidenced identity/session; use for structured filtering and counts.
