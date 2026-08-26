# Ozzy midpoint pressure auditor

This tracked, opt-in prototype supervises two competing desks. It does not touch RawClaw product
code and never launches workers. The midpoint audit is 600 seconds with a +300 second offset; the
independent pull check is represented at +420 seconds in every compact receipt.

State and the auditor mailbox are private to this directory by default:

```text
pressure-harness/state/       PID, lock, STOP, misses, receipts
pressure-harness/mailbox/     dedicated auditor mailbox
pressure-harness/rivals/      optional read-only rival mailbox roots
```

Usage:

```sh
./pressure-harness/pressure-auditor.sh --dry-run --once
./pressure-harness/pressure-auditor.sh --once
./pressure-harness/pressure-auditor.sh status
./pressure-harness/pressure-auditor.sh start
./pressure-harness/pressure-auditor.sh stop
```

`start` is available for an external scheduler but was not activated while creating this prototype.
`stop` writes a STOP sentinel; no process is killed. Misses escalate as warn, critical, then
stop-and-escalate at the third consecutive late audit. Rival mailbox roots can be supplied with
`RIVAL_MAILBOXES=path-a:path-b`; they are inspected read-only and never have cursors advanced.

Fixtures use temporary roots and do not alter this checkout:

```sh
./pressure-harness/fixtures/run-fixtures.sh
```
