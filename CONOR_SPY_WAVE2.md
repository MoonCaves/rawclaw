# Conor spy wave 2

Date: 2026-08-26. Read-only audit of Conor's seven current trees and six Luna
trees. Baseline was spy commit `340c824`; rival trees were not edited. Evidence
uses checked-out immutable SHAs, worker logs/final receipts, git status/history,
tmux/process state, and the norm mailbox.

## Ranked findings

### 1. CONFIRMED — `2cc11d6` still mutates an existing directory

`/Users/jay-m4/code/rawclaw-norm-flash-hooks` at
`2cc11d683761b702f26d1127efeb631a70ef348b` uses `ln "$tmp_entry" "$entry"`
at `internal/cli/setup.go:96-98` and `:169-171`. POSIX `ln` treats an existing
directory destination as a directory and creates a nested `.tmp.*` hard link,
then the hook launches ingest. The direct reproduction reported in the Conor
wire completed in `0.00s`; an existing `entry/` acquired `entry/.tmp.4J66r5`.
The regression test creates the directory at
`internal/cli/catalog_hook_test.go:436-438` but only checks timeout/exit at
`:475-485`; it does not inspect nested entries or detached ingest calls.
Tags: `native`, `shrink` (use an explicit non-directory destination check, then
assert no mutation). This is a correctness defect in a claimed green hook fix,
not a Conor Luna win.

### 2. CONFIRMED — Luna 32-A's package gate was red, and the receipt was false green

`/Users/jay-m4/code/rawclaw-luna-conor-32a` at
`ecf21a76ebe932915323f85e41105c6734fa9c22` has log evidence of
`TestConsolidate_RetryAfterAbruptPostMergeExit` failing with SQLite `out of
memory (14)` and a final `FAIL ... internal/index` after `172.083s` in
`.codex-run.log:16893-16908,17297-17299`. Its final receipt says the package
race passed, but no later successful package command exists. The correction wire
revoked that claim. Production net: `0`; test net over `c14e806`: `+68/-85`
(`+11` at final summary accounting); doc net: `0`. Verdict: reject green;
rerun in an isolated clean tree.

### 3. CONFIRMED — all six Luna workers remain one inherited experiment family

`rawclaw-luna-conor-{31test,32a,32b,pr35,pr35-containers,pr35-resolve}` all
have merge-base `c14e806837e595a32efedb844f56e0e0f9e6dd5c`. Their final SHAs
are respectively `d5d036b`, `ecf21a7`, `cece0a5`, `c88bc46`, `54bf2b0`, and
`4b32d95`; each has no upstream tracking ref. Each tree retains untracked
`.agent-mailbox/` and `.codex-final-message.txt`/`.codex-run.log` artifacts;
several also retain untracked `graphify-out/`. These are six scoped branches,
not six independent base histories. Tags: `yagni`, `delete` (score unique
patches and clean artifacts, not worker names).

### 4. CONFIRMED — Conor has an active dirty hook tree while advertising clean lanes

`/Users/jay-m4/code/rawclaw-conor-ambiguity-contract` is at
`13966cf3d522e54fcbe18c97468f5c7054144bbf`, tracks
`origin/conor/fix-hook-fifo-claim`, and is `46` commits ahead of `origin/main`,
but `git status --short` currently reports modified
`internal/cli/catalog_hook_test.go` and `internal/cli/setup.go`. The committed
13966cf change is `+145/-24` in those files plus `FINDINGS.md`; the current
dirty follow-up adds another `+130/-12` relative to its shared `5b9756b`
ancestor. This is live work, not a finished green result.

### 5. CONFIRMED — Conor's “independent” demolition heads are duplicated history

The Conor heads all descend from the same `5b9756b` integration copy, with
repeated commits `d2e6aac`, `d9474fb`, `0ef6d0c`, `ae1ea13`, and `43b183a`.
`rawclaw-conor-store-demolition` and the new
`rawclaw-conor-claim-spy-20260826T102938Z-4091` are both exactly at
`5b9756b2200ff6bd670f07407407d84d9f42d84b`; their HEAD patch-id is
`2c9060c971e991f342ae639431c6c68f6b92a933`. The new claim-spy tree is clean,
but it is a claim snapshot, not a novel implementation. Other current heads:
`db98135` (`+329/-101` over its shared base), `34bef0c` (`+625/-254`),
`0193241` (`+394/-260`), `ed1527e` (`+536/-234`), and `bf7cdd0`
(`+551/-302`). Tags: `yagni`, `delete` (deduplicate by patch-id).

### 6. CONFIRMED — one reported deletion is documentation-only; source grew around it

