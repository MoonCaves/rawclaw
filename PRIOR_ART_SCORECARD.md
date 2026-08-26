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
| Lenny | `Cmd.Wait` child ownership mechanism | explicitly accepted by Norm as actionable | +3 | mechanism report `c4d67bd`; public ruling 2026-08-26 10:44:39Z |
| Lenny | exact-first `(source, fullID)` ambiguity handling | explicitly accepted by Norm as actionable | +3 | mechanism report `c4d67bd`; public ruling 2026-08-26 10:44:39Z |
| Lenny | mandatory `patch-id` plus `range-diff` novelty evidence | explicitly accepted by Norm as actionable | +3 | mechanism report `dd655e7`; public ruling 2026-08-26 10:44:39Z |

Current totals after bootstrap: **Lenny +12, Norm 0, Conor 0, Ozzy +3**.

Own-desk implementation does not count as cross-desk adoption: Lenny's phase worker reported applying
the scoped `slog.New/With` mechanism with a race-count-10 pass, and the architecture worker reported
using patch identity to select a benchmark transplant. These remain implementation progress until a
different desk accepts or reuses them.

## Wave 1 — 2026-08-26

| desk | item | ruling | points | immutable evidence |
|---|---|---|---:|---|
| Norm | hook cleanup and error plumbing fix | accepted and transplanted by Ozzy (`847426c`) and Lenny (`fa485c8`) | +3 | `f026d6a`, patch ID `e6322da4ca5faaa5b3b596fdbb33409bf376a4e5` |
| Norm | fault-slim informational test cleanup | accepted and transplanted by Ozzy (`539de03`) | +3 | `cfccbc6`, patch ID `7addd4ca88dd31164e993883d4b57a4852e8e5b8` |
| Conor | stop on `6c41f54` check-to-link directory descent defect | accepted: `ln` into existing directory succeeds with rc=0 and creates nested link | +2 | `6c41f54`; direct probe audit receipt 2026-08-26 11:06:21Z; Lenny acknowledgment `022d07e3` |
| Conor | stop on `aae80a4` un-fenced live generation deletion | accepted: un-fenced snapshot-then-unlink TOCTOU; plain file test omitted open connection race | +2 | `aae80a4`; audit receipt 2026-08-26 11:04:11Z; Lenny acknowledgment `022d07e3` |
| Ozzy | stop on `be4ef6c` 99-line helper-coupled test bloat | accepted: unexported `containerMeta` mass deleted in `d7106e9` | +2 | `be4ef6c`; rejection ruling 2026-08-26 11:04:31Z; Lenny deletion commit `d7106e9` |
| Lenny | shared topic-segment range resolution | accepted and implemented by Ozzy as a fenced one-file salvage | +3 | source `fc1a075`; implementation `b944d08`; pushed harvest head `b944d082e9b8d02611b018a25ce9a049066629fc` |

Current cumulative totals: **Lenny +15, Norm +6, Ozzy +5, Conor +4**.

### Defense, narrowing, and withdrawal rulings in Wave 1

- **Defended:** POSIX claim directory (`mkdir` atomic `EEXIST` / `O_CREAT|O_EXCL` with `O_NOFOLLOW`). Conor proved that hardlink prechecks (`[ -e "$entry" ]` then `ln`) fail when a directory is created in the check-to-link gap. POSIX `mkdir` / `O_EXCL` is defended as the only mechanism with zero directory descent.
- **Defended:** Continuous writer fence for refresh generation lifecycle. Conor and Ozzy proved that grouped mtime deletion without holding the writer fence across stat, close, and unlink allows concurrent openers to recreate sidecars.
- **Narrowed / Withdrawn:** `TagFile` publisher shortcut via `spawnIngestChild` is withdrawn per Graphify dead-end signal. Tag publication is narrowed to strict SQL immediate transaction revision check (CAS style) without a resident daemon.

## Pending proposals — zero points

| proposal | status | next acceptance test |
|---|---|---|
| POSIX claim directory with separate metadata publication | defended / pending | a rival desk accepts it and proves regular/FIFO/directory/symlink/socket behavior plus exactly-once ingest |
| continuous writer fence held through decision, checkpoint, close, and removal | defended / pending | a rival desk lands an implementation holding the serialization fence across the entire generation lifecycle |
| CAS-style expected-revision SQL tag publication | narrowed / pending | a rival desk implements expected-revision validation at the immediate SQL transaction boundary |

## Method feedback loop

1. Read the full wire and grade the preceding wave.
2. Defend, narrow, withdraw, or mark adoption before new research.
3. Rebuild the live worker/function census; never reuse stale branch state.
4. Search the existing source corpus before adding canonical primary sources.
5. Record Graphify outcomes as useful, dead end, or corrected; remember durable lessons in Mnemon.
6. Publish technical wins and method improvements together. Never publish self-awarded adoption.
7. Compute dual patch IDs (whole-commit and path-scoped) to distinguish identical product code from documentation differences.
