# Tick 59 harness mailbox-rule audit

## Verdict

Workers appeared to lose mailboxes for two different reasons. The earlier failure was supervisor-mailbox inheritance: a worker process received the supervisor's `AGENT_MAILBOX_DIR` and could trigger the parent mailbox guard or advance the parent cursor. The later Tick 35 correction removed that inheritance path, but introduced a separate visibility failure by allowing a worker-local mailbox **or no mailbox**. The evidence supports missing mailbox provisioning, not deletion of permanent worker mailboxes.

The smallest correction is the accepted rule already recorded in the harness: require one permanent dedicated mailbox for every worker, bind it at process launch, require a two-way nonce handshake before work, and reserve cursor ownership to that worker. Keep supervisor mail as a send-only receipt destination; never use it as a worker inbox.

## Evidence chain

Graphify orientation over `/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/graphify-out/` connected `WORKER_PROMPT.md`, `startup-documentation.md`, `TEN_MINUTE_ROTATION.md`, and `bin/supervisor-tick.sh`. The relevant graph nodes are `Non-negotiable mailbox invariant`, `Ownership boundary`, `Required launch handshake`, and `Failure corrected`; the graph's `LESSONS.md` identifies `send_tick()` and `supervisor-tick.sh` as the only prior useful ticker findings. I then verified the source documents directly.

### 1. Inheritance and cursor violation

`/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/state/HARNESS_CHANGELOG.md` records the earlier defect:

> “Worker conduct rules prohibited parent-mailbox access, but the launch contract did not require an explicit non-inheritance rule for `AGENT_MAILBOX_DIR` or reserve scheduler/cursor ownership to the supervisor.”

The same entry says the old rule gave workers “a worker-local mailbox or none” and that only the supervisor should process ticks and advance its cursor. The launch contract now states at `/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/LAUNCH_NOW.md:114-118`:

> “The worker owns and advances only its dedicated worker cursor. It may send receipts into MY_MAILBOX, but it never reads, acknowledges, marks, or advances MY_MAILBOX or RIVAL_MAILBOX. Launch the worker process with `AGENT_MAILBOX_DIR` pointing exactly to its worker mailbox; never inherit the supervisor mailbox and never unset the variable.”

`startup-documentation.md` makes the process boundary explicit:

> “It must never inherit the supervisor's `AGENT_MAILBOX_DIR`, and it must never run with the variable unset.”

and:

> “A supervisor reads, acts on, marks, and advances only its own supervisor mailbox. A worker reads, acts on, marks, and advances only its own worker mailbox.”

This explains why workers could appear to consume or lose mail: the inherited variable exposed the wrong inbox and cursor. It does not show that a worker mailbox was deleted.

### 2. The no-mailbox visibility gap

The decisive current wording is `/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/startup-documentation.md:108-113`:

> “Tick 35 correctly stopped workers from inheriting and advancing the supervisor cursor, but the rule allowed a worker-local mailbox **or no mailbox**. That fallback removed the visible control plane and permitted hours of silent worker activity or inactivity. The corrected rule preserves cursor isolation while requiring a dedicated mailbox for every worker.”

The accepted changelog entry at `state/HARNESS_CHANGELOG.md:53-63` independently says:

> “Workers could launch without mailboxes as long as they did not inherit the supervisor mailbox.”

and records the replacement:

> “Every supervisor and worker has a dedicated mailbox. Worker startup is blocked until a two-way nonce handshake succeeds.”

This is mailbox absence at launch, not mailbox deletion after launch. The current filesystem also contains this worker's permanent mailbox at `/Users/jay-m4/code/rawclaw-furiosa-t59-harness-mailbox-rule/.agent-mailbox`; its cursor is worker-owned and was advanced only there. No recovery-harness rule or receipt inspected here records an `rm`, `rmdir`, or other deletion of a worker mailbox. The only `rm`/`rmdir` references in the exact referenced ticker helper are lock cleanup in `bin/supervisor-tick.sh`, not mailbox cleanup.

## Executable-boundary challenge

Han's challenge asked for an executable launch guard rather than documentation alone. The exact referenced helper hashes at audit time are:

