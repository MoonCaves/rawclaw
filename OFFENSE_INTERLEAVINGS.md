# Offense interleavings audit

Bounded, report-only audit of rival heads. No product or test files were
modified. All source claims below are pinned to immutable commit SHAs.

## Verdict

| Target | Classification | Finding |
| --- | --- | --- |
| Ozzy `89c8a284d20e4f6adba72accb3c0b34831a3b422` | **CONFIRMED** | The refresh sweeper performs a short SQLite lock probe, releases it, then unlinks the database and sidecars. This is a probe-to-unlink TOCTOU race, not a writer fence. The “atomic group” comment is also not implemented by three independent ignored-error `os.Remove` calls. |
| Norm `2cc11d683761b702f26d1127efeb631a70ef348b` | **CONFIRMED** | `ln "$tmp_entry" "$entry"` treats an existing directory as a destination directory, returns success, and lets the hook launch ingest while leaving a nested temporary link. The special-path test only checks hook exit/non-blocking and does not inspect ingest calls or target mutation. |
| Norm `6ddd17a373114f8ca643cabe26014370e9e432a9` | **NO-DEDUCTION** | Test-only changes. They strengthen phase start-before-duration assertions and fence-timeout logging; no production interleaving or incorrect synchronization was introduced by this SHA. |
| Conor `6d20bda91501aeb341c46181556137d273d77a38` | **NARROWED / NO-DEDUCTION** | The current hook repair uses a private temporary directory and links into the catalog directory, then checks both `-e` and `-L` on an existing target. That closes the Norm directory/symlink target-class trap in the inspected paths. It still has an explicit fail-soft branch that launches ingest when the catalog cannot be claimed; that is an availability tradeoff, not a dedup guarantee, and this audit found no separate race defect in the repaired path. |

## 1. Ozzy refresh cleanup: released probe before deletion

Target: `89c8a284d20e4f6adba72accb3c0b34831a3b422`.

At `internal/index/containers.go:78-90`, the sweeper computes staleness,
calls `isLockedOrActive(dbPath)`, and then calls `removeRefreshDB(dbPath)`.
`isLockedOrActive` at `internal/index/containers.go:93-107` opens a separate
SQLite handle, executes `BEGIN IMMEDIATE; ROLLBACK` at lines 99-103, and closes
the handle on return. The lock therefore does not cover the later deletion.
A refresh writer can start after line 107 and before line 89. The source proves
the interleaving is possible; the existing tests do not force it.

The candidate's claimed atomic sidecar grouping is also narrower than stated:
`internal/index/containers.go:57-58` says the database and WAL/SHM are removed
as one atomic group, but `removeRefreshDB` at lines 110-113 performs three
independent ignored-error `os.Remove` calls. A crash, permission error, or
concurrent creator can leave a split group. This is a confirmed source-level
durability gap; no crash experiment was claimed.

The active-writer regression at
`internal/index/containers_test.go:710-805` holds the SQLite transaction before
the probe and releases it only after the sweep. It does not cover the
probe-to-unlink window. The five-worker test at lines 807-851 uses distinct
fresh session databases and does not race a writer against stale cleanup.

Classification: **CONFIRMED source-level TOCTOU and non-atomic deletion**;
empirical reproduction of the exact scheduling window is **UNCERTAIN**, not a
green test claim.

## 2. Norm hook claim: existing directory is a false successful claim

Target: `2cc11d683761b702f26d1127efeb631a70ef348b`.

The Claude hook at `internal/cli/setup.go:88-106` writes a temporary entry and
uses `ln "$tmp_entry" "$entry"` at line 96. The same shape appears in the
Codex template later in the file. If `$entry` already names a directory,
POSIX `ln` interprets it as a destination directory, creates a link below it,
and returns status 0. The branch at lines 96-98 then removes the temporary
path and launches detached ingest, even though no catalog claim for the
session was made.

Independent reproduction (outside the repository) used:

```text
d=$(mktemp -d); mkdir "$d/catalog" "$d/catalog/existing"; printf x > "$d/tmp"; ln "$d/tmp" "$d/catalog/existing"; rc=$?; find "$d/catalog" -maxdepth 3 -print | sort; printf 'ln_exit=%s\n' "$rc"
```

Observed result included `catalog/existing/tmp` and `ln_exit=0`.

