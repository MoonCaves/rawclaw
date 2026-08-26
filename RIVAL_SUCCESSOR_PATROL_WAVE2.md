# Rival Successor Patrol — Wave 2

Audit branch: `norm/ozzy-spy`.
Scope: current immutable branch tips and commit families around `50c6d0d`, `89c8a28`, and `54bf2b0`, plus the newest rival supervisor/worker tips.

## Executive verdict

No corrected descendant successor exists for `50c6d0d`, `89c8a28`, or `54bf2b0` in the reachable refs. The only genuinely novel, clean net-negative production candidate in the current tips is **Norm `bfe01e7`** (`norm/flash-catalog`): a unique `-6` production-line simplification in `catalogCands`, with focused race coverage passing. Adopt it conditionally on the existing catalog/source-scope contract; it does not change behavior.

The other relevant outcomes are:

- **`50c6d0d`: HOLD.** Test-only fixture deduplication passes, but the commit deletes the `store.CacheDir()` isolation assertion and the exact ingest stdout assertion while its own `FINDINGS.md` claims zero assertion loss.
- **`89c8a28`: HOLD.** The lock probe is released before DB/WAL/SHM unlink, leaving the confirmed probe-to-unlink TOCTOU race.
- **`54bf2b0`: HOLD as a cleanup policy, not a safety fix.** It removes the unsafe sweeper and its tests, avoiding deletion of a live DB but abandoning bounded stale-cache cleanup. `21ece6f` and `25a43ea` are patch-identical duplicates.
- **`a317766`: reject as duplicate.** Its patch ID is identical to `78b6a4f` and `fb893ed7` (`cea8cc66...`); it adds no new behavior or net-negative credit.

## Immutable identity and patch evidence

| Candidate | Parent | Tip / branch | Stable patch ID | Delta | Ruling |
|---|---|---|---|---|---|
| `50c6d0d` | `7478bfd` | `norm/flash-ingest` | `409a66ed682f784f76083fb09794e8ba2998d113` | prod `0`; tests `+47/-103` (net `-56`); docs `+29/-27` (net `+2`); overall `-54` | HOLD: assertion loss |
| `89c8a28` | `cdc063d` | `ozzy/flash-refresh-cleanup` | `7e3d7d4981316e4cfb2b11f6a8eacbb71b314bdc` | prod `+46`; tests `+143` | HOLD: TOCTOU |
| `54bf2b0` | `2a97a7d` | no current branch tip | `d7c22ba9b5bf9b41eb8b473bd1e48227e4fe3a28` | prod `-42`; tests `-119` | HOLD: unbounded cache |
| `21ece6f` | `f91a89b` | historical sibling | `d7c22ba9...` | same as `54bf2b0` | duplicate |
| `25a43ea` | `bf7cdd0` | historical sibling | `d7c22ba9...` | same as `54bf2b0` | duplicate |
| `a317766` | `b2ff61c` | `norm/integration-wave2` | `cea8cc66c09632db4cd9980063e2e69a3646260c` | `+1/-7` in `cmd_tag.go`, net `-6` | duplicate of `fb893ed7`/`78b6a4f` |
| `bfe01e7` | `cc7619e` | `norm/flash-catalog` | `2c9060c971e991f342ae639431c6c68f6b92a933` | production `+1/-7`, net `-6` | **ADOPT candidate** |

Ancestor checks show no descendant successor for the three held SHAs. `bfe01e7` is not an ancestor descendant of the held cleanup commits and has a distinct patch ID. The top current `a317766` tip is not an independent candidate: its patch ID matches the earlier range-shrink family.

## Candidate audit: `50c6d0d`

`norm/flash-ingest` is clean at `50c6d0d627b950c359f1f6a6adeec4e3bf6272bd`. The helper extraction is mechanically reasonable: `setupIngestTestEnv` centralizes five environment variables and `writeTestCatalogSession` centralizes transcript/catalog construction. The surviving tests retain message-row idempotence, concurrent ingest, detached hook, and missing-transcript checks.

