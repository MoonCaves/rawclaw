# Live Agent Registry

Snapshot: WITA 2026-08-27T12:36:39+0800 (UTC 2026-08-27T04:36:39Z). Evidence is read-only. A PID or pane alone is not treated as a unique agent.

## Confirmed running

- Sarah Connor, Luna medium: tmux `furiosa-t56-77947bd-verifier-luna`, pane PID 63419, codex child 63507; CWD `/Users/jay-m4/code/rawclaw-furiosa-t56-77947bd-verifier`; branch@HEAD named in prompt `worker/furiosa-t56-77947bd-verifier-20260827@77947bd769ac9cf219aaa68fc2f06b336dd9bea5`; exact semantic/race/lint gates and two RED mutations specified; result UNCERTAIN.
- Ada Lovelace, Luna medium: tmux `furiosa-t56-execcontext-minimal-luna`, pane PID 63435, codex child 63512; CWD `/Users/jay-m4/code/rawclaw-furiosa-t56-execcontext-minimal`; branch@HEAD `worker/furiosa-t56-execcontext-minimal-20260827@48661f403f880e2c1dac7615f39bbb8264eeafe7`; four cancellation variants specified; result UNCERTAIN.
- Margaret Hamilton, Luna medium: tmux `furiosa-t56-harness-cursor-audit-luna`, pane PID 63452, codex child 63526; CWD `/Users/jay-m4/code/rawclaw-furiosa-t56-harness-cursor-audit`; branch named in prompt, HEAD UNCERTAIN; disposable mailbox reproduction specified; result UNCERTAIN.
- Han supervisor: tmux `han-recovery-20260827-2113`, pane PID 63333, `gpt-5.6-sol`, reasoning `xhigh`; CWD `/Users/jay-m4/code/rawclaw-supervisor-han-b`; alive TUI, branch/result UNCERTAIN.

## Infrastructure and uncertain

- Supervisor ticker PID 59492 and watchdog PID 43675 run from the two-supervisor harness; infrastructure automation, not unique agents.
- Codex rescue PID 68995 and pull watcher PID 69005 run in `/Users/jay-m4/code/rawclaw-codex-rescue-12h-20260827`; output target `/private/tmp/rawclaw-codex-rescue-12h-20260827.result`; result UNCERTAIN.
- PID 6492 runs `rawclaw archive autosync --timeout 0`; runtime automation.
- Long-lived Codex app-server/Graphify processes in older worktrees lack a safe mailbox/session join and are classified UNCERTAIN.

## Collector and evidence limits

Collector: `/Users/jay-m4/code/rawclaw-furiosa-agent-registry-live`, branch `worker/furiosa-agent-registry-live-20260827@c818ea1212bb1f1110cefa65472f658b844840ef`, mailbox `.agent-mailbox`. At snapshot, checkout was clean before report creation and `origin/main...HEAD = 1 0`; local branch is one commit behind origin/main. No worker result was observed in the targeted evidence. Codex session JSONL files were fresh but not safely joinable for every agent. Log freshness, dirty/upstream state of other worktrees, exact gate outcomes, mutations, patch identity, and current-base relevance are UNCERTAIN unless stated above. No process was killed and no worktree was reset, deleted, or removed.
