# RawClaw architectural and philosophical audit

Date: 2026-09-03 (WITA, UTC+8)
Auditor: Codex Luna Medium
Base: `d2b1723` (`main`, `origin/main`, and `origin/master`)

## Executive verdict

The four initiatives fit RawClaw's north star: one static, keyword-first binary;
durable local truth; explicit source adapters; and an agent-readable CLI. The CLI
split is mechanically sound. Worktree identity and source registration are useful
but less general than their names imply. Tombstone relocation has a real
mixed-version compatibility defect and no atomic delete/tombstone boundary.

| Decision | Verdict | Reason |
|---|---|---|
| Git worktree identity and cross-worktree scoping | **CAUTION** | Correct common case, but narrow queries eagerly build the full scope universe; malformed Git metadata is trusted too much. |
| Tombstone relocation and durability | **CAUTION** | Correct durable location and flat I/O, but legacy fallback loses old IDs when any new ID exists; delete and tombstone are non-atomic. |
| Monolithic CLI decomposition | **APPROVED WITH NOTES** | Effectively mechanical relocation; full race suite passes; compatibility lacks a permanent golden surface. |
| Unified source registration and discovery | **CAUTION** | Central registry helps, but `scopes` retains built-in special cases and registration callbacks are unchecked. |
| Unrelated bundle validation change in `d2b1723` | **REJECT** | `git bundle list-heads` is not equivalent to `git bundle verify`; it weakens an integrity gate. |

## Evidence and method

Reviewed `3196d3e`, `96df592`, `b4724cd`, `466d210`, `d2b1723`, and the
intervening shipped merge `ca4e147`. The requested ponytail audit lens was
applied: new layers were checked for smaller existing mechanisms,
single-implementation abstractions, duplicated source knowledge, and speculative
flexibility. No fixes were applied.

Receipts from this worktree:

```
git status --short --branch
## audit/luna-philosophical-decisions

CGO_ENABLED=0 go test -race -count=1 ./...
PASS: all packages, including archive, cli, index, lifecycle, paths, scopes,
all source adapters, sources, and store.

gofmt -l internal/
PASS: no output.
```

## 1. Git worktree identity and cross-worktree scoping

### Design

`GitRoot` aliases `GitCommonRoot` at `internal/paths/paths.go:93-98`.
`GitCommonRoot` walks upward, recognizes a normal `.git` directory, and
follows a worktree `.git` file through `gitdir` and optional `commondir` at
`internal/paths/paths.go:232-277`. `FindTranscriptDir` preserves recorded-CWD
and encoded-path resolution, then tries the common root at
`internal/paths/paths.go:201-225`.

CLI scope resolution uses it in `thisScope`
(`internal/cli/cli_search.go:27-43`) and `verbScope`
(`internal/cli/cli.go:400-418`). `FilterByProjectDir` compares canonical Git
roots and falls back to exact canonical CWD equality, deduplicating by
project/TDir/DBP at `internal/scopes/scopes.go:320-353`. Consolidated-store
filters receive all matching labels through `SearchOpts.Projects`, handled at
`internal/agentproto/topics.go:170-220` and
`internal/agentproto/search.go:408-437`.

The identity model is right: a worktree is a checkout of one repository, not a
new semantic project. The tests cover regular repositories, worktree pointers
with `commondir`, subdirectories, non-Git directories, and sibling-worktree
discovery (`internal/paths/paths_test.go` additions in `3196d3e` and
`d2b1723`; `internal/cli/cmd_worktree_scoping_test.go:11-90`). Detached HEAD
does not affect this filesystem-level calculation.

### Material concern: the narrow path is not narrow

The worktree branch calls `allScope(ctx, o.Source, o.Reindex)` before filtering
at `internal/cli/cli_search.go:29-33` and
`internal/cli/cli.go:405-410`. `scopes.All` loops through every registered
source and calls `containerScopes`
(`internal/scopes/scopes.go:40-69`); `containerScopes` discovers and may
index every live CWD via `index.EnsureIndexedContainers`
(`internal/scopes/container.go:23-63`). Thus `--this-project` from a
worktree can pay for unrelated Codex, Antigravity, Goose, Pi, OpenCode, and
archive scopes before reducing the result to one repository. The consolidated
query remains efficient, but the fast narrow-path claim is not met.

Ponytail finding: `internal/cli/cli_search.go:29-33` — `shrink:` do not
build and index the complete universe merely to discard it; pass a canonical
repository predicate into discovery, or query the consolidated store directly
with repository-aware metadata.

### Edge cases

- **Nested repositories:** the walk stops at the nearest `.git`, so nested
  repositories are separate identities. This is the least surprising Git policy,
  but should remain explicit.
- **Broken `gitdir`:** an unreadable or non-`gitdir:` file returns the
  worktree directory itself at lines 248-268, silently inventing an identity.
- **Malformed `commondir`:** any readable content, including empty content, is
  accepted at lines 257-263. Empty content returns the metadata directory's
  parent, not the primary repository root; symlink/traversal targets are not
  validated.
