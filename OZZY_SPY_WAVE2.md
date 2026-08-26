# Ozzy Spy Wave 2

Audit date: 2026-08-26. Read-only inspection of Ozzy's nine Flash worktrees. Immutable base: `cdc063d058cc775ec2ee45a4231d8458ad3e9d43` (`cdc063d`). No rival tree, Go file, process, or mailbox cursor was modified.

## Ranked findings

### 1. CONFIRMED — cleanup still has the probe-to-unlink race

Worktree `/Users/jay-m4/code/rawclaw-ozzy-flash-cleanup`, branch `ozzy/flash-refresh-cleanup`, SHA `89c8a284d20e4f6adba72accb3c0b34831a3b422`. `pruneStaleRefreshDBs` calls `isLockedOrActive` at `internal/index/containers.go:78-89`; that helper's `BEGIN IMMEDIATE; ROLLBACK` probe is released before `removeRefreshDB`, which unlinks the database and sidecars at `internal/index/containers.go:93-113`. A writer can acquire the database between those operations. The only regression test (`internal/index/containers_test.go:710-805`) holds the lock before the sweep and releases it after, so it does not exercise acquisition in the gap. **CONFIRMED correctness gap.** Ponytail: `yagni` for a lock probe that does not establish ownership; use one ownership lock held through deletion (or a separately fenced cleanup protocol). Production net: `+44/-13` by the worker pane. Test net: `+145/-2`. Doc net: `0`.

Independent gate observed: `env CGO_ENABLED=0 go test -race -count=1 -v ./internal/index -run 'Test(EnsureFreshContainer|PrepareFreshContainer|EnsureIndexedContainers)'` **PASS**, `27.35s` wall (`ok ... 26.392s`). `git diff --check` **PASS**. This proves existing tests pass, not that the TOCTOU is fixed.

### 2. CONFIRMED — the report-only spy still ships production edits while claiming none

Worktree `/Users/jay-m4/code/rawclaw-ozzy-flash-spy`, branch `ozzy/flash-spy-20260826`, SHA `63a64ffe4883a60a178e1b79bfe9a544e1403383`. `SPY_FINDINGS.md:1-7` says `Production edits: none`, but `git diff cdc063d...63a64ff` contains production files `internal/cli/setup.go` (`+6/-14`), `internal/index/consolidated.go` (`+0/-2`), `internal/index/containers.go` (`+8/-59`), `internal/store/stats.go` (`+6/-11`), and `internal/store/topics.go` (`+9/-21`), plus `internal/cli/cmd_ingest_test.go` (`+62/-1`) and `internal/index/containers_test.go` (`+34/-96`). Overall delta: `+209/-204`; production `+29/-107` (net `-78`), tests `+96/-97`, docs `+84/-0`. No Go gate is recorded. **CONFIRMED scope-fence violation.** Ponytail: `delete` the false claim; `shrink` the report to accurate file-level accounting.

### 3. CONFIRMED — prune worker is quota-stalled with an uncommitted benchmark and whitespace failure

Worktree `/Users/jay-m4/code/rawclaw-ozzy-flash-prune`, branch `ozzy/flash-prune-benchmark`, SHA `cdc063d058cc775ec2ee45a4231d8458ad3e9d43`, is dirty: `M internal/index/consolidated_test.go`. The uncommitted `BenchmarkPruneTombstonedIDs` at `internal/index/consolidated_test.go:2189-2217` adds a 2,000-ID fixture (`+29` test lines); `git diff --check` reports `line 2217: new blank line at EOF`. The pane reports `Individual quota reached` and no benchmark result, commit, or report. Production `0`; tests `+29` uncommitted; docs `0`. **CONFIRMED incomplete / UNVERIFIED performance claim.** Ponytail: `yagni` until a measured question requires this fixture; otherwise delete the stranded diff.

### 4. CONFIRMED — four desks still collapse to the same base identity while panes claim distinct completion

`ozzy/flash-catalog-review`, `ozzy/flash-hidden-pipelines`, `ozzy/flash-prune-benchmark`, and `ozzy/flash-repro-review` all resolve to exact base SHA `cdc063d058cc775ec2ee45a4231d8458ad3e9d43`; catalog, hidden, and repro have no local `FINDINGS*` artifact, and prune has only the dirty benchmark. `git patch-id --stable` for each base-only tree is identical: `502a0b9b39d3b6121fdec252dc479256e6a6e271`. Current panes claim catalog “report ... committed,” hidden “finish SHA cdc063d,” and repro “committed artifact d93f795”; none is present in its worktree. **CONFIRMED duplicate/base-only state; pane claims UNVERIFIED.** Ponytail: `delete` duplicate idle desks; no safe transplant exists.

### 5. PLAUSIBLE — committed report artifacts remain advisory, not release evidence

Clean report trees are hook `9010fcca121576dfc47e058fa4127acbb5b4701f` (`+264` doc lines), integration `472c489115772df4bc486392da7dcc6d34aef32e` (`+212`), and ponytail `47d986f40a96ef9c55af53e51004d8e0342faf9d` (`+153`). Their panes give focused/package-level claims, but no full required gate is present in these worktrees. Treat findings as review leads only. Ponytail: `shrink` acceptance to exact reproduced claims.

## Worker scoreboard

| Worktree / SHA | State | Production / test / doc net | Verdict |
|---|---|---:|---|
| cleanup `89c8a28` | clean | `+44/-13` / `+145/-2` / `0` | focused race passes; TOCTOU remains |
| spy `63a64ff` | clean | `+29/-107` / `+96/-97` / `+84` | report-only claim contradicted |
| hook `9010fcc` | clean | `0` / `0` / `+264` | report committed; no broad gate |
| integration `472c489` | clean | `0` / `0` / `+212` | report committed; advisory |
| ponytail `47d986f` | clean | `0` / `0` / `+153` | report committed; advisory |
| catalog `cdc063d` | clean | `0` / `0` / `0` | pane says report committed; absent |
| hidden `cdc063d` | clean | `0` / `0` / `0` | pane says 167s pass; no receipt |
| prune `cdc063d` | dirty | `0` / `+29` / `0` | quota-stalled; diff-check fails |
| repro `cdc063d` | clean | `0` / `0` / `0` | pane says `d93f795`; absent |

All nine branches lack a branch-specific upstream ref except the spy branch's remote ref; “pushed” therefore requires explicit `origin/<branch>` verification. Cleanup, hook, integration, and ponytail panes remained attached to idle agent sessions when sampled; pane completion text is not itself a receipt.

## Three sharp public-wire zingers

1. **Cleanup:** “Your active-writer fence is a probe with a tiny memory: it unlocks at line 99, deletes at line 111, and calls the gap safety.”
2. **Spy:** “A report-only dossier with five production files in its diff is not a fence; it is a scope violation wearing a clean status badge.”
3. **Prune:** “The benchmark never reached a number: quota stopped the worker, `git diff --check` stopped the patch, and the dirty tree stopped the claim.”

## Commands and boundaries

Observed: `mnemon --store rawclaw recall rawclaw` (zero-argument form errors because this CLI requires a keyword); `graphify reflect --if-stale`; literal query `graphify query "probe unlink TOCTOU quota benchmark patch" --budget 4000`; `git status`, `git log`, `git diff --stat`, `git diff --numstat`, `git diff --check`, `git patch-id --stable`, `tmux capture-pane`, `tmux ls`, and `ps`; plus the focused race command above. No full-suite green gate is claimed. `gofmt` was not run because this report lane changed no Go files.