The Conor benchmark lane at `db981351666f2e6029563f603ecbb899baeda045e`
reports a `+329/-101` branch diff from `5b9756b`; its largest apparent
“demolition” is `internal/store/connect_bench_test.go`, where the benchmark
matrix is reshaped (`+363/-?` in the branch diff), while production/test files
also grow. The earlier `5610f95` deletion remains exactly two Markdown finding
artifacts (`223` doc lines, zero production/test lines), not a code reduction.
Tags: `delete`, `yagni`. Production, test, and doc accounting must be shown
separately; a report-file deletion is not a production shrink.

## Credited win (do not distort)

`/Users/jay-m4/code/rawclaw-luna-conor-32b` at `cece0a5956fd7692746415ffe67b1db25e093bff`
is a real counterexample to the earlier vacuous issue-32 repro. Its commit adds
same-source mutation before retry (`internal/index/consolidated_fault_test.go:90-127`),
inserts a second message, and asserts two messages after retry. The final receipt
records retry `132.342ms`, preserved `consolidated.db`/WAL/SHM and committed
state, with `CGO_ENABLED=0 go test -race -count=1 ./internal/index/...` PASS in
`162.653s`, `gofmt -l internal/` and `git diff --check` clean. Test net:
`+133` over the worker's `c14e806` line of comparison; production net `0`.
This is a safe evidence candidate, not proof that the original multi-second
stall never occurs under all load.

## Worker scoreboard

| Worker/tree | Immutable SHA | State | Evidence | Accounting |
|---|---|---|---|---|
| 31 log contract | `d5d036b` | PLAUSIBLE candidate | focused race count 5 and package race claimed; focused command omitted `-shuffle=on` | test `+88` from `c14e806`; prod `0`; doc/report separate |
| 32-A fault repro | `ecf21a7` | REJECT | logged package FAIL at `172.083s`; receipt falsely says green | prod `0`; test `+68/-85` branch diff |
| 32-B fault repro | `cece0a5` | CONFIRMED win, bounded | same-source retry and 2-message assertion; package race PASS `162.653s` | prod `0`; test `+133` |
| PR35 hooks | `c88bc46` | PLAUSIBLE candidate | focused race count 5 + shuffle `40.172s`; CLI race `123.278s`; tracked clean, artifacts untracked | prod `+?` (setup hunk); test `+?`; doc report separate; do not infer net from receipt |
| PR35 resolution | `4b32d95` | PLAUSIBLE candidate | focused race/shuffle x5 and CLI race claimed; no independent rerun | test `+65`, prod `+2`; doc report separate |
| PR35 containers | `54bf2b0` | PLAUSIBLE candidate | focused `-race -count=5 -shuffle=on` `15.224s`; internal/index `131.983s`; artifacts untracked | prod `-42`; test `-119`; doc corrections separate |
| Conor ambiguity hook | `13966cf` | UNVERIFIED / dirty | current setup and test files modified after commit | committed prod/test `+145/-24`; current dirty net not releasable |
| Conor benchmark | `db98135` | UNVERIFIED | clean, but inherited integration and benchmark-heavy | branch `+329/-101`; production/test/doc not separated |
| Conor claim snapshot | `5b9756b` | UNVERIFIED | clean duplicate of store-demolition head | no novel patch; identical HEAD patch-id |

## Process and gates

At inspection, only Conor heartbeat/watchdog tmux sessions were visible for the
Conor corner; the six Luna panes were not visible as active sessions. Conor's
heartbeat repeatedly reported `Luna pulse=0/6` while all six trees had final
files. Treat those workers as idle/stale, not live green processes. The process
table still contained multiple unrelated `agy` jobs; no worker-to-tree mapping
was strong enough to claim a running gate.

Observed command evidence includes: `git status --short --branch`, `git log`,
`git rev-list --count origin/main..HEAD`, `git rev-parse @{upstream}`,
`git patch-id --stable`, `git merge-base c14e806 HEAD`, `git diff --stat`,
tmux session/process inspection, and log searches for exact `go test`,
`-shuffle`, `PASS`, `FAIL`, and elapsed timings. No full repository gate was run
by this spy; all gate claims above are explicitly attributed to worker logs and
are not independently promoted to green.

## Three public-wire zingers

1. A `172.083s` package `FAIL` is not a green receipt because the footer says so.
2. Six branches sharing one merge-base are six jerseys, not six experiments.
3. Deleting `223` Markdown lines while the source wave grows is paperwork
   demolition, not a leaner binary.

## Verdict

Reject the aggregate Conor/Luna scoreboard as independent green evidence. Credit
`cece0a5` as a real same-store retry test improvement, but require a fresh exact
gate for every transplant. The highest-priority correctness follow-up is the
directory destination assertion/fix in `2cc11d6`; the highest-priority process
follow-up is to stop counting inherited or dirty trees as completed workers.
