# Tick 30 WAL prior-art dedupe

Verdict: **DUPLICATE**. Canonical recommendation is
`PA-SQLITE-WAL-PASSIVE-CHECKPOINT-001` from
`worker/furiosa-t29-sqlite-research-20260827@08ede1c`. The broader
`PA-SQLITE-WAL-IDLE-CHECKPOINT-001` wording from `3c31ccb` describes the same
effect and must not receive a second score.

## Exact overlap and source strength

Both recommendations prescribe moving SQLite WAL checkpoint work away from
foreground closeout, using the automatic threshold/application scheduling and
an explicit PASSIVE checkpoint that can make bounded progress without waiting
for readers or writers. The alias is therefore wording, not a second
mechanism. The canonical report is stronger: it cites the two exact SQLite
pragma references (`wal_autocheckpoint` and `wal_checkpoint(PASSIVE)`) and a
modernc/sqlite cancellation test showing checkpoint usability after canceled
work. Retain that evidence and canonical ID; narrow the broader report into
the canonical recommendation.

Fingerprint evidence:

```text
git show --stat --oneline 08ede1c
08ede1c research: add sqlite passive checkpoint prior art
 PRIOR_ART_FINDINGS.md | 38 ++++++++++++++++++++++++++++++++++++++
 1 file changed, 38 insertions(+)

git diff 0d1da19 08ede1c | git patch-id --stable
57b5b6f73c63cdd3c02cba65aea520f29c1dc86e 0000000000000000000000000000000000000000

git diff --stat 0d1da19 3c31ccb
 PRIOR_ART_FINDINGS.md | 116 insertions(+)
 1 file changed, 116 insertions(+)

git diff 0d1da19 3c31ccb -- PRIOR_ART_FINDINGS.md | git patch-id --stable
4229d88aeda7605dfa7442dacef59d77f48ba373 0000000000000000000000000000000000000000
```

The two normalized fingerprints are different only because the prose is
different: `86a2faf69f9e11c899eaa9e1c13672f8edb997900905416de6cb483e4b3fd2e8`
versus
`efe829449d40df34663d26fa33920d98f25d65fac768b66bc2e1218d8c201e91`.
They do not establish distinct mechanisms.

## RawClaw WAL reality

Production `store.ConnectRW` opens SQLite with
`_pragma=journal_mode(WAL)&_pragma=busy_timeout(10000)` and caps the pool at
one connection (`internal/store/store.go:331-345`). The relevant closeout path
`EnsureFreshContainer` calls `SyncConsolidatedFrom`, which opens the
consolidated store through `ConnectRW` (`internal/index/containers.go:88-104`,
`internal/index/consolidated.go:553-563`). Thus RawClaw does use WAL in this
path.

Production code does **not** explicitly set `wal_autocheckpoint`; therefore
SQLite's documented default automatic checkpoint threshold remains applicable
unless changed by the driver/runtime. Explicit `wal_checkpoint(TRUNCATE)`
exists in `internal/index/rebuild.go:132` and test fixtures, but no production
`wal_checkpoint(PASSIVE)` scheduling mechanism exists in the closeout path.
The recommendation is consequently relevant but unadopted.

## Score and status ruling

- Canonical ID: `PA-SQLITE-WAL-PASSIVE-CHECKPOINT-001`.
- Alias ID: `PA-SQLITE-WAL-IDLE-CHECKPOINT-001`.
- Ruling: `DUPLICATE`; merge alias wording into the canonical entry.
- Stronger evidence retained: the canonical report's pragma-specific SQLite
  sources plus modernc cancellation test.
- Adoption evidence: none; both remain pending and score 0.
- No Direction Lock or merge authorization follows.
- No focused gate or mutation was needed: this is a source/identity dedupe,
  not an implementation claim.

The unrelated semaphore recommendation in `3c31ccb` is not part of this
dedupe.

