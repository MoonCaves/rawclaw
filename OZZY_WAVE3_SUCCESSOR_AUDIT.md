# Ozzy Wave 3 successor audit

Audit date: 2026-08-26

Scope: `rawclaw-ozzy-flash-catalog`, `rawclaw-ozzy-flash-cleanup`,
`rawclaw-ozzy-flash-hidden`, `rawclaw-ozzy-flash-hook`,
`rawclaw-ozzy-flash-integration`, `rawclaw-ozzy-flash-ponytail`,
`rawclaw-ozzy-flash-prune`, `rawclaw-ozzy-flash-repro`,
`rawclaw-ozzy-flash-spy`, and `rawclaw-ozzy-prior-art`.

Immutable comparison refs: baseline `0d1da19c4c21`; audited mechanisms
`89c8a284d20e4f6adba72accb3c0b34831a3b422`,
`37ec96bebb2a8317617544836ef9730149e1f0d4`, and
`b944d082e9b8d02611b018a25ce9a049066629fc`.

## Verdict

| current tip | result | evidence |
|---|---|---|
| `89c8a284d20e4f6adba72accb3c0b34831a3b422` (`ozzy/flash-refresh-cleanup`) | **HOLD** | Lock probe is released before deletion; see below. |
| `cdc063d058cc775ec2ee45a4231d8458ad3e9d43` (`catalog`, `hidden`, `prune`, `repro`) | **ACCEPT, narrowed** | Global mixed-source ambiguity is preserved; scoped lookup remains broken. |
| `9010fcca121576dfc47e058fa4127acbb5b4701f` (`hook`) | **NO-NOVELTY** | Report audits old `92d0067`; it is not a successor to path-safe `37ec96b`. |
| `472c489115772df4bc486392da7dcc6d34aef32e` (`integration`) | **HOLD** | Its own report documents unresolved scoped source loss. |
| `47d986f40a96ef9c55af53e51004d8e0342faf9d` (`ponytail`) | **NO-NOVELTY** | Estimates are recommendations; no implementation or measured deletion landed. |
| `37a2012` (`prior-art`) | **HOLD on cleanup claim** | The ledger itself says probe-then-unlink is not a fence. |
| `c0988ee` (`spy`) | **NO-NOVELTY** | Documentation-only wave update; no successor production patch. |

## Confirmed HOLD: refresh cleanup remains probe-to-unlink TOCTOU

`89c8a284...` changes `pruneStaleRefreshDBs` to call `isLockedOrActive` and
then `removeRefreshDB` (`internal/index/containers.go:78-90`).
`isLockedOrActive` opens a separate SQLite handle, executes
`BEGIN IMMEDIATE; ROLLBACK`, closes it via defer, and only then returns
(`internal/index/containers.go:93-108`). The three removes happen afterward
(`internal/index/containers.go:110-114`). A writer can acquire the database
between the probe's rollback/close and the first `os.Remove`; the probe is
therefore advisory and does not fence the deletion. This is the exact failure
the prior-art ledger warns about: `PRIOR_ART_SOURCES.md:106-107` and
`PRIOR_ART_SOURCES.md:164-166` require serialization through unlink and
explicitly reject a `BEGIN IMMEDIATE; ROLLBACK` probe followed by unlink.

The new regression test only holds a transaction during the probe, then
releases it and checks a later sweep (`internal/index/containers_test.go:745-805`).
It cannot acquire a writer in the probe-to-remove gap, so it does not test the
claimed concurrent safety. The 5-worker test exercises distinct databases only
(`internal/index/containers_test.go:807-853`), not deletion interleaving.

Commit payload: production `+46` net lines (`61 added, 15 removed`), tests
`+143`, total `+189`. **HOLD** until one owner fence spans decision through
DB/WAL/SHM removal, with a deterministic interleaving test. `net: -189 lines
possible` if this unproven successor is rejected wholesale (or at minimum
`net: -21 production lines possible` by removing the probe helper and its
comments after a correct fence is supplied).

## Accepted but narrowed: mixed-source catalog fix

`54afa70...` changed foreign catalog handling from reconstructing a Claude
scope to skipping it; `cdc063d...` changed `continue` to `return nil`
(`internal/agentproto/agentproto.go:1809-1818` at `cdc063d`). The added test
proves only an unscoped lookup reaches the source-aware fallback and does not
silently resolve a mixed Claude/Codex prefix (`internal/cli/cmd_tag_onestore_test.go:123-170`).
The focused race gate was observed fresh:

```
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run \
  'TestGuardedSessionLookup(PreservesMixedSourceAmbiguity|UsesForeignPreResolvedScope|DoesNotTreatForeignCatalogPathAsClaude)'
ok   github.com/MoonCaves/rawclaw/internal/cli  1.772s
wall: 8.889s
```

However, the integration report at immutable `472c489...` explicitly records
that a non-nil project scope never calls `more()` and can silently select the
Claude row (`FINDINGS-OZZY-CATALOG.md:44-50`, `65-77`, `83-112`). Therefore
`cdc063d` is **ACCEPT, narrowed to global fallback behavior**, not a complete
source-resolution successor. Its production delta is `+1` line and test
delta `+48`; no duplicate patch-id was found among the named Ozzy tips.

## No successor / unsupported credit

The named Ozzy hook tip `9010fcc...` is a 264-line report whose target is the
older `92d0067` (`FINDINGS-OZZY-HOOK.md:1-8`). It does not contain the audited
path-safe successor `37ec96b...`; no current named Ozzy tip is a descendant of
that commit. The path-safe implementation itself remains an external audited
ref, not Ozzy Wave 3 output.

The ponytail tip `47d986f...` claims approximately 520 removable production
lines and 480 test lines, but lists only estimated opportunities and no code
changes (`FINDINGS-OZZY-PONYTAIL.md:1-20`, `133-150`). The current Ozzy
benchmark file still contains the repeated benchmark bodies, including the
baseline/mmap/query-only/full-tuned variants (`internal/store/connect_bench_test.go:129-227`);
no benchmark deduplication successor landed. The later 8-line benchmark
cleanup `61b79574...` is on Norm's branch, not a named Ozzy worker, and has a
distinct patch ID from the original table-drive `e19b80e...`; it cannot be
credited to Ozzy.

The prune worktree is dirty with an uncommitted 29-line benchmark addition to
`internal/index/consolidated_test.go`; it was not inspected as a commit or
credited (`git status` observed `M internal/index/consolidated_test.go`).
No files were changed in any rival worktree.

## Patch identity and gate accounting

- `37ec96b`, `b944d08`, and `89c8a28` retain distinct stable patch IDs; no
  byte-identical successor among the named Ozzy tips was found.
- Shared tip `cdc063d` is one commit lineage referenced by four named worktrees,
  not four independent implementations.
- `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run
  'TestEnsureFreshContainer_(PruneStaleLeftovers|ConcurrentIsolation)'` on
  `89c8a28` passed in `2.368s`; this is package-focused evidence only and does
  not clear the TOCTOU HOLD.
- No full repository gate was run in this report-only audit. `gofmt` was not
  required because only this Markdown report was edited.

Final accounting: one cleanup successor **HOLD**, one catalog change
**ACCEPT (global-only)**, remaining current Ozzy tips **NO-NOVELTY or
unsupported for successor credit**. `net: -189 lines possible`.
