# T66 ticker finish — executable transplant trial

Date: 2026-08-28 WITA (UTC+8)  
Scope: ticker inventory `772dc6b1fb34115568718cc49537226cfb91aabf`; activation `9cc6099cb7e461056e7f2e0f7f3a0b94fafe17d2`.  
Base: public `main` `f73ee0e89c30d1c5429068708325426d5c5b5fbe`.

## Graphify-first orientation

Graph: `/private/tmp/furiosa-ticker-graph.9F4VFZ/harness/graphify-out/graph.json`.

- `graphify reflect --if-stale`: lessons current; `send_tick()` and `supervisor-tick.sh` are tentative useful results.
- Vocabulary query `restart_ticker send_tick wake_supervisor`: found `restart_ticker()` at `bin/supervisor-24h-watchdog.sh:16`, `send_tick()` at `bin/supervisor-tick.sh:214`, and `wake_supervisor()` at `:281`.
- `graphify explain restart_ticker`: watchdog restart function.
- `graphify path send_tick wake_supervisor`: both are defined by `supervisor-tick.sh`; no watchdog-to-wake edge was found.

## Immutable candidate and unchanged trial

Activation commit `9cc6099cb7e461056e7f2e0f7f3a0b94fafe17d2` is documentation-only (`HAN_TICKER_ACTIVATION.md`, 30 lines); it records the persistent command:

```text
while :; do supervisor-tick.sh supervisor-tick.env; sleep 600; done
```

I copied the harness and mailboxes to disposable `/private/tmp/t66-trial.Uughpo`, rewrote only the disposable config paths, and ran that command unchanged under `gtimeout 12s`.

- exit `124` at `12s`: expected outer timeout while the unchanged `sleep 600` remained active.
- inner script completed at `2026-08-27T18:54:59Z`, exit `0`.
- disposable `tick.count`: `103`.
- two disposable mailbox receipts were created (A and B).
- stderr recorded both missing-tmux warnings and `sent tick 103; run_completion_utc=...`.

This is `UNCHANGED-RUN PASS` for one tick and durable advancement, but it does not establish a bounded failure path.

## Live evidence (read-only)

`ps`/`lsof` for PID `28906` observed:

```text
28906 28815 03:12:14 zsh -c while :; do .../supervisor-tick.sh .../supervisor-tick.env; sleep 600; done
fcwd /Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness
```

The inspected watchdog source at lines 16–20 is the same unbounded command: `tmux kill-session`, then `tmux new-session ... "while :; do ${tick_script} ${tick_env}; sleep 600; done"`.

The watchdog log was stale at tick `93` (last shown `2026-08-27T17:07:22Z`), while the live PID was still present. Historical log cadence showed tick 90→91 after roughly 10 minutes and did not prove failure-path recovery.

## Failure simulation and smallest disposable repair

I ran the unchanged loop against a disposable config whose `RAWCLAW_REPO_DIR` was `/private/tmp/nonexistent-t66-repo`, forcing the `git ls-remote`-equivalent validation failure during prior-art preparation.

- command: `gtimeout 4s sh -c 'while :; do supervisor-tick.sh config.env; sleep 600; done'`
- observed error: `RawClaw repository directory is missing: /private/tmp/nonexistent-t66-repo`
- exit `124`, elapsed `4047ms`.
- the loop did not establish a bounded next cycle; it entered the 600-second sleep after the failure path.

`TICKER_FINISH.patch` records the smallest disposable launcher change: wrap each tick invocation with `gtimeout --signal=TERM --kill-after=5s 60s ... || true`, retaining the existing 600-second cadence.

For a deterministic ls-remote-hang equivalent, two repaired cycles used `gtimeout 2s sh -c 'sleep 30'`:

```text
cycle=1 timeout_rc=124
cycle=2 timeout_rc=124
REPAIRED_EXIT=0
REPAIRED_ELAPSED_MS=4061
```

Both failure cycles returned at the 2-second deadline; no 30-second hang escaped. This is disposable simulation evidence only, not a live harness edit.

## Verdict

`PATCH/HOLD — REJECTED_AFTER_EXECUTABLE_TRIAL` for adoption of activation `9cc6099cb7e461056e7f2e0f7f3a0b94fafe17d2` as a reliable persistent scheduler. The unchanged transplant proves one successful tick but leaves a failure-path hang/sleep cadence unbounded. The one-line `gtimeout` wrapper is the smallest candidate repair, but it remains untested against the live harness and therefore is not a merge or publication authorization.

No live harness file, supervisor mailbox cursor, PR, merge, or shared state was modified.
