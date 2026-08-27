# T97 ticker dedup inventory

Date: 2026-08-28 WITA (UTC+8)
Inventory checkout: `worker/furiosa-t97-ticker-inventory@f73ee0e89c30d1c5429068708325426d5c5b5fbe`

## Verdict

**HARVEST EXISTING / REUSE.** Do not launch a new 21m58s ticker-gap repair. An existing
activation lane is present and its persistent ticker is live. The lane is not proof that
supervisor follow-through is healthy: the watchdog evidence is stale, so harvest and gate
the existing lane before any repair decision.

## Existing implementation and live status

| Evidence | Exact receipt | Owner/status |
|---|---|---|
| Existing activation branch | `han/luna-ticker-activation-20260827@9cc6099cb7e461056e7f2e0f7f3a0b94fafe17d2`; remote branch `origin/han/luna-ticker-activation-20260827` points to the same SHA | Han/Luna activation lane; clean checkout observed |
| Existing activation report | `/Users/jay-m4/code/rawclaw-han-luna-ticker/HAN_TICKER_ACTIVATION.md`, SHA-256 `81cf1344cb1fd26e6105132a3b272b178a7590bbed2576f4b152cba64d0ce73f` | Records one-shot tick 0 PASS; persistent follow-through was initially UNCERTAIN |
| Persistent ticker process | PID `28906`, started `2026-08-27 23:43:11` WITA; command is `while :; do .../two-supervisor-harness/bin/supervisor-tick.sh .../.agent-mailbox/supervisor-tick.env; sleep 600; done` | Live process observed in `rawclaw-supervisor-ticker` |
| Live ticker state | `/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/state/tick.count` contains `98`, SHA-256 `4960a9ce196ee53a9ae6f2b038ebf5bb7949312406eabe79835e766b4a3d0d88`, mtime `2026-08-28 02:06:13` WITA | Persistent loop advanced through tick 98 |
| Ticker implementation | `/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/bin/supervisor-tick.sh`, SHA-256 `c45c35004e6b7fcbaca096ea3d0575ac7b3820df956a92b0598d9222bba37032` | `send_tick` queues both mailboxes, then `wake_supervisor`, then advances state |
| Private config | `/Users/jay-m4/code/rawclaw-han-luna-ticker/.agent-mailbox/supervisor-tick.env`, SHA-256 `a6ca0ceea9d324c6ef07ad936b8bb2e0a3bcad51245257af0ab7f033858c4869` | Targets Furiosa and Han mailboxes and wake relays; not committed |
| Watchdog freshness | `supervisor-24h-watchdog.log` last observed line `2026-08-27T17:07:22Z tick=93 age=208s ticker=alive`, SHA-256 `397ca93e92a74e99804b620f6cf1e7685ad99911de758f23f85dda83862aba47` | **STALE** relative to tick 98; supervisor follow-through remains unproven |

## Dedup search

- Sibling path `/Users/jay-m4/code/rawclaw-furiosa-t98-ticker-inventory` exists only as a
  directory containing `.agent-mailbox`; it is not a Git worktree and has no report/branch.
- No other ticker/21m58/scheduler branch or worktree was found. The only matching Git branch
  is `han/luna-ticker-activation-20260827`; the only matching worktree is
  `/Users/jay-m4/code/rawclaw-han-luna-ticker`.
- GitHub search found no ticker issue or PR. Issue #39 is the closed supervisor closeout issue;
  no public ticker repair is published. `git ls-remote origin refs/heads/main` returned
  `f73ee0e89c30d1c5429068708325426d5c5b5fbe`.
- The activation report explicitly says the one-shot sends and state advancement are proven,
  while persistent scheduler and supervisor follow-through were UNCERTAIN. Current live state
  upgrades only the scheduler fact (tick 98); the stale watchdog prevents claiming end-to-end
  supervisor health.

## Graphify receipt

Used the supplied code-only graph at
`/private/tmp/furiosa-ticker-graph.9F4VFZ/harness/graphify-out/graph.json` after
`graphify reflect --if-stale` and reading `reflections/LESSONS.md`.

- Literal query `ticker scheduler loop` surfaced `restart_ticker()` and prior ticker evidence.
- Literal query `public main resolver ls remote` surfaced `public_blast()` and branch/public-main
  freshness helpers.
- Literal query `prior art SHA` surfaced `supervisor-tick.sh`, `sha256_file()`, and the cumulative
  prior-art ledger.
- `graphify explain "supervisor-tick.sh"` identified `send_tick()` and `wake_supervisor()` as
  defined connections.
- `graphify path "send_tick()" "wake_supervisor()"` returned the two-hop path through the
  `supervisor-tick.sh` script.

## Disposition

Existing lane should be harvested: verify current mailbox delivery receipts, inspect the stale
watchdog gap, and only then decide whether a narrowly fenced repair is needed. This report owns
no product or harness files and launches no repair.

Terminal: **SHIP** (inventory report only; existing ticker lane is the reusable candidate).