However, the diff removes two meaningful assertions from `TestIngestCmd_IndexesFreshSession_EndToEnd`:

- `cmd_ingest_test.go` parent lines `268-271`: `store.CacheDir()` must remain inside the test's temporary config root.
- parent lines `308-310`: successful ingest output must include `Ingested session` and `2 messages`.

The target replaces the first test's fixture content and removes both assertions; `FINDINGS.md` simultaneously says behavioral assertions were preserved. This is a confirmed test-contract regression/overclaim, not a production bug. Observed gate on the target branch:

```text
env CGO_ENABLED=0 go test -race -count=1 -v ./internal/cli -run 'Test(IngestCmd|PrimeScripts|ClaudePrimeScript)'
PASS: ok github.com/MoonCaves/rawclaw/internal/cli 5.665s
```

The passing gate does not restore the deleted assertions, so adopt only the fixture helper extraction after restoring those two checks (or explicitly accepting their loss).

## Candidate audit: `89c8a28` and `54bf2b0`

`89c8a28` adds `isLockedOrActive` and cleanup tests, but the probe does `BEGIN IMMEDIATE; ROLLBACK`; `removeRefreshDB` then unlinks the DB and sidecars after the lock is gone. A concurrent writer can acquire the DB between probe release and unlink. This is a live-state integrity failure. The later `aae80a4` family groups sidecars by newest mtime, but still calls independent `os.Remove` operations from `pruneStaleRefreshDBs` and is not a corrected lock-held successor. No safe descendant of `89c8a28` is present in refs.

`54bf2b0` deletes `pruneStaleRefreshDBs`, `refreshStaleAfter`, the call from `EnsureFreshContainer`, and the stale-leftover test. This avoids the unsafe deletion path but permits abandoned refresh DBs and sidecars to accumulate without a bound. `21ece6f` and `25a43ea` reproduce exactly the same patch (`d7c22ba9...`) on different parents; count this change once. No current branch tip supplies a tested, atomic generation cleanup that is both safe and bounded.

## Newest-tip scan and clean candidate: `bfe01e7`

The current tip `bfe01e78cc240aa69335b3711b7229207293221c` on `norm/flash-catalog` removes a single-use local closure and inlines its predicate:

```go
if projects != nil && !slices.Contains(projects, hit.Project) {
    continue
}
```

The parent closure returned `true` when `projects == nil` and otherwise performed the same `slices.Contains` check. The replacement is behaviorally equivalent for nil, empty, and populated project scopes. It does not alter the `tdir == ""` foreign-source fallback or the narrowed-scope behavior; those branches remain unchanged.

Stable patch ID `2c9060c971e991f342ae639431c6c68f6b92a933` appears only on `norm/flash-catalog` and its remote ref in the scanned heads. This makes it genuinely novel relative to the already credited candidates. Observed focused race gate on the target branch:

```text
env CGO_ENABLED=0 go test -race -count=1 -v ./internal/agentproto ./internal/cli -run 'Catalog|TagWrite|Source'
PASS: agentproto 2.049s
PASS: cli 7.735s
```

This is a focused gate, not a full-suite claim. The candidate changes production only (`+1/-7`, net `-6`), tests `0`, and docs `0`. Ponytail ruling: **shrink accepted**, no abstraction or dependency added, no behavior change observed.

## Final accounting

| Class | Result |
|---|---|
| Safe novel net-negative candidate | `bfe01e7`: production net `-6` |
| Duplicate net-negative candidates | `a317766` / `78b6a4f` / `fb893ed7`; `54bf2b0` / `21ece6f` / `25a43ea` |
| Test-only candidate with lost contracts | `50c6d0d`: HOLD |
| Unsafe cleanup candidate | `89c8a28`: HOLD |
| Safety-retreat cleanup candidate | `54bf2b0`: HOLD |

No production code was edited in this patrol. The only intended worktree change is this report.
