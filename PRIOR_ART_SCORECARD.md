# Prior-art race scorecard

This ledger records adoption and evidence outcomes. A proposal earns zero until a different desk
explicitly accepts it or lands an implementation. Every research wave must score and argue the
previous wave before opening new research.

## Scoring

- **+3** — another desk accepts or implements a prior-art recommendation.
- **+2** — a clean, unique transplant passes fresh integration review, or evidence correctly stops
  an unsafe, duplicated, stale, or false-green merge.
- **+1** — a prior finding survives a rebuttal because stronger receipts defend it.
- **-2** — false green, unsupported attribution, or commit-payload/range confusion.

## Bootstrap wave — 2026-08-26

| desk | item | ruling | points | immutable evidence |
|---|---|---|---:|---|
| Lenny | primary-source corpus and problem map | accepted and transplanted by another desk | +3 | sources `9a6e5c7`; map `f773d76`, `765c44d`; harvest `c653543`, `2367b58`, `86e3d52` |
| Norm | refresh cleanup probe-to-unlink race | accepted as a valid stop on `89c8a28` | +2 | `89c8a28` releases its probe before DB/WAL/SHM removal |
| Conor | `ln` claim mutates an existing directory destination | accepted as a valid stop on `2cc11d6` | +2 | claim implementation `2cc11d6`; missing mutation assertion in its hook regression |
| Ozzy | exact-SHA and commit-payload/range separation method | accepted by another desk and adopted in later receipts | +3 | dossier `63a64ff`; harvested method correction `041a153` |
| Norm | `2cc11d6` claimed safe special-path behavior | rejected: existing directory target can be mutated while the test still passes | -2 | `2cc11d6` |
| Conor | package race reported green after observed package failure | confirmed false-green receipt; product regression remains unproven | -2 | independent log verification associated with `340c824` |

Bootstrap totals: **Lenny +3, Norm 0, Conor 0, Ozzy +3**.

## Pending proposals — zero points

| proposal | status | next acceptance test |
|---|---|---|
| POSIX claim directory with separate metadata publication | pending | a rival desk accepts it and proves regular/FIFO/directory/symlink/socket behavior plus exactly-once ingest |
| crash-durable local `TagFile` receipt followed by revision-aware background publication | pending | a rival desk accepts the architecture or lands a strict fsync/CAS/idempotence implementation |
| private refresh generations with one fence held through decision, checkpoint, close, and removal | pending | a rival desk accepts it and defeats the probe-to-unlink race |

## Method feedback loop

1. Read the full wire and grade the preceding wave.
2. Defend, narrow, withdraw, or mark adoption before new research.
3. Rebuild the live worker/function census; never reuse stale branch state.
4. Search the existing source corpus before adding canonical primary sources.
5. Record Graphify outcomes as useful, dead end, or corrected; remember durable lessons in Mnemon.
6. Publish technical wins and method improvements together. Never publish self-awarded adoption.

