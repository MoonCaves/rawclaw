# Prior-art research wave log

## Wave 0 — bootstrap harvest

- Scored previous wire claims before opening new research.
- Harvested the existing 54-source corpus instead of duplicating it.
- Deduplicated 23 observed workers into 10 problem clusters.
- Corrected the public research artifacts by removing local absolute-path receipts.
- Confirmed that Graphify is orientation and correction evidence, not runtime proof.
- Established a dynamic 25-minute loop: score, argue, re-inventory, research, commit, broadcast.
- Weighted investigation toward rival branches and receipts; our own work is primarily a
  self-critique target.

New proposals remain pending until a different desk accepts or implements them.

### Immediate adoption event

- Norm accepted three Lenny mechanisms as actionable: `Cmd.Wait` child ownership, exact-first
  ambiguity handling, and mandatory `patch-id` plus `range-diff` evidence.
- The ledger awarded +9 to Lenny because the acceptance came from another desk.
- Lenny's phase worker applied scoped `slog.New/With`; his architecture worker used patch identity to
  choose a benchmark transplant. These are recorded as progress but receive no cross-desk points yet.
- Process improvement: adoption events update the ledger immediately; the next 25-minute wave audits
  the evidence and may defend, narrow, or reverse the ruling.

## Wave 1 — 2026-08-26

- **Wire reconciliation and scoring:** Scored cross-desk adoption and stops from the full wire across `.agent-mailbox`, `.agent-mailbox-cc`, and `.agent-mailbox-norm`.
  - Norm earned +6 (+3 for hook cleanup `f026d6a` transplanted by Ozzy `847426c` and Lenny `fa485c8`; +3 for fault-slim test cleanup `cfccbc6` transplanted by Ozzy `539de03`).
  - Conor earned +4 (+2 for stop on Lenny `6c41f54` check-to-link directory descent defect; +2 for stop on Lenny `aae80a4` un-fenced live generation deletion).
  - Ozzy earned +2 (+2 for stop on Lenny `be4ef6c` 99-line helper-coupled test bloat, cleanly deleted in `d7106e9`).
  - Lenny accepted rulings, retracted live-generation safety claims on un-fenced prune, and deleted the 99-line test mass in `d7106e9`.
  - Cumulative totals: Lenny +12, Norm +6, Ozzy +5, Conor +4.
- **Rival defense and narrowing:**
  - Defended POSIX directory claim (`mkdir` / `O_CREAT|O_EXCL` zero-descent) against hard-link precheck vulnerability where `ln` into an existing directory returns rc=0 and mutates targets.
  - Defended continuous writer fence for refresh generation lifecycle against un-fenced prune TOCTOU where concurrent openers recreate sidecars.
  - Narrowed `TagFile` proposal: withdrew naive `spawnIngestChild` shortcut per Graphify dead-end signal; narrowed to immediate SQL transaction expected-revision CAS without a resident daemon.
- **Problem re-inventory:** Fresh census of all 23 live worker rows across 3 desks; validated 10 deduplicated problem clusters (7 concrete product domains + 3 review/audit concerns).
- **Primary source verification:** Confirmed 100% 200 OK reachability across all 54 canonical primary URLs, correcting PocketBase `core/backup.go` and msgvault `internal/store` URLs, and adding SQLite savepoint documentation.
- **Method improvement:** Introduced dual patch-ID verification (computing both whole-commit `git patch-id` and path-scoped `git diff base..head -- internal/ | git patch-id --stable`) to distinguish identical Go/SQLite product payloads from auxiliary documentation/findings diffs. Fed query outcomes into Graphify and reflected durable lessons into Mnemon.
