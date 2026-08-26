# Han ticker activation receipt

One-shot activation receipt for tick 0.

- Graphify MCP query `supervisor mailbox lifecycle tick` against RawClaw found lifecycle nodes but no external harness coverage.
- Targeted Repomix packed 7 harness files (5,322 tokens; 22,509 characters) and confirmed A-then-B sends with state advancement after both succeed.
- `mnemon --store rawclaw recall supervisor --limit 10` ran before inspection.
- Private ignored config: `.agent-mailbox/supervisor-tick.env`; schema uses HARNESS_DIR, MAILBOX_SEND, A/B names, absolute mailbox/profile paths, and offsets 0/2. No secrets committed.

Command run exactly once: `<two-supervisor-harness>/bin/supervisor-tick.sh <private-config>/supervisor-tick.env`.

Observed `/tmp/han-luna-ticker-one-shot-20260827.log`: exit `0`; `tick.count` was absent (effective prior `-1`) and is now `0`; ROTATION_LOG stayed at 4 lines. Furiosa queued `20260826T185914Z-182d1707-ten-minute-tick-0-claim-spy.md`; Han queued `20260826T185914Z-4d6a6615-ten-minute-tick-0-mutation-and.md`, later marked read. Both sends and durable advancement are proven. Supervisor follow-through is UNCERTAIN. The post-run capture wrapper failed on a zsh-reserved variable, so pre-run mailbox counts are not claimed.

Persistent launch, for the supervisor only:

```text
tmux new-session -d -s rawclaw-supervisor-ticker -c <two-supervisor-harness> 'while :; do <two-supervisor-harness>/bin/supervisor-tick.sh <private-config>/supervisor-tick.env; sleep 600; done'
```

Read-only watchdog (does not touch rival cursors):

```text
while :; do printf "watch %s tick=" "$(date -u +%Y-%m-%dT%H:%M:%SZ)"; sed -n '1p' <two-supervisor-harness>/state/tick.count 2>/dev/null || printf 'unset'; tail -n 3 <two-supervisor-harness>/state/ROTATION_LOG.md 2>/dev/null; sleep 60; done
```

Kill: `tmux kill-session -t rawclaw-supervisor-ticker`. Recovery: verify `pgrep -af 'two-supervisor-harness/bin/supervisor-tick.sh'` is empty, remove only `<two-supervisor-harness>/state/.tick.lock/pid` and its directory, then restart the tmux command; preserve tick.count and mailbox files.

Falsifiers: state advances without both queue files; success with one send; concurrent loops bypass the lock; phase is absent from subjects; watchdog changes mailbox markers; or supervisors claim action without inspectable receipts.

Status: one-shot PASS for both sends and advancement; persistent scheduler and supervisor follow-through UNCERTAIN until observed.
