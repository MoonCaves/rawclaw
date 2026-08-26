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

The state also persists the cadence/configuration, session identity, and current slot. The lock
token contains PID, process start identity, and creation time; an unavailable process identity uses
a bounded two-cadence age check.

`start` runs the auditor loop; it was not activated while creating this prototype. The loop checks
STOP after every audit and exits before another tick. The third consecutive late audit writes the
local STOP sentinel. A same-slot rerun is idempotent: it emits an idempotent receipt without sending
or incrementing misses. Misses escalate as warn, critical, then stop-and-escalate. The expected Ozzy
session is fixed to `01a03ca0-d617-7c90-bfa4-6dc2d0316f7e`; mismatched state or configuration is
rejected. `PRESSURE_SEND_FAIL=1` exercises the checked local-mailbox failure path; failures are
recorded without advancing state. Rival mailbox roots can be supplied with
`RIVAL_MAILBOXES=path-a:path-b`; they are inspected read-only and never have cursors advanced.

Fixtures use temporary roots and do not alter this checkout:

```sh
./pressure-harness/fixtures/run-fixtures.sh
```