The candidate regression test at
`internal/cli/catalog_hook_test.go:419-489` creates FIFO, directory, symlink,
and socket targets, but at lines 480-484 asserts only that the hook exits and
does not time out. It does not assert zero detached-ingest calls, target
immutability, or absence of nested temporary artifacts. Therefore a green
result from that test would not establish the claimed dedup behavior.

Classification: **CONFIRMED behavior defect** and **CONFIRMED test gap**.

## 3. Norm fence/phase test hardening

Target: `6ddd17a373114f8ca643cabe26014370e9e432a9`.

This SHA changes only
`internal/index/consolidated_fence_test.go` and
`internal/index/consolidated_test.go`. The timeout test at lines
`consolidated_fence_test.go:84-155` checks that an acquire `event=start` log
precedes a duration log and that the duration is at least the timeout. The
phase assertion at `consolidated_test.go:42-94` similarly records first start
and duration indexes and requires start-before-duration. These are useful
observability assertions, but they do not claim to fence new production
writers and do not alter the lock implementation.

Classification: **NO-DEDUCTION** for this bounded interleaving audit.

## 4. Current Conor hook repair

Target: `6d20bda91501aeb341c46181556137d273d77a38`.

The repaired Claude path at `internal/cli/setup.go:72-97` creates a unique
temporary directory, writes the candidate entry, and runs
`ln "$tmp_entry" "$catalog_dir"` at line 84. Because the destination is the
catalog directory, an existing `catalog/<session>` directory or symlink cannot
be treated as `ln`'s destination folder. The branch at lines 90-96 launches
ingest only after a successful link, or in the explicit fail-soft case where
the catalog cannot be claimed; existing regular paths and symlinks are checked
with both `-e` and `-L` at line 92. The Codex template carries the same shape.

This repairs the specific Norm directory false-success interleaving inspected
above. The fail-soft fallback intentionally favors ingest availability when
catalog persistence fails, so it is not evidence of exactly-once ingest under
filesystem failure. No additional race defect was established here.

Classification: **NARROWED / NO-DEDUCTION** for the repaired target-class
interleaving; availability-vs-dedup remains an explicit tradeoff.

## Observed commands and gates

Graph orientation:

```text
graphify reflect --if-stale
graphify query "consolidate lock fence claim publish snapshot swap" --graph /Users/jay-m4/code/rawclaw/graphify-out/graph.json --budget 4000
graphify explain "ConsolidateFrom" --graph /Users/jay-m4/code/rawclaw/graphify-out/graph.json
graphify path "Ingest" "ConsolidateFrom" --graph /Users/jay-m4/code/rawclaw/graphify-out/graph.json --budget 4000
```

The worktree had no local `graphify-out/graph.json`; the canonical main
checkout graph was used after the local `reflect --if-stale` pass. The path
query reported no direct `Ingest` → `ConsolidateFrom` path; `explain` located
`ConsolidateFrom` at `internal/index/consolidated.go:402` and its fence and
refresh relationships.

Memory orientation:

```text
mnemon --store rawclaw recall rawclaw --limit 10
```

Focused current-tree gates (all passed; these do not close the candidate
TOCTOU or Norm false-green gaps):

```text
CGO_ENABLED=0 go test -race -shuffle=on -count=3 -run '^(TestEnsureFreshContainer_PruneStaleLeftovers|TestEnsureFreshContainer_PruneStaleLeftovers_ActiveWriterFenced|TestEnsureFreshContainer_ConcurrentIsolation|TestEnsureFreshContainer_PreservesRefreshDBOnPublishFailure)$' ./internal/index
ok github.com/MoonCaves/rawclaw/internal/index 4.396s

CGO_ENABLED=0 go test -race -shuffle=on -count=5 -run '^(TestConsolidatedFence|TestConsolidate_LogsPhaseStartsAndDurations)' ./internal/index
ok github.com/MoonCaves/rawclaw/internal/index 4.693s

CGO_ENABLED=0 go test -race -shuffle=on -count=3 -run '^TestPrimeScripts_ExistingSpecialCatalogPathDoesNotBlock$' ./internal/cli
ok github.com/MoonCaves/rawclaw/internal/cli 1.888s [no tests to run]
```

The last command is explicitly recorded as `[no tests to run]`, not as
evidence for the Norm candidate.
