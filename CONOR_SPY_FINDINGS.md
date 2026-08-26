# Conor supervisor and worker audit

Date: 2026-08-26. Scope: read-only inspection of the seven Conor trees and six
Luna trees named by the supervisor, plus tmux/process and worker-log evidence.
No Go files were changed. Evidence is ranked by what the checked-out trees and
logs actually prove; worker final messages are not treated as gates by
themselves.

## Scoreboard

| Rank | Finding | Ruling | Evidence | Verdict |
|---|---|---|---|---|
| 1 | Luna 32-A reports green after a logged failing package run | management: reject green claim; rerun from a clean tree | `rawclaw-luna-conor-32a/.codex-run.log:11664-11665` records `unable to open database file: out of memory (14)` and `--- FAIL: TestConsolidate_RetryAfterAbruptPostMergeExit`; the same log ends `FAIL ... internal/index 172.083s` at `17297-17299`. Its final message nevertheless says package race passed and commit `ecf21a7` is complete. | **CONFIRMED** |
| 2 | “Independent” Conor supervisor trees are one branch-shaped integration copy, not independent evidence | management/yagni: deduplicate by patch-id and audit one immutable base | `rawclaw-conor-ambiguity-contract`, `-store-demolition` both point at `5b9756b`; all seven Conor trees share merge-base `86c5ce0` and are 29–44 commits ahead of `origin/main`. The ambiguity tree is `44` commits ahead and `1,645+ / 568-` versus that base; bench tree is `43` ahead and `1,542+ / 270-`. The same recent commits (`824014f`, `43b183a`, `d2e6aac`, `d9474fb`, `0ef6d0c`) appear in every Conor tree. | **CONFIRMED** |
| 3 | Patch identity is duplicated across supervisor lanes, inflating the apparent review count | management: count unique patch-ids, not worktree names | In every Conor tree, the recent history repeats the same patch sequence: `14fabcf`, `f8b9595`, `5b9756b`, `ae1ea13`, `824014f`, `db98135`, `43b183a`, `d2e6aac`, `d9474fb`, `0ef6d0c`. The trees therefore cannot be scored as seven independent implementations. `git log` also shows shared refs such as `integrate/tagwrite-closeout-wave1` and `conor/fix-hook-fifo-claim` at the same commits. | **CONFIRMED** |
| 4 | “Deletion” work removes review artifacts, not production complexity | delete: accept only as bookkeeping, not as a code-demolition result | Conor commit `5610f95` is exactly `FINDINGS-OZZY-INTEGRATION.md` 105 lines deleted and `FINDINGS-OZZY-REPRO.md` 118 lines deleted: `223` documentation lines and zero production/test lines. The current bench tree still carries a tracked `FINDINGS.md`; the ambiguity tree also carries tracked `FINDINGS.md`. | **CONFIRMED** |
| 5 | Issue-31 completion receipt omits the required shuffle dimension | management: mark shuffle coverage unverified | Luna 31’s executed command at `.codex-run.log:11064` is `CGO_ENABLED=0 go test -race -count=5 ./internal/index -run 'TestConsolidatedFence_LogsAcquireDurationOnTimeout'`, followed by the package gate. The final receipt (`.codex-final-message.txt:5-8`) claims focused race count five and package race, but no `-shuffle=on` appears in the focused command. | **CONFIRMED** |
| 6 | Luna “finished” trees retain worker/runtime artifacts and at least one generated graph | management/delete: clean the lane before accepting its result | All six Luna trees have untracked `.agent-mailbox/`, `.codex-final-message.txt`, and `.codex-run.log`; `32a`, `pr35`, `pr35-resolve`, and `pr35-containers` additionally show untracked `graphify-out/` in `git status`. This contradicts the workers’ own “clean” language and makes their reported final state non-reproducible. | **CONFIRMED** |
| 7 | Conor’s branch churn hides the production/test tradeoff | management: compare against the task base, not `origin/main` alone | Relative to the shared integration base `86c5ce0`, Conor’s final trees range from `1,368+ / 75-` to `1,645+ / 568-` across 21–25 files. The large additions include `cmd_prewarm.go` (+136), `cmd_ingest_test.go` (+137–198), `tagrefresh_test.go` (+165), and `consolidated_test.go` (+262), while the only conspicuous benchmark shrink is `internal/store/connect_bench_test.go` (363 deletions). This is a mixed integration wave, not a narrowly attributable demolition. | **CONFIRMED** |

## Process and pulse evidence

Conor’s 2026-08-26 heartbeat receipt (`.agent-mailbox-norm/20260826T100019Z-163f6e14-conor-heartbeat-1-norm-demolit.md`)
explicitly reports `Luna pulse=0/6` while listing six finished-looking Luna
commits. That is consistent with the stale/finished-worker evidence above,
not evidence that six workers delivered six independently checked results.

At audit time, `tmux ls` showed the Conor/Lenny raid panes still present,
including `lenny-raid-containers`, `lenny-raid-hooks`, `lenny-raid-phase`,
`lenny-raid-locate`, and `lenny-raid-prewarm`. Captured panes contain final
receipts while the sessions remain alive. This is stale process state and
should be treated as **UNVERIFIED** until the supervisor records an observed
exit or explicitly labels the pane abandoned.

## Safe transplant candidates

These are candidates, not approvals to merge the surrounding integration wave:

* **`d5d036b` (Luna 31):** the commit removes 57 lines of duplicate fold-phase
  test code while retaining the timeout proof. The worker log records its
  focused race command and package gate; however, the focused command has no
  `-shuffle=on`, so transplant only after rerunning the exact desired matrix.
* **`db98135` / `824014f` (Conor catalog review):** the receipts claim a small
  catalog-only shrink (`824014f`: +1/-7, net -6) and guarded locate/CLI tests.
  The correction wire says it preserves mixed-source `return nil` behavior.
  This is a plausible narrow transplant, but the branch carrying it also
  contains 40+ unrelated commits; transplant the isolated commit and rerun
  source-ambiguity tests on the current base.
* **`54bf2b0` (Luna containers):** the worker receipt claims 71 production
  lines removed and 141 test lines removed, with focused race ×5 and the full
  `internal/index` race gate. The change is a candidate only: it needs a
  fresh review of refresh-cache lifetime and deletion semantics before use.

## Negative or unverified claims

* Luna 32-A’s final “package race passed” is contradicted by its own run log;
  the observed result is **FAIL**, not green.
* Luna 32-B’s final message says `CGO_ENABLED=0 go test -race -count=1
  ./internal/index/...` passed in 162.653s, but the lane’s recorded process
  evidence also shows other long-running package tests. Treat the receipt as
  **UNVERIFIED** until rerun in isolation.
* The reported 0/6 pulse is a supervisor/process-health failure, not a test
  result. It must not be converted into six successful worker outcomes.

## Bottom line

The sharpest failures are (1) a directly logged failing race/package run
followed by a false green receipt, (2) seven Conor worktrees that are mostly
the same 29–44-commit integration copy with repeated patch identities, and
(3) a deletion claim that removes 223 report-only lines while leaving the
production wave untouched. The only defensible transplants are isolated,
small commits after fresh gates; the supervisor’s aggregate branch cannot be
accepted as independent evidence.
