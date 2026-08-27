# TICK59 Mailbox Topology Audit

Date: 2026-08-27 WITA
Worker: Beatrix Kiddo
Branch: `worker/furiosa-t59-mailbox-topology-20260827`

## Boundary and cursor safety

- Beatrix mailbox: `/Users/jay-m4/code/rawclaw-furiosa-t59-mailbox-topology/.agent-mailbox`
  exists, mode `755`, inode `104282116`, cursor present. Only this cursor was advanced:
  first to `20260827T051244Z-2b771438-authorized-external-relaunch-w.md`, then to `README`
  after later local steering messages were observed.
- No supervisor cursor contents were read or advanced. Supervisor cursor safety below is
  therefore an existence/inode check only, not a claim about their cursor values.
- The registry-plus-current-supervisor reference set contains 158 mailbox directories:
  158 exist; 106 have a cursor; 52 have no cursor. No referenced directory is missing or
  deleted, so no directory was restored.

## Current supervisors and active workers

| mailbox | exists | mode | inode | cursor | topology result |
|---|---:|---:|---:|---|---|
| `/Users/jay-m4/code/rawclaw-supervisor-furiosa-a/.agent-mailbox` | yes | 755 | 95809274 | present | distinct supervisor mailbox; cursor not read |
| `/Users/jay-m4/code/rawclaw-supervisor-han-b/.agent-mailbox` | yes | 755 | 95809751 | present | distinct supervisor mailbox; cursor not read |
| `/Users/jay-m4/code/rawclaw-wt-instant-closeout-spec/.agent-mailbox` | yes | 755 | 88782564 | present | distinct active-worker mailbox |
| `/Users/jay-m4/code/rawclaw-khan-supervisor/.agent-mailbox` | yes | 755 | 104217250 | absent | uninitialized cursor boundary; steerability **UNCERTAIN** |
| `/Users/jay-m4/code/rawclaw-lenny-raid-phase/.agent-mailbox` | yes | 755 | 91638627 | present | verified active Lenny mailbox |

All five advertised destinations have different directory inodes. No shared physical
mailbox was found among those five. The shared mailbox identified in the reference set is
`/Users/jay-m4/code/rawclaw/.agent-mailbox` (inode `72070366`), which is referenced by
multiple lanes and is not a safe substitute for a lane-owned mailbox.

## Messages sent

The permanent-mailbox advertisement was sent with `agent-mailbox-send.sh` to:

- `/Users/jay-m4/code/rawclaw-supervisor-furiosa-a/.agent-mailbox/20260827T051742Z-2d6c125e-t59-permanent-mailbox-advertis.md`
- `/Users/jay-m4/code/rawclaw-supervisor-han-b/.agent-mailbox/20260827T051742Z-5f1d5c27-t59-permanent-mailbox-advertis.md`
- `/Users/jay-m4/code/rawclaw-wt-instant-closeout-spec/.agent-mailbox/20260827T051742Z-10ce25f0-t59-permanent-mailbox-advertis.md`
- `/Users/jay-m4/code/rawclaw-khan-supervisor/.agent-mailbox/20260827T051742Z-427e6fb9-t59-permanent-mailbox-advertis.md`
- `/Users/jay-m4/code/rawclaw-lenny-raid-phase/.agent-mailbox/20260827T051742Z-69c36bf7-t59-permanent-mailbox-advertis.md`

The authoritative relaunch nonce `nonce-beatrix-external-0f729a` was sent separately to:

- `/Users/jay-m4/code/rawclaw-supervisor-furiosa-a/.agent-mailbox/20260827T051537Z-15f95fad-external-relaunch-nonce.md`

## Graphify evidence

Graphify orientation and revisits were run before filesystem/source inspection. The current
worktree has no `graphify-out/graph.json`, so its local query was a recorded dead end. Queries
against the existing RawClaw graph surfaced `Mailbox lifecycle mapping`, `Active inventory:
existence versus activation`, and `Worker roster and live evidence`; the filesystem and
registry inventory above corroborated those leads.

## Separate cursor-defect evidence

These are distinct mechanisms and are not collapsed into the topology result:

1. **Future or nonexistent explicit target poisoning:** the current helper accepts a
   future filename and an explicit filename that is not present, then writes it as the
   cursor. The affected owner is whichever `AGENT_MAILBOX_DIR` the caller selects; no
   owner check exists. Prior disposable-fixture output hashes are recorded in
   `TICK56_HARNESS_CURSOR_INTEGRITY_AUDIT.md`: future fixture
   `83b076a4d53599e4aaade769e952c0a454b94d5be60e464a8ba924972baec41e`, tick55
   `eac86563ae0394eb8b958d4abe1f2d324f2ba355c9d51443403220b711c171cb`, and tick56
   `bba8a6c2c19102cd6a3776093c37b0abeec0b2e8e1affd61907e8a444fc01a73`.

2. **Monotonic regression:** concurrent explicit acknowledgements can leave the cursor at
   an older target; the independent boundary attack observed 96 regressions in 100 trials.
   This is separate from future-name validation and remains **OPEN**.

3. **Bash 3.2 empty-array behavior:** an absent-cursor empty disposable mailbox crashes at
   `MESSAGE_FILES[@]: unbound variable` on line 50 under `set -u`. The subsequent first-message
   acknowledgement initializes the cursor successfully. The captured reproduction output SHA-256
   is `cb133c0e1f1a531e1ba2a39dd20b46644f79bc29f7aea2b157ad80a8a54e11e9`.

Audited helper: `/Users/jay-m4/org/builds/steering-kit/bin/agent-mailbox-mark-read.sh`,
source commit `7e213da6dd61e282d5cbfa7345868152e8965750`, SHA-256
`7be2a25201efa051f5f284fba43de6161145f0333d77c75fbaa4598483922d155`. Current UTC boundary:
`2026-08-27T05:20:47Z`.

Malformed or quarantined receipts must not become cursors; preserve them and quarantine them
through an owner-operated recovery path. This audit did not move, delete, or inspect any
supervisor receipt. The smallest helper-only contract is: accept only an existing top-level
message with one canonical UTC filename, reject future/malformed/nonexistent explicit targets,
ignore such entries for automatic advancement, and advance the selected inbox cursor
monotonically and atomically.

The 05:14 duplicate Furiosa disposition conflict and authoritative session
`01a03fdb-2bb0-70a3-966e-4163be3ab394` resolution are external challenge context; this report
did not read a supervisor cursor or independently adjudicate that conflict.

## Verdict

No missing or deleted referenced mailbox directory required repair. No cursor was changed
outside Beatrix's own mailbox. Khan's mailbox directory exists but lacks a cursor; because it
contains messages, this audit classifies its boundary as uninitialized/UNCERTAIN rather than
non-steerable. The root RawClaw mailbox is shared by
multiple lanes and must not be used as a worker-owned replacement. The three cursor defects
above remain separate control-plane findings; no helper was changed here.
