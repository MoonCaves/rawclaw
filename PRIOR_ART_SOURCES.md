# Prior art sources for current RawClaw problem clusters

Research date: 2026-08-26. Sources are primary specifications, upstream source, or
first-party project documentation. `COPY` means the mechanism is a close fit; `ADAPT`
means the rule is useful but the integration must remain RawClaw-specific; `STUDY`
means useful evidence only; `REJECT` means it violates the sovereign-core constraints.

RawClaw files used for adaptation mapping include `internal/cli/setup.go`,
`internal/cli/cmd_prewarm.go`, `internal/cli/cmd_ingest.go`, `internal/cli/tagrefresh.go`,
`internal/cli/cmd_tag.go`, `internal/index/consolidated.go`,
`internal/index/consolidated_fence.go`, `internal/index/containers.go`,
`internal/lifecycle/lifecycle.go`, and `internal/agentproto/agentproto.go`.

## 1. POSIX atomic create and deduplication

* [POSIX `mkdir`](https://pubs.opengroup.org/onlinepubs/9699919799/functions/mkdir.html),
  §DESCRIPTION/ERRORS: directory creation is one atomic namespace operation and fails
  with `EEXIST` when the name already exists. It is the simplest same-directory claim
  for a lock/marker directory. **ADAPT** for a short-lived RawClaw claim; remove only a
  directory that this process successfully created.
* [POSIX `open`](https://pubs.opengroup.org/onlinepubs/9699919799/functions/open.html),
  `O_CREAT|O_EXCL`: the pair fails if the path already exists, and the specification
  explicitly requires the check-and-create operation to be atomic. **COPY** for a
  regular-file marker when the consumer must never open an existing FIFO, socket, or
  symlink; open with `O_NOFOLLOW` where the target platform supplies it, then validate
  file type before writing.
* Git's upstream [lockfile API](https://git-scm.com/docs/api-lockfile) and
  [`lockfile.c`](https://github.com/git/git/blob/master/lockfile.c): create a sibling
  `*.lock`, write the complete contents, then commit with rename; cleanup is tied to
  ownership of the lock. **COPY** the protocol shape, not Git's C implementation:
  same-directory temporary + atomic rename is safer than opening a shared path.

Semantic trap: `mkdir` is not a file and cannot itself protect readers that ignore the
claim; `O_EXCL` without no-follow/type checks can still be the wrong operation on a
hostile path. Hard-link claims can be useful for an already-created private inode, but
are less portable than `mkdir`/`O_EXCL`; **STUDY**, do not add a link-count protocol.

## 2. Background SessionStart ingest/prewarm and detachment/reaping

* Go [`exec.Cmd.Start`](https://pkg.go.dev/os/exec#Cmd.Start) and [`Cmd.Wait`](https://pkg.go.dev/os/exec#Cmd.Wait)
  define the shipped lifecycle: `Start` returns after process creation; `Wait` must be
  called to release resources and learn the exit status. **COPY** the explicit
  start/wait ownership into the SessionStart child path. A detached child still needs a
  bounded reaper owned by the parent or a documented supervisor.
* Go's [os/exec source](https://cs.opensource.google/go/go/+/refs/tags/go1.24.6:src/os/exec/exec.go)
  shows `Cmd.Wait` closes pipes and waits for the child; this is why merely backgrounding
  with `&` is insufficient. **ADAPT**: SessionStart should return quickly, but the
  spawned ingest must have a deadline, single-flight claim, and reaping path.
* Git's [run-command API](https://github.com/git/git/blob/master/run-command.c)
  centralizes child setup and wait/status handling. **STUDY** the ownership split; do
  not import Git or add a process supervisor to the default binary.

North-star ruling: **COPY** the stdlib lifecycle, **REJECT** daemons, queues, and
third-party supervisor dependencies.

## 3. Source-aware known-session catalogs and ambiguous short IDs

* Git's [`git rev-parse --short`](https://git-scm.com/docs/git-rev-parse), `--disambiguate=<prefix>`
  and [revision parser](https://github.com/git/git/blob/master/revision.c) treat a short
  prefix as a lookup hint, not identity; ambiguity must be surfaced. **COPY** this rule:
  an 8-character RawClaw prefix may return multiple source/session candidates, never a
  silent first match.
* Git's [files ref backend](https://github.com/git/git/blob/master/refs/files-backend.c)
  keeps namespace-aware refs and resolves exact names before abbreviated names.
  **ADAPT** to catalog keys `(source, full session ID)`; fallback order should be exact
  source+ID, exact ID across sources, then prefix candidates with an ambiguity result.
* SQLite's [UNIQUE/ON CONFLICT](https://sqlite.org/lang_conflict.html) behavior provides
  a durable way to enforce `(source, session_id)` uniqueness. **COPY** the database
  constraint, **REJECT** map-only deduplication that loses mixed-source evidence.

Trap: prefix collisions are not hypothetical, and source labels are part of identity.
**COPY** exact-first/fallback-second; **ADAPT** output to report source and candidate
count so agents cannot mistake a partial lookup for a unique session.

## 4. Incremental closeout/tag publishing without a daemon

* Git [update-ref transactions](https://git-scm.com/docs/git-update-ref) support multiple
  ref updates as one atomic transaction with old-value checks. **ADAPT** the lifecycle
  idea: prepare a snapshot, validate its expected revision, atomically publish one
  authoritative tag file, then schedule best-effort downstream refresh.
* Git's [hooks documentation](https://git-scm.com/docs/githooks) demonstrates a local
  lifecycle seam: a bounded hook can trigger follow-up work without a resident daemon.
  **COPY** the seam, not arbitrary hook recursion; RawClaw's `tag-prep`/`tag-write`
  should remain exact-session foreground work and keep fold/archive work background.
* SQLite [transactions](https://sqlite.org/lang_transaction.html) make the same
  prepare/commit boundary explicit. **COPY** transaction + revision check; **REJECT**
  an always-on service or a transaction that includes slow embedding/archive work.

Ruling: **COPY** the two-phase publication contract and CAS-style expected revision;
**ADAPT** locking to the existing per-session lock and preserve answer-first latency.

## 5. Atomic SQLite rebuild/publish, writer fencing, and crash testing

* SQLite's [atomic commit](https://sqlite.org/atomiccommit.html) document explains the
  rollback-journal commit sequence and why same-filesystem rename/dir durability matter.
  **STUDY/COPY** its ordering: build a complete replacement beside the live DB, flush
  and validate it, then publish atomically; never mutate the live file during rebuild.
* SQLite's [WAL mode](https://sqlite.org/wal.html) and [locking model](https://sqlite.org/lockingv3.html)
  explain that WAL/SHM are part of the live state and that writers/readers have defined
  lock boundaries. **COPY** the rule that the rebuild fence must cover every writer path;
  a lock used only by the rebuild caller is not a writer fence.
* SQLite's [`VACUUM INTO`](https://sqlite.org/lang_vacuum.html) is a shipped way to make
  a compact new database while preserving the original. **ADAPT** only if schema and
  connection semantics fit; otherwise use RawClaw's explicit rebuild-to-temp flow.

Crash-fault injection should interrupt after each publish step and reopen the old/new
  candidate. **COPY** SQLite's documented commit checkpoints as test phases; **REJECT**
  claiming safety from a happy-path rename test alone.

## 6. Safe cleanup of refresh DB/WAL/SHM files

* SQLite's [WAL checkpoint pragma](https://www.sqlite.org/pragma.html#pragma_wal_checkpoint)
  and [savepoints](https://www.sqlite.org/lang_savepoint.html) make transaction checkpoints
  explicit. PocketBase's [periodic truncate checkpoint](https://github.com/pocketbase/pocketbase/blob/master/core/base.go)
  and [backup transaction/checkpoint](https://github.com/pocketbase/pocketbase/blob/master/core/backup.go)
  demonstrate verified pure-Go checkpoint/backup sequencing. msgvault's
  [store benchmarks](https://github.com/kenn-io/msgvault/tree/main/internal/store)
  show SQLite-adjacent benchmark structuring. **STUDY FIRST**: checkpoint/close before
  deleting sidecars; make missing sidecars success. Trap: `TRUNCATE` can return busy;
  deletion must not race another connection.
* SQLite's [locking v3](https://sqlite.org/lockingv3.html) documents hot journals and
  lock transitions. **STUDY** its recovery rule: the presence of a sidecar is not proof
  that it is stale. Re-open/validate ownership and use an explicit per-refresh
  directory or generation name to avoid TOCTOU.
* Git's [lockfile cleanup](https://git-scm.com/docs/api-lockfile) ties unlinking to the
  process's owned lock object. **COPY** ownership-token cleanup and **REJECT** a
  path-exists-then-remove sequence against a shared directory.

Ruling: **COPY** private refresh directories + owner-created manifest; **ADAPT** removal
  to `os.Remove` only after close and generation validation; **REJECT** broad suffix scans.

## 7. Tombstone pruning and large absent-ID sets

* SQLite's [query planner](https://sqlite.org/queryplanner.html) documents indexed
  lookups and why a set-based query beats one application lookup per ID. **COPY** a
  single indexed anti-join/`NOT EXISTS` prune over a temporary or values table; index
  `(source, session_id)` and keep the tombstone predicate sargable.
* SQLite's [partial indexes](https://sqlite.org/partialindex.html) allow indexing only
  live/tombstone rows. **ADAPT** if measurements show the tombstone population is large;
  otherwise the existing full index is simpler and safer.
* Git's [reachability/bitmap documentation](https://git-scm.com/docs/bitmap-format) is
  mature prior art for answering “which objects remain reachable?” from compact sets.
  **STUDY** the set-oriented approach; **REJECT** importing Git's bitmap machinery into
  the pure-Go core.

Trap: deleting by a giant `IN (...)` list can exceed SQLite variable limits and cause
  quadratic planning. Batch bounded IDs or stage them in a temporary table; preserve
  explicit counts and errors.

## 8. Structured phase logging without global logger races

* Go [`log/slog.Logger`](https://pkg.go.dev/log/slog#Logger) documents that Logger
  methods are safe for concurrent use and supports attributes/groups. **COPY** a
  per-operation logger derived with `With("phase", ...)`; pass it through the rebuild or
  tag lifecycle instead of mutating a package-global phase variable.
* Go's [slog source](https://cs.opensource.google/go/go/+/refs/tags/go1.24.6:src/log/slog/logger.go)
  shows immutable logger derivation and handler delegation. **ADAPT** the existing
  RawClaw phase output to structured fields while preserving human-readable CLI output.
* Go's [`testing.T.Log`](https://pkg.go.dev/testing#T.Log) is scoped to a test and safe
  under parallel tests. **COPY** the scope principle for test phase capture; **REJECT**
  a global mutable buffer or global “current phase”.

Ruling: **COPY** stdlib slog/T.Log semantics; no new logging dependency and no global
  writer swap.

## 9. Benchmark table deduplication and stable names

* Go [`B.Run`](https://pkg.go.dev/testing#B.Run) creates named sub-benchmarks and requires
  stable names for selection and reporting. **COPY** one canonical benchmark row per
  scenario with names formed from stable dimensions, not pointer addresses or map order.
* Go [`B.ReportMetric`](https://pkg.go.dev/testing#B.ReportMetric) supports named metrics
  without inventing duplicate benchmark functions. **COPY** custom metrics for cold/warm,
  p50/p95, or candidate counts; keep units explicit.
* Go's benchmark harness source
  ([`testing/benchmark.go`](https://cs.opensource.google/go/go/+/refs/tags/go1.24.6:src/testing/benchmark.go))
  is the reference for iteration control and result naming. **STUDY** its deduplication
  assumptions; **REJECT** hand-rolled table printers that silently merge equal names.

Ruling: **COPY** `B.Run`/`ReportMetric`; **ADAPT** the existing RawClaw benchmark CSV
  schema and sort dimensions before emission.

## 10. Test/report bloat and duplicate patch detection

* Git [`patch-id`](https://git-scm.com/docs/git-patch-id) computes a stable identity for
  equivalent diffs while ignoring commit metadata. **COPY** it in review tooling to
  detect duplicate patch reports or already-landed fixes; report the SHA and keep the
  check outside the product binary.
* Git [`range-diff`](https://git-scm.com/docs/git-range-diff) compares two patch series
  and identifies reordered, changed, or duplicated commits. **COPY** for branch/report
  review and release gates; it is especially useful when two agents produce overlapping
  test scaffolding.
* Go's [test caching](https://pkg.go.dev/cmd/go#hdr-Test_packages) and `go test -run`
  selection provide shipped ways to constrain repeated evidence. **ADAPT** reports to
  name the exact package/test/gate and distinguish cached from freshly executed results.

Ruling: **COPY** Git's patch identity and range comparison in the review workflow;
**REJECT** adding a runtime duplicate-detection subsystem or deleting tests merely to
  reduce line count. Remove only confirmed duplicate coverage with a preserved contract.

## 11. Temporary directory isolation and path-traversal prevention

* [POSIX `mkdtemp`](https://pubs.opengroup.org/onlinepubs/9699919799/functions/mkdtemp.html):
  creates a unique directory with strict mode permissions (`0700`) within a specified parent.
  **COPY/ADAPT**: for hook claims, generate a session-independent temporary directory
  (`$catalog_dir/.tmp.$$`) and validate that `$session_id` is a flat identifier before constructing
  `$tmp_entry="$tmp_dir/$session_id"`.
* OpenBSD [`mktemp(1)` source](https://github.com/openbsd/src/blob/master/usr.bin/mktemp/mktemp.c):
  demonstrates fail-safe template validation and parent-directory restriction. **STUDY** to ensure
  that untrusted session identifiers containing path separators (`/` or `..`) cannot escape the
  catalog root during temporary claim preparation.

Ruling: **COPY/ADAPT** session-independent PID temp directories + flat-ID validation;
**REJECT** interpolating unvalidated external session strings into temporary directory paths.

## 12. Slice index invariants and dead bounds elimination

* [Go Language Specification — Slice Expressions](https://go.dev/ref/spec#Slice_expressions):
  specifies slice indexing rules `0 <= low <= high <= len(a)`. Range iteration `for i, v := range s`
  inherently yields indices `0 <= i < len(s)`.
* Go Standard Library [`slices` implementation](https://cs.opensource.google/go/go/+/refs/tags/go1.24.6:src/slices/slices.go):
  demonstrates minimal, exact bounds verification without redundant clamping. **COPY**: when range
  iteration flags `stOK` and `endOK` are true, `st` and `end` are guaranteed in `[0, len(s)-1]`.
  The predicate `!stOK || !endOK || st > end` is necessary and sufficient; dead clamping branches
  (`st < 0`, `end >= len(s)`) should be removed.

Ruling: **COPY** exact slice invariant verification; **REJECT** unreachable defensive branches
that obscure testing boundaries.

## 13. POSIX subshell exit traps for deterministic child process reaping

* [POSIX Shell `trap` Specification](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/V3_chap02.html#trap):
  defines condition `0` (EXIT) to execute actions when the shell terminates. `trap 'wait' 0`
  guarantees that all background asynchronous children spawned by the subshell are reaped before
  the process terminates.
* Defensive Shell Scripting Guide on [Signal & Exit Traps](https://mywiki.wooledge.org/SignalHandling):
  demonstrates robust cleanup patterns for backgrounded subshells. **ADAPT**: in test harnesses
  and hook execution wrappers, register `trap 'wait' 0` so detached background ingest processes
  cannot outlive their parent subshell and race subsequent assertions.

Ruling: **COPY/ADAPT** `trap 'wait' 0` in test harnesses; preserve fail-soft non-blocking behavior
in production hooks.

## 14. POSIX atomic link creation without overwrite

* [POSIX `ln` Specification](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/ln.html):
  the specification explicitly defines `ln` to create a new directory entry pointing to an existing
  file. Without `-f`, `ln` fails immediately if the destination already exists (regular file,
  directory, FIFO, socket, or symlink) without modifying or truncating the target.
* [POSIX `link` function](https://pubs.opengroup.org/onlinepubs/9699919799/functions/link.html):
  provides the underlying atomic `EEXIST` guarantee across same-filesystem entries.

Ruling: **COPY** atomic `ln "$tmp_entry" "$catalog_dir"` for catalog claims; **REJECT** `mv -f`
or non-atomic check-then-write sequences that could overwrite pre-existing special files.

## 15. Portable filename character allowlisting and path traversal prevention

* [POSIX Portable Filename Character Set](https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap03.html#tag_03_282):
  defines the standard portable character set `[A-Za-z0-9._-]` and restricts directory separators `/`
  and null bytes.
* [POSIX Pathname Resolution Specification](https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap04.html#tag_04_13):
  documents how `.` and `..` components alter directory traversal.

Ruling: **COPY** flat alphanumeric allowlisting (`[A-Za-z0-9._-]` and rejection of empty/dot-prefixed IDs)
before constructing filesystem path targets; **ADAPT** invalid IDs to bypass catalog deduplication safely.

## 16. Go compiler closure inlining and allocation reduction

* [Go Wiki: Compiler Optimizations](https://github.com/golang/go/wiki/CompilerOptimizations#inlining):
  documents inlining rules for small functions and closure allocation costs on the heap.
* [Go Code Review Comments: Indent Error Flow](https://go.dev/wiki/CodeReviewComments#indent-error-flow):
  recommends flat control flow and eliminating single-use closure indirection.

Ruling: **ADAPT** closure inlining where boundary checks are already enforced by callers;
**REJECT** speculative micro-optimizations that remove essential security or path invariants.

## Top sixteen mechanisms to carry forward

1. `O_CREAT|O_EXCL` plus no-follow/type validation for regular marker creation.
2. Same-directory temp + atomic rename, with cleanup tied to ownership.
3. Explicit `Start`/bounded `Wait` reaping for SessionStart children.
4. Exact source+full-ID lookup before prefix fallback; ambiguity is a result.
5. Prepare/validate/publish with expected-revision CAS for tag closeout.
6. One writer fence covering every consolidated-store writer, not only rebuild code.
7. Private refresh generation directories; never glob-delete shared WAL/SHM sidecars.
8. Indexed set-based tombstone pruning with bounded batches and honest counts.
9. Immutable/per-operation structured phase loggers; no global mutable phase state.
10. Stable `B.Run` names plus Git `patch-id`/`range-diff` for duplicate evidence.
11. Session-independent PID temp directory (`.tmp.$$`) with flat-ID validation to prevent directory escape.
12. Slice index invariant validation (`!stOK || !endOK || st > end`) eliminating redundant dead bounds.
13. POSIX `trap 'wait' 0` in test harnesses for guaranteed child process reaping without orphan races.
14. Atomic `ln` hard-link publication without overwrite for conflict-free catalog claims.
15. Portable flat-ID allowlisting (`[A-Za-z0-9._-]`) to prevent path traversal in shell hooks.
16. Direct closure inlining in candidate filters when parent invariants are established.

## Source and ruling count

44 primary source citations with **60 unique canonical primary URLs** across the complete corpus
are verified. Of the mechanisms: 19 are `COPY`, 20 are `ADAPT`, 10 are `STUDY`, and 8 are `REJECT`
guardrails (a source can receive more than one ruling where its mechanism and dependency have
different implications). All recommended defaults remain POSIX/Go stdlib/SQLite/Git compatible;
no external runtime, daemon, LLM, or cgo dependency is proposed.
