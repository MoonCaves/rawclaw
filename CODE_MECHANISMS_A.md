# Code mechanisms A: pinned prior art for four RawClaw clusters

Research target: RawClaw `5b9756b2200ff6bd670f07407407d84d9f42d84b`.
This is report-only; no product source was changed. `COPY` means the mechanism
and invariant transfer directly, `ADAPT` means the protocol transfers but the
implementation must remain RawClaw-specific, and `REJECT` means the apparent
shortcut would weaken the contract.

## 1. POSIX SessionStart catalog claim: claim without opening the target

**Upstream mechanism.** Git's tempfile implementation creates a sibling with
`open(..., O_RDWR|O_CREAT|O_EXCL|O_CLOEXEC)`, treats any failure as “no owned
tempfile,” and only then activates the object:

- Git `c44beea485f0f2feaf460e2ac87fdd5608d63cf0`, `tempfile.c:create_tempfile_mode`,
  [lines 136-152](https://github.com/git/git/blob/c44beea485f0f2feaf460e2ac87fdd5608d63cf0/tempfile.c#L136-L152).
- The lock wrapper keeps the lock object as the owner and reports creation
  errors rather than opening an existing path:
  [lockfile.c lines 175-190](https://github.com/git/git/blob/c44beea485f0f2feaf460e2ac87fdd5608d63cf0/lockfile.c#L175-L190).

Minimal shape:

```text
tmp = exclusive_create(same_directory, random_name)
if tmp failed: target was not claimed; do not open target
write JSON to tmp; close tmp
publish only through an ownership-safe same-directory operation
```

**RawClaw mapping.** `internal/cli/setup.go:24-178` renders Claude and Codex
POSIX hooks; `renderHookScript` is at `:319-321`, and the catalog claim is in
the two template bodies around `:70-111` and `:145-184`. Current HEAD uses
`(set -C; : > "$entry")`: this opens the target before checking its type. A
FIFO can block the hook, a directory/socket fails ambiguously, and a symlink is
not a safe claim. The prior local repair in `13966cf` demonstrates the intended
shell adaptation: write a private temporary regular file, then `ln "$tmp"
"$entry"`; `ln` fails for every pre-existing target without opening it. The
`[ -e "$entry" ] || [ -L "$entry" ]` branch is required for dangling symlinks.

Verdict: **ADAPT**, with the safety rule **COPY**. Do not copy direct shell
redirection. Keep the empty/partial marker semantics and fail-soft ingest, but
make the claim operation one that never opens `$entry`; leave rich JSON in the
private temp file and remove only a temp path this invocation created.

Semantic trap: `O_EXCL` protects the final create but does not make a broad
`[ -e ]` test authoritative; a dangling symlink is false for `-e`, and a
directory or FIFO is still an existing hostile target. Also, `:` is a POSIX
special builtin: redirection failure can terminate `dash` before `|| true` is
consulted. The hook must remain POSIX `sh`, must not use `bash` features, and
must not claim that a failed metadata upgrade lost the dedup claim.

Exact target experiment/gate:

```sh
for shell in /bin/sh /bin/dash; do
  for kind in regular fifo directory symlink socket missing-parent; do
    # run the rendered hook with a unique session id and a stub rawclaw ingest
    # recorder; assert timeout-free exit 0, no FIFO reader/writer block, and
    # at most one ingest for 20 concurrent invocations.
  done
done
```

The fixture must include a dangling symlink (`[ -L ]`), a UNIX socket, a
directory at `$entry`, and a catalog parent that is a regular file. Then run
the concurrent-winner case with 20 processes under both shells and inspect the
recorder plus catalog inode/type. Required gate: `go test -race -count=1
./internal/cli -run 'Test(Prime|Catalog).*'` plus the shell matrix above; a
hang, target open, or duplicate winner is a failure.

## 2. Detached ingest child: Start creates ownership, Wait reaps it

**Upstream mechanism.** Go's `os/exec` separates process creation from
resource ownership. `Cmd.Start` closes setup descriptors on failed start but
leaves a started process owned by the `Cmd`; `Cmd.Wait` calls
`Process.Wait`, records exit state, waits for I/O goroutines, and closes parent
pipe descriptors:

- Go `7f36edc26d4e3becb6d9c9008ff00f260bb19055`, `exec.go:635-651`,
  [Start cleanup](https://github.com/golang/go/blob/7f36edc26d4e3becb6d9c9008ff00f260bb19055/src/os/exec/exec.go#L635-L651).
- Same commit, `exec.go:914-950`,
  [Wait and Process.Wait](https://github.com/golang/go/blob/7f36edc26d4e3becb6d9c9008ff00f260bb19055/src/os/exec/exec.go#L914-L950).

Minimal shape:

```go
if err := cmd.Start(); err != nil { return }
go func() { _ = cmd.Wait() }() // the owner, even when caller returns now
```

**RawClaw mapping.** `internal/cli/bg_ingest.go:70-102` (`spawnIngestChild`)
builds `exec.Command(selfExe(), "ingest", ...)`, applies `detach`, redirects
to `ingest.log`, starts, and currently reaps in a goroutine at `:101`.
`internal/cli/cmd_answer_first_test.go:613-647` replaces `selfExe` with a
fake executable. `internal/cli/cmd_ingest_test.go` exercises hook detachment.

Verdict: **COPY** explicit `Start`/`Wait` ownership. **ADAPT** detachment:
`setsid`/stdio detachment is a RawClaw platform seam, not a substitute for
reaping. Preserve the test seam (`spawnIngest`) and make package tests stub it
by default; the real child path must never resolve `os.Executable()` to
`cli.test` during ordinary tests.

Semantic trap: `Process.Release()` is not reaping. It discards the handle and
can leave a child zombie/orphan; in this repository the more dangerous variant
recursively starts `cli.test`, causing test hangs and lock contention. A
goroutine `Wait` is best-effort if the parent exits immediately, so claims must
be limited to “reaped while the owner remains alive,” not durable supervision.
Do not add a daemon or third-party supervisor to the static binary.

Exact target experiment/gate:

1. Run the focused detached-child test with a fake executable and assert the
   log receipt plus eventual child exit.
2. Run the whole `internal/cli` package with `TestMain` stubbing `spawnIngest`;
   assert no process command line contains `cli.test ingest`.
3. Run the real seam once in an isolated subprocess and inspect `ps` before
   and after exit for no unreaped child; repeat 5 times under `-race`.

Required gate: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli` and
`gofmt -l internal/`; a green focused test without the package-wide recursion
check is insufficient.

## 3. Exact-first, source-aware session resolution

**Upstream mechanism.** Git treats abbreviated object names as candidates, not
identity. Its candidate accumulator ignores duplicate full IDs, marks the
lookup ambiguous when two distinct candidates survive, and returns an explicit
ambiguous status instead of selecting the first one:

- Git `c44beea485f0f2feaf460e2ac87fdd5608d63cf0`, `object-name.c:update_candidates`,
  [lines 55-102](https://github.com/git/git/blob/c44beea485f0f2feaf460e2ac87fdd5608d63cf0/object-name.c#L55-L102).
- The final resolver distinguishes ambiguous, missing, and one-candidate
  results in `finish_object_disambiguation`,
  [lines 221-251](https://github.com/git/git/blob/c44beea485f0f2feaf460e2ac87fdd5608d63cf0/object-name.c#L221-L251).

Minimal shape:

```text
exact namespace/source/full-id lookup first
if no exact hit: collect prefix candidates
deduplicate only identical full identities
0 -> not found; 1 -> resolve; >=2 -> ambiguity with candidates
```

**RawClaw mapping.** `internal/agentproto/agentproto.go:1761-1833` contains
`locateSession`, `locateSessionWithCatalog`, `catalogCands`, and `decideSession`.
`oneStoreCands` at `:1838-1881` currently asks `SessionRowsByPrefix` first;
catalog resolution is `internal/paths/paths.go:298-348`, where an exact file
probe precedes a directory prefix scan. Callers include Read, Outline, and tag
resolution (`agentproto.go:1513`, `:1991-2034`). Current `decideSession`
correctly rejects multiple candidates, but source identity is not carried by
`SessionHit`; `catalogCands` must abandon narrowing when a hit cannot safely be
mapped back to its source (`:1804-1810`) or it can hide a mixed-source match.

Verdict: **ADAPT** Git's candidate protocol and **COPY** the ambiguity result.
Use a candidate key `(source, full session ID)`; exact full ID in the requested
source wins, an exact ID across sources is one identity only when the source
semantics say it is the same merged session, and otherwise prefix candidates
remain visible. Do not use FTS as identity resolution and do not silently rank
one source above another.

Semantic trap: two rows with the same full ID can be one session continued in a
new project and should resolve to the consolidated merged row; two different
full IDs sharing the eight-character label must remain ambiguous. A catalog
miss, unreadable catalog entry, or missing consolidated DB is fallback state,
not proof of “not found.” Project narrowing must be applied in every tier.

Exact target experiment/gate: table-test all of (a) exact full ID, (b) unique
prefix, (c) ambiguous prefix with two different full IDs, (d) same full ID in
two project/source indexes and a consolidated merged row, (e) same short ID
across Claude and Codex with different full IDs, (f) catalog hit whose source
cannot map to a project, and (g) missing/empty consolidated store. Assert the
returned DB, full ID, source/project, and error type. Run focused resolver tests
with `-race`, then `CGO_ENABLED=0 go test -race -count=1 ./internal/agentproto
./internal/paths`.

## 4. CAS-style tag-prep/tag-write publication

**Upstream mechanism.** Git's `update-ref --stdin` transaction has explicit
`start`, `prepare`, `commit`, and `abort` states; each update may include an
expected old object ID. The parser passes that expected value into the
transaction, and commit is a separate publication step:

- Git `c44beea485f0f2feaf460e2ac87fdd5608d63cf0`, `builtin/update-ref.c`,
  update with expected old ID [lines 248-272](https://github.com/git/git/blob/c44beea485f0f2feaf460e2ac87fdd5608d63cf0/builtin/update-ref.c#L248-L272).
- Same file, prepare/abort/commit state machine [lines 548-604](https://github.com/git/git/blob/c44beea485f0f2feaf460e2ac87fdd5608d63cf0/builtin/update-ref.c#L548-L604).
- Same commit, transaction object captures old/new values before prepare in
  `refs.c:1244-1270`, [transaction update](https://github.com/git/git/blob/c44beea485f0f2feaf460e2ac87fdd5608d63cf0/refs.c#L1244-L1270).

Minimal RawClaw-shaped pseudocode:

```text
prep = read messages + existing segments
prep.revision = hash(session messages, segment rows, source)
write = parse model output
BEGIN IMMEDIATE
if current revision != prep.revision: rollback; report stale/conflict
validate anchors and complete range against the same snapshot
replace whole authored set (retag) or insert only the declared disjoint window
COMMIT
```

**RawClaw mapping.** `internal/cli/cmd_tag.go:90-149` (`runTagPrep`) and
`:151-244` (`computeUntaggedWindow`) read existing segments and choose the
earliest untagged window. `runTagWrite` at `:583-697` recomputes the current
window, validates prefix anchors and ordering (`:640-669`), then calls either
`store.ReplaceSessionSegments` or `store.InsertTopicSegments` (`:688-695`).
The store primitives are `internal/store/topics.go:63-110` (whole-session
delete+insert transaction) and `:112-135` (append transaction).

Verdict: **ADAPT** Git's expected-old-value CAS and prepare/commit phases;
**COPY** atomic publication and conflict refusal. Keep RawClaw's useful
insert-only incremental window and explicit `--retag-all` replacement, but bind
the write to a prep fingerprint/revision. Do not publish a replacement if the
session grew, its message UUID/order changed, or another writer changed the
segment set after prep. Downstream fold/archive/ingest remains outside the
foreground transaction.

Semantic trap: transaction atomicity alone is not snapshot freshness. A writer
can delete and insert a perfectly atomic but stale “franken-set” if it does not
compare the prep state. `InsertTopicSegments` is safe only for a disjoint,
validated untagged window; it cannot make overlapping concurrent taggings
converge. `ReplaceSessionSegments` prevents shifted-boundary stacking but must
not be used for a stale prep. A mutation between prep and write must be an
observable conflict, not silent acceptance.

Exact target experiment/gate: create a session with two tagged ranges and an
untagged tail; run prep, mutate a message or segment row, then run write and
assert a conflict with no row change. Repeat with (1) overlapping segment,
(2) shifted boundaries, (3) concurrent writers racing the same prep, (4) clean
disjoint append, and (5) `--retag-all`. Verify no stale rows survive a
successful replacement and no partial set appears after injected insert
failure. Required gates: focused `cmd_tag` and `store` tests under `-race`,
then `CGO_ENABLED=0 go test -race -count=1 ./internal/cli ./internal/store`.

## Strongest directives for target workers

1. Replace direct catalog-target redirection with an owned private temp plus a
   non-opening exclusive publication; test FIFO, directory, symlink, socket,
   missing parent, `/bin/sh`, `/bin/dash`, and 20 concurrent hooks.
2. Keep `Cmd.Wait` ownership and stub `spawnIngest` package-wide so no test can
   recursively launch `cli.test`; detachment is not reaping.
3. Make resolver identity `(source, full ID)`, exact-first, and ambiguity an
   explicit result; same full ID may merge, different full IDs never silently
   rank.
4. Add a prep fingerprint/expected revision and reject mutation between
   `tag-prep` and `tag-write`; use replacement only after that CAS succeeds.