- **Missing `commondir`:** the fallback at lines 264-265 assumes Git's standard
  `<main>/.git/worktrees/<name>` layout. Reasonable, but damaged metadata is
  untested.
- **Symlink races:** `Realpath` resolves ordinary symlinks
  (`internal/paths/paths.go:633-653`), but resolution and indexing are not one
  filesystem transaction. Identity is best effort.
- **Detached heads:** no issue; no HEAD or branch data is consulted.

### Verdict: CAUTION

Approve the model and common-case implementation. Hold the stronger speed claim
until narrow scope resolution stops eagerly ensuring unrelated sources. Add
malformed-pointer and `commondir` tests, and make unresolved Git metadata
explicit instead of silently treating a broken worktree as a project.

## 2. Tombstone durability and storage relocation

### Design

`paths.TombstonePath` maps to
`$XDG_DATA_HOME/rawclaw/.deleted`, or
`~/.local/share/rawclaw/.deleted`, at
`internal/paths/paths.go:81-90`. Lifecycle delegates empty `cacheDir` to
that path at `internal/lifecycle/lifecycle.go:195-203`.
`appendTombstones` preserves a newline-delimited flat file, creating the
parent and using `O_CREATE|O_WRONLY|O_APPEND`, with one joined write at
`internal/lifecycle/lifecycle.go:414-432`. `LoadTombstones` uses a bounded
scanner at lines 219-263.

Moving deletion state out of disposable cache is correct: a tombstone is a user
decision, not rebuildable index state. The flat format is minimal, inspectable,
and avoids JSON read-modify-write. Ordinary descriptor handling is sound:
reads and writes use deferred close, and the full race suite passed lifecycle,
archive, index, and CLI tests.

### Concrete compatibility defect

At `internal/lifecycle/lifecycle.go:228`, the legacy file is read only when
`len(set) == 0`. If the durable file contains `new-session` and the legacy
file contains `old-session`, `old-session` is never loaded. A mixed-version
machine can resurrect old deleted sessions after its first new deletion.

The compatibility rule must union the legacy set whenever `cacheDir == ""`,
regardless of durable-set size, with a regression test for both files. The
reader is used by index paths including `internal/index/rebuild.go:103`,
`internal/index/containers.go:335`, and
`internal/index/consolidated.go:1178`.

### Durability and concurrency limits

“Durable” has two meanings:

- **Durable location:** approved. XDG data is the correct owner.
- **Crash/process durability:** incomplete. Append does not call `Sync`,
  deferred close errors are ignored, and deletion is not transactionally coupled
  to the append.

`O_APPEND` plus one write generally avoids interleaving on local POSIX
filesystems, but is not a cross-platform transaction guarantee. More
importantly, `Delete` removes files first at
`internal/lifecycle/lifecycle.go:177-189`, then appends tombstones. If append
fails, files are gone but reindexing has no tombstone and may resurrect them.
Writing tombstones first needs an explicit rule for tombstoned-but-present files;
a lock alone cannot make unlink plus append atomic.

No descriptor leak is indicated. The weakness is ignored sync/close errors and
cross-process ordering, not an unreleased handle.

Ponytail finding: `internal/lifecycle/lifecycle.go:228` — `shrink:` always
union the two flat files; the conditional is unnecessary and wrong.

### Verdict: CAUTION

Approve location and format. Fix mixed legacy/new loading before calling the
migration backward-compatible. Document crash semantics; if deletion must never
resurrect, add a small journal/lock protocol and tests for concurrent writers
and append failure.

## 3. Monolithic CLI modular decomposition

### Design and evidence

`b4724cd` extracts options, search, read, and lifecycle concerns into
`cli_options.go`, `cli_search.go`, `cli_read.go`, and
`cli_lifecycle.go`, leaving construction and execution in `cli.go`. Current
sizes are 1,068, 253, 595, 222, and 203 lines. The extraction is effectively
mechanical: `git diff --find-renames --numstat b4724cd^ b4724cd` shows the
original file reduced by 1,225 lines and corresponding files added. `ca4e147`
carries the shipped merge.

Root construction remains at `internal/cli/cli.go:53-165`; shared flags are
bound once at `internal/cli/cli_options.go:158-194`. Registrations remain for
read, outline, topics, indexing, tags, archive, live, delete, setup, upgrade,
version, and completion. The required full race gate passed, including CLI in
108.141 seconds.

Notes:

- `Options` remains mutable and shared by root and explicit `search`
  (`internal/cli/cli.go:58-60`, `:168-184`). This predates the split, but
  reusing one Cobra tree across executions can retain normalized date state or
  changed flags. Add a repeated-execution isolation test.
- Read and outline define local `--dir`, `--this-project`, `--json`, and
  richness flags in `internal/cli/cli_read.go`; intentional, but a contract
  seam.
- There is no permanent command/flag manifest or golden `--help` snapshot.
  Journey tests are strong but do not prove every help string, hidden alias,
  default, ordering, or error code.
