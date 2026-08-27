# Tick 56 harness cursor-integrity audit

Date: 2026-08-27 (WITA; command evidence is UTC)

## Verdict

**High severity, reproducible cursor-integrity defect.** The Phase 3 documentation is internally
consistent: workers own only their worker cursor, the startup handshake requires a dedicated worker
mailbox, and future or malformed receipts are invalid cursor and prior-art watermark evidence.
However, enforcement is not sufficient. The current `agent-mailbox-mark-read.sh` accepts an existing
future-dated filename as an explicit cursor target and then treats later real receipts as read. The
rule therefore still relies on human discipline and a compliant caller.

Smallest enforcement location: **the mark-read helper**. Reject explicit targets whose top-level name
is not a conforming UTC `YYYYMMDDTHHMMSSZ` prefix no later than `date -u`; reject targets that do not
exist; and make automatic advancement ignore hidden/quarantined/future or malformed entries. Keep
the documentation's quarantine and recovery procedure as the operator-facing recovery path. Scheduler
validation is useful defense in depth, but cannot protect cursors advanced by direct helper use.

## Contract comparison

The worker-mailbox handshake and Tick 55 future-receipt rule do not contradict one another:

- `startup-documentation.md:8-25` requires a dedicated worker mailbox and `AGENT_MAILBOX_DIR` set to
  it; `:27-45` assigns cursor ownership to the worker and forbids parent-cursor access.
- `startup-documentation.md:47-64` requires the nonce round trip before code or Graphify work.
- `startup-documentation.md:66-84` requires a conforming, non-future filename before marking and
  excludes future, hidden, malformed, and quarantined receipts from prior-art watermarks.
- `LAUNCH_NOW.md:118-127` repeats the same ownership, quarantine, and watermark boundary.
- `HARNESS_CHANGELOG.md:78-101` records the Tick 55 accepted correction and its prior future-cursor
  incident.
- `supervisor-tick.sh:168-183` carries the same prior-art watermark rule and uses the sender helper
  for scheduler mail. It does not itself advance a mailbox cursor.

The defect is the missing executable check at the cursor boundary. The mark helper only performs
lexical ordering (`agent-mailbox-mark-read.sh:38-47`) and unconditionally writes an explicit target
(`:57-74`). It therefore cannot enforce the documented contract by itself today.

## Disposable reproduction

The following was run in a fresh `mktemp -d` mailbox; no supervisor or rival mailbox was touched.
The fixture contents were only `tick 55`, `future injected receipt`, and `tick 56`.

Command:

```sh
MAILBOX_DIR="$(mktemp -d /tmp/t56-mailbox.XXXXXX)"
printf '%s\n' 'tick 55' >"$MAILBOX_DIR/20260827T044100Z-0001-tick55.md"
printf '%s\n' 'future injected receipt' >"$MAILBOX_DIR/20990101T000000Z-0002-future.md"
printf '%s\n' 'tick 56' >"$MAILBOX_DIR/20260827T044200Z-0003-tick56.md"
AGENT_MAILBOX_DIR="$MAILBOX_DIR" agent-mailbox-mark-read.sh --list
AGENT_MAILBOX_DIR="$MAILBOX_DIR" agent-mailbox-mark-read.sh 20990101T000000Z-0002-future.md
AGENT_MAILBOX_DIR="$MAILBOX_DIR" agent-mailbox-mark-read.sh --list
```

Observed output (2026-08-27T04:43:19Z UTC):

```text
before:
[agent-mailbox-mark-read] 3 unread message(s):
  - 20260827T044100Z-0001-tick55.md
  - 20260827T044200Z-0003-tick56.md
  - 20990101T000000Z-0002-future.md
mark future:
[agent-mailbox-mark-read] Mailbox cursor updated to: 20990101T000000Z-0002-future.md
[agent-mailbox-mark-read] Marked 3 message(s) as read.
cursor:
20990101T000000Z-0002-future.md
after future mark:
[agent-mailbox-mark-read] No unread messages (cursor: 20990101T000000Z-0002-future.md).
```

The command's current UTC was `2026-08-27T04:43:19Z`. The future fixture's filesystem mtime was
`2026-08-27T12:43:19Z`; its SHA-256 was
`83b076a4d53599e4aaade769e952c0a454b94d5be60e464a8ba924972baec41e`. The real tick fixture
SHA-256 values were:

```text
eac86563ae0394eb8b958d4abe1f2d324f2ba355c9d51443403220b711c171cb  tick55
bba8a6c2c19102cd6a3776093c37b0abeec0b2e8e1affd61907e8a444fc01a73  tick56
```

This proves acceptance of a future explicit target and suppression of a later real tick. The helper
also does not verify that an explicit target exists before writing it.

## Malformed receipt observation

The handshake ACK supplied this supervisor observation: filename `20260827T044000Z` arrived while
current UTC was `20260827T043902Z` and lacked a `date:` header. It is consistent with the documented
future/malformed class, but this audit did not alter or inspect that supervisor mailbox, so the case
is recorded as supervisor-provided evidence rather than independently reproduced here.

## Checks and source hashes

Commands run:

```text
bash -n two-supervisor-harness/bin/supervisor-tick.sh: PASS
bash -n two-supervisor-harness/bin/supervisor-24h-watchdog.sh: PASS
bash -n steering-kit/bin/agent-mailbox-mark-read.sh: PASS
```

SHA-256 of the audited inputs:

```text
86a094f6010768dc753e6ea1727f425cd8018c2d84e1c4794ccb107aa4953086  LAUNCH_NOW.md
89b77dac3a136e9819ab60168c602c71bbb359df4388ce4f5b3775562093deed  startup-documentation.md
644f3cb8771aacf6c9cd5b9c474a5d40bb8acc02b2abc6b2de5324e33512faf3  state/HARNESS_CHANGELOG.md
11c61f5e0b378abe6a385971215a749c90cc7d60634aabf679037cfe49f496c3  bin/supervisor-tick.sh
420f01b222750e84d3a8fe6795ba2005f85361c5c464e8367399ab0f4bd8222f  bin/supervisor-24h-watchdog.sh
7be2a25201efa051f5f284fba43de6161145f0333d77c75fbaa459848392d155  agent-mailbox-mark-read.sh
```

The separate lifecycle evidence that `tmux send-keys -l` followed by `Enter` is required for actual
prompt submission is relevant context but outside this cursor-integrity finding; durable receipt
ordering must not be conflated with prompt submission.

## Remaining proof boundary

This report proves the present helper's future-target failure. It does not prove a corrected helper's
quarantine move, concurrent writer behavior, or scheduler recovery of ticks hidden by a previously
poisoned cursor. Those require a focused implementation and mutation suite. Until then, Phase 3
documentation improves operator behavior but does not constitute executable cursor integrity.