- `/Users/jay-m4/org/builds/steering-kit/bin/agent-mailbox-send.sh` — SHA-256 `d9f0d2c7d052767a4ba620417887b2ae782a2517460805fe1cfc61a49d6b56ca`.
- `/Users/jay-m4/org/builds/steering-kit/bin/agent-mailbox-mark-read.sh` — SHA-256 `7be2a25201efa051f5f284fba43de6161145f0333d77c75fbaa459848392d155`.
- `/Users/jay-m4/code/rawclaw-session-recovery-20260827/two-supervisor-harness/bin/supervisor-tick.sh` — SHA-256 `11c61f5e0b378abe6a385971215a749c90cc7d60634aabf679037cfe49f496c3`.

The smallest executable guard is at the process boundary: create the worker directory, export `AGENT_MAILBOX_DIR="$WORKER_MAILBOX"` in the worker's environment before starting it, refuse launch if the value is empty or differs from the recorded absolute path, and make every worker `mark-read` invocation inherit that same value. The worker must then prove the nonce round trip before any repository action. A receipt sent to a supervisor uses a one-command `AGENT_MAILBOX_DIR="$SUPERVISOR_MAILBOX"` override only for `agent-mailbox-send.sh`; it must never use that override for read or mark operations.

The requested pre-fix RED receipt hashes are not present in the inspected recovery-harness documents. The documents do preserve the immutable failure descriptions and, for the separate future-cursor incident, hashes `6bec1ec147626feac168a6617de6d19e91f3344a3bb13c70172b9da9b522befa` and `94ed7d97a5608a424b2696d9e00a7733c7a3ca3772a2bf6bb809467b1b325b07`. I therefore do not invent a RED hash for the inherited-mailbox incident; that field remains `UNCERTAIN` until an immutable receipt or file hash is supplied.

Cleanup authority remains supervisor-owned: a supervisor may direct a worker's termination and remove its worktree only after preservation proof; a worker never cleans or changes a supervisor or rival mailbox. The separate tmux submission invariant is also explicit in the challenge evidence: send the prompt with `send-keys -l`, then send Enter, and accept `UserPromptSubmit` as proof that the prompt was submitted. It is not proof of mailbox ownership or cursor authorization.

### 3. Why all four supervisors can steer every worker

The required handshake in `startup-documentation.md:47-60` requires the supervisor to create and record the dedicated worker path, the worker to start with that exact `AGENT_MAILBOX_DIR`, and both sides to exchange a nonce before code inspection or work. The ownership rule permits receipts into the supervisor mailbox without granting cursor ownership. Therefore each worker has one durable control inbox, while each supervisor can steer or challenge it by sending to that inbox. A worker must not consume scheduler ticks or any supervisor cursor.

The smallest safe operational contract is therefore:

1. Create one permanent mailbox directory per worker and never reuse a supervisor mailbox.
2. Launch with `AGENT_MAILBOX_DIR` set exactly to that worker path; fail closed if isolation is unavailable.
3. Complete and verify the two-way nonce handshake before Graphify, grep, edits, tests, or commits.
4. Have every supervisor send steering to the worker's dedicated mailbox; workers send receipts to supervisors only through an explicit one-command target override.
5. Mark only the cursor owned by the current process. Treat no-mailbox, inherited-mailbox, failed-handshake, or stale-receipt workers as failed launches.

## Audit receipts

- Initial unread mail was read from `/Users/jay-m4/code/rawclaw-furiosa-t59-harness-mailbox-rule/.agent-mailbox`; only its `.cursor` was advanced.
- Exact nonce `nonce-ellen-external-729e5c` was sent with `/Users/jay-m4/org/builds/steering-kit/bin/agent-mailbox-send.sh` to `/Users/jay-m4/code/rawclaw-supervisor-furiosa-a/.agent-mailbox` and accepted in `20260827T052003Z-2a0448d0-ellen-external-nonce-accepted.md`.
- Mailbox advertisements were sent to Han, Ozzy, Khan, and verified Lenny. No supervisor cursor was read or advanced.
- The recovery harness was not modified.
