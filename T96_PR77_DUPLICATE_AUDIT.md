# T96 PR77 duplicate audit

## Scope and evidence

- Base: `origin/main` at `758aa4417794c7a000e90f67c19e51f03817bdfd`.
- Audited head: `e3ec1b86e4ea96d6e11016a63e65bda294d18b3f`.
- Old stable patch ID: `76a55136932d4039d2effdec4c972a5cf3c4d1b1`.
- Current head: `daffc5f50c306e09f7008b295262a7ccedab6cd3` (`fix: bound closeout locks and process trees`).
- Current stable patch ID: `ebdf56d63bdf5828550c6e964d9e0e2485f54d49`.
- Old delta: 348 added lines: 225 production Go, 112 tests, and 11 documentation/findings lines.
- Current delta: 543 added lines: 301 production Go, 231 tests, and 11 documentation/findings lines (the update is `207 added / 12 removed` versus the old head).
- `git diff --check`: clean.
- `CGO_ENABLED=0 go test -race -count=1 -run 'Closeout' ./internal/cli`: passed.
- `~/go/bin/golangci-lint run`: `0 issues`.
- `~/go/bin/dupl -threshold 50 -plumbing .`: no report involving the PR77 files; its output is existing repository-baseline pairs.

## Old versus current head

The update improves correctness coverage: `acquireCloseoutToken` now uses an exclusive, non-expiring lock released by the child, and `closeout_process_unix.go`/`closeout_process_windows.go` bound and kill the tagger process tree. Those are distinct contracts from ingest's expiring throttle and `detach`-only child launch, so they are not duplicate mechanisms to delete. The update does not remove the original `spawnCloseoutChild` wrapper duplication: current `cmd_closeout.go:86-107` still repeats `bg_ingest.go:107-136` for executable resolution, ingest-log setup, stdio, detached start, and asynchronous wait.

The current `dupl -threshold 50` run additionally flags `internal/cli/bg_ingest.go:37-42` and `:80-85`: the same allowlisted session-ID sanitization is present in the expiring ingest marker and the closeout lock path. This is a confirmed small duplicate introduced by the update, independent of the larger process-wrapper finding.

The current 231-line test file adds concurrency, token lifetime, and descendant-timeout coverage. No test deletion is justified; these tests pin the new lock/process-tree contracts.

## Findings

1. `internal/cli/cmd_closeout.go:85-106` duplicates the process wrapper in `internal/cli/bg_ingest.go:73-102`: resolve `selfExe`, open the ingest log, configure `detach`, close stdin, route stdout/stderr to one file, start, and reap with `cmd.Wait`. The only meaningful differences are the argument vector, returned-versus-ignored errors, and closeout's need to keep a waiter. `selfExe`, `detach`, `openIngestLog`, and the per-session token are already native shared seams; no replacement is needed for those.

   `shrink:` extract one small helper in `bg_ingest.go`, for example `startDetachedLogged(args []string, wait bool) error`, which owns `selfExe`, `exec.Command`, `detach`, stdio, `Start`, and either asynchronous `Wait` or `Process.Release`. `spawnCloseoutChild` passes `[]string{"--timeout", "0", "closeout", "--child", sessionID}` and `wait=true`; `spawnIngestChild` passes its ingest args and ignores the error with its current best-effort contract. This is a cross-file follow-up, not a safe report-only transplant inside PR77. A helper of roughly 14 lines replacing both wrappers can remove about 14 production lines net; measure the actual diff before accepting it. The current head leaves this same opportunity intact.

   Contract to preserve: closeout returns executable/log/start errors to its foreground caller; ingest remains best-effort and silent; both use a bare `exec.Command`, `setsid`, closed stdin, the append/rotation behavior of `openIngestLog`, and asynchronous reaping so test binaries do not leak children. Do not make `CommandContext` or unconditionally `Release` the closeout child.

2. `internal/cli/autosync.go:71-91`, `internal/cli/tagpublish.go:46-70`, and `internal/cli/vectortopup.go:27-47` contain the same detached command shape. They are useful prior art for a future shared helper, but they are not direct PR77 reuse: autosync/vector-topup/tag-publish intentionally `Process.Release`, while ingest/closeout wait asynchronously; their log paths and error postures also differ. A helper must expose the wait/release choice and accept an already-open log file. Do not merge the log-openers merely to make the call sites look alike.

2a. `internal/cli/bg_ingest.go:37-42,80-85` is a confirmed `dupl` hit for session-ID sanitization. `shrink:` extract `sanitizeSessionID(string) string` and use it in both marker-name builders. Preserve the exact allowlist (`A-Z`, `a-z`, `0-9`, `-`, `_`) and underscore substitution; this is a safe local helper and should remove roughly 6 production lines net.

3. `internal/cli/cmd_closeout.go:108-135` has a one-caller config path/loader. `store.CacheDir()` looks similar, but direct replacement is not behavior-preserving: `CacheDir` creates the directory and degrades to relative `.cache` when home resolution fails, whereas closeout currently surfaces a home-resolution error and does not create the tagger-config directory during a read. Keep this wrapper unless the config-path contract is deliberately changed and tested. A new configuration interface or process abstraction would be YAGNI.

4. `internal/cli/cmd_closeout.go:175-201` and `204-219` are not safe native duplicates. The tagger runner needs a timeout, captured stdout, stderr forwarding, and JSON validation; the self-command runner needs inherited executable resolution, stdin bytes, captured stdout, and a non-context command. Combining them would add options and obscure two different error contracts. Keep them separate.

5. `internal/cli/cmd_closeout_test.go:14-27` repeats a small test config writer, but it is local to the new feature and no existing helper owns this exact tagger-config schema. `dupl` did not flag it. Do not add a test fixture package for 14 lines; the smallest option is to leave it until a second config consumer exists.

## Native reuse decision

Native reuse already replaces the individual `selfExe`, `detach`, `openIngestLog`, and token mechanisms. It cannot replace the whole `spawnCloseoutChild` body without introducing a smaller shared process helper in `bg_ingest.go`; that helper should be a follow-up patch with explicit wait/release behavior. No standard-library function removes this wrapper: `exec.Cmd` still needs the platform-specific `detach` seam and log wiring.

## Verdict

PATCH — current head adds honest lock/process-tree contracts and tests, but the detached process wrapper remains avoidable duplication. Request the small cross-call-site helper before calling PR77 fully lean; do not alter configuration semantics, closeout lock lifetime, or collapse the timeout/tagger and self-command runners.

Potential shrink: approximately 20 production lines after both behavior-preserving helper extractions; no justified test or documentation deletion identified.