- Keeping `cli.go` above 1,000 lines is not itself failure: it owns a coherent
  command-tree and execution boundary. Further function-level splitting would
  add indirection.

Ponytail finding: none material in the extraction itself; it adds no dependency
or new abstraction.

### Verdict: APPROVED WITH NOTES

Accept the decomposition. Add a command/flag compatibility manifest and
repeated-root-execution state test before claiming the contract is permanently
pinned. Do not split further solely to reduce line count.

## 4. Unified source registration and discovery

### Design

`internal/sources/sources.go:21-33` is an explicit composition root. It
idempotently registers built-in adapters and returns a registry snapshot.
`source.Registration` carries `Label` and `OptedIn` alongside `ID`,
`Detect`, `New`, and `Lookup` at `internal/source/source.go:43-56`.
The mutex, copied snapshots, and callback-without-lock behavior are at
`internal/source/source.go:58-107`.

`scopes.All` consumes `sources.Registered()` at
`internal/scopes/scopes.go:40-69`; `refreshThisProject` does likewise at
`internal/cli/cli_search.go:515-541`. Goose policy now lives in its adapter
at `internal/source/goose/goose.go:55-85`. The behavioral `Source` interface
remains narrow—`Discover` and `Messages`—at
`internal/source/source.go:33-41`.

The boundary is good in the important direction: parsers remain adapters,
canonical containers feed indexing, and selection metadata is outside the
behavioral interface. Explicit registration avoids init-time ordering and
improves test isolation.

### Why it is only partly unified

1. `scopes.All` special-cases Claude by string at
   `internal/scopes/scopes.go:46-55`, bypassing `reg.New`, `reg.Label`, and
   generic `containerScopes`.
2. Orphan discovery hard-codes built-in cache prefixes at
   `internal/scopes/scopes.go:137-165`. A new adapter cannot obtain complete
   orphan lifecycle merely by registering an ID and constructor.
3. Codex, Pi, and OpenCode wrappers still hard-code IDs and labels at
   `internal/scopes/scopes.go:198-267); two dispatch paths remain.
4. `sources.Get` returns a pointer to a range-variable copy at
   `internal/sources/sources.go:36-43`. Safe for current read-only callers,
   but a value plus boolean would make ownership clearer.
5. `All` and refresh assume non-nil `reg.New` at
   `internal/scopes/scopes.go:58-62` and
   `internal/cli/cli_search.go:532-538`. `source.Register` validates neither
   IDs nor callbacks, so malformed extension registration can panic the CLI.
6. `DetectID` exists at `internal/source/source.go:96-107`, but scope
   discovery does not use it. Known source roots still require adapter-specific
   enumeration. The registry unifies dispatch and metadata, not all discovery
   mechanics.

The sovereign-core boundary is upheld if `scopes` is treated as the
adapter-aware composition layer. It would be misleading to call `scopes`
source-agnostic while these built-in cases remain.

Ponytail finding: `internal/scopes/scopes.go:198-267` — `yagni:` keep
one-source wrappers only while callers need them; then delete the duplicate
dispatch path.

### Verdict: CAUTION

Approve registry and metadata placement. Before calling the architecture fully
unified, move orphan/cache naming and Claude's special discovery policy behind
registration capabilities, or document the deliberate two-family model.
Validate extension registrations and test a custom registered source through
both `All` and refresh.

## 5. Rejected scope drift: bundle validation in `d2b1723`

`d2b1723` changes `internal/archive/bundle.go:121-123` from:

```go
a.run(ctx, parent, "bundle", "verify", absBundle)
```

to:

```go
a.run(ctx, parent, "bundle", "list-heads", absBundle)
```

The surrounding comment still says “Verify bundle validity upfront.” These
commands are not equivalent. `git bundle verify` is the integrity/prerequisite
validation gate; `git bundle list-heads` primarily enumerates advertised refs.
Listing heads is not a substitute for validating a backup before clone state
changes at lines 126-140.

This is unrelated to the four initiatives and weakens an existing safety gate.
Revert it or separately justify it with a test demonstrating the required
integrity guarantee. Until then the decision is **REJECT**.

## Final action list

1. Union legacy and durable tombstones whenever `cacheDir == ""`; add a mixed
   file regression test.
2. Decide and document crash/concurrent deletion semantics; test append failure
   and concurrent writers.
3. Reject malformed `gitdir`/`commondir` metadata; add broken and symlink
   cases.
4. Avoid eager all-source indexing before `--this-project` filtering, or revise
   and measure the performance claim.
5. Add a CLI flag/command manifest and repeated-root-execution state test.
6. Generalize source orphan/cache policy or document intentional special cases;
   validate registrations.
7. Revert the `list-heads` bundle-validation substitution.

Overall disposition: **PARTIAL / HOLD for the architectural claims**, with the
CLI decomposition accepted and the core direction sound. The mixed tombstone
fallback and weakened bundle validation should be resolved before describing
this batch as fully durable and fully unified.

