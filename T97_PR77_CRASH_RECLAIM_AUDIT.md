# T97 PR77 crash reclamation audit

Date: 2026-08-28 WITA  
Scope: closeout token loss after acquisition and before detached child `Start`  
Product edits: none

## Verdict

**ACCEPT for the bounded crash-reclamation mechanism at candidate `8dd07064357bbb1b922e1c4953d58ff0fbaaaf31`.** The repair addresses the Tick96 failure mode: a process dying after ownership acquisition leaves a lock directory, and a later acquisition can reclaim it after the 10-minute TTL. The public `daffc5f` remains the historical pre-reclamation implementation and is not promoted by this report.

## Identities and patch

- Historical assignment HEAD: `6e3694366473b8b656d931b9eb4fae2e03a4fe2e` (`worker/han-t97-pr77-crash-reclaim-20260828`).
- Decisive moving-target HEAD: `8dd07064357bbb1b922e1c4953d58ff0fbaaaf31`, detached worktree `/Users/jay-m4/code/rawclaw-han-t97-pr77-crash-reclaim-8dd`.
- Public comparison base: `daffc5f50c306e09f7008b295262a7ccedab6cd3`.
- `git diff daffc5f..8dd0706 --stat`: 5 files, 312 insertions, 57 deletions.
- Stable patch ID for the complete comparison diff: `8b0a65bdea749410c0d7993673f4d3eb929ff787` (range diff is not a single commit patch).

Graphify orientation used the supplied `/private/tmp/rawclaw-pr77-source-rereview3-6e36943/graphify-out/graph.json`: `acquireCloseoutToken()` is in `internal/cli/bg_ingest.go:64`, calls `reclaimCloseoutToken()` at line 72, and is called from `runCloseout()` at `internal/cli/cmd_closeout.go:80`; `spawnIngestChild()` is in `bg_ingest.go:168`. The graph snapshot predates the 8dd moving-target commits, so source and tests below are authoritative for current behavior.

## Evidence

### Baseline / historical behavior

At `daffc5f`, `acquireCloseoutToken` uses `O_CREATE|O_EXCL` on one lock file and has no stale reclamation. A crash after acquisition and before `spawnCloseout`/child `Start` therefore leaves the lock indefinitely. The baseline focused closeout suite was green (`CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'Closeout' -v`, exit 0, 3.94s), but it cannot prove recovery because that implementation has no reclamation path.

### Candidate tests

Exact-one preflight on 8dd:

```text
go test ./internal/cli -list 'Closeout'
```

Listed 13 closeout-related tests. Focused race gate:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^(TestRunCloseout_|TestCloseoutToken_)' -v
```

Exit 0; all 7 selected tests passed; elapsed 1.97s.

Additional exact lease/child gate:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/cli \
  -run '^TestRunCloseoutChild_|^TestCloseoutToken' -v
```

Exit 0; all 5 selected tests/subtests passed; elapsed 1.70s.

A disposable test (created, run, and removed outside the report fence) acquired a token, simulated death before `Start` by aging the lock directory to `-2*closeoutTokenTTL`, and reacquired successfully. It also verified that the live lease was not reclaimed. Exit 0; elapsed 1.92s under `-race`.

## Boundary inspection

- **Owner/PID reuse:** no PID is stored or trusted. Ownership is an opaque 64-hex-character token file; PID reuse cannot validate a stale lease.
- **Stale timing:** `closeoutTokenTTL = closeoutChildTimeout*2 = 10m`. `reclaimCloseoutToken` checks lock-directory `ModTime`, then atomically renames it to a unique `.stale-<token>` quarantine before removal. A bounded child has a 5m context timeout, so a normal owner cannot remain valid beyond the reclaim window.
- **Malformed token:** reclamation is based on stale lock-directory age, not token filename contents. This safely recovers a crashed or partially-written lease, while `validateCloseoutToken` still requires a valid 32-byte hex token for child execution.
- **Failed token write/remove:** token-write failure returns false and attempts to remove the lock directory; a leftover empty directory is still reclaimable after TTL. Spawn failure in `runCloseout` explicitly calls `releaseCloseoutToken`.
- **Concurrent acquisition:** `os.Mkdir` is the winner gate. Only one contender can own the directory; stale contenders race on `Rename`, where only one can win, then the acquisition loop retries. Existing `TestRunCloseout_ConcurrentParentsLaunchOnce` passed.
- **Spawn failure:** `TestRunCloseout_SpawnFailureReleasesToken` passed, and the code releases the exact opaque token when `spawnCloseout` returns an error. A child defers exact-token release after completion or failure.

## Residual limitation

The stale decision uses filesystem directory mtime and wall-clock `time.Since`; a clock moving backward delays recovery, and an owner process exceeding the intended 5m child bound could be reclaimed after 10m. Neither occurs on the bounded closeout path tested here. No empirical SIGKILL process run was needed: the disposable exact state-machine test isolates the required crash point without a timing-dependent 10-minute wait.

## Tree/commit receipt

The report-only worker tree was kept free of product edits. `gofmt -l internal/` produced no output. The report is the only intended file change in the worker branch.
