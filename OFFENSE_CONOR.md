# Hostile evidence audit: Conor's six advertised heads

Date: 2026-08-26  
Base: `5b9756b2200ff6bd670f07407407d84d9f42d84b`  
Scope: immutable advertised heads only; product code was not edited.

## Verdict

Three confirmed deductions are supported by exact receipts: `ecf21a7` has a
logged failing package run followed by a green claim; `4b32d95` is patch-
identical to `bf7cdd0`; and `54bf2b0` is patch-identical to integrated
`21ece6f`. `d5d036b` is narrowed because it deletes coverage and its receipt
omits shuffle. `cece0a5` is a bounded, observed green test improvement.
`c88bc46` is no deduction: its production hunk is whitespace normalization
and the focused gate passed.

## Exact identity and accounting

All patch IDs below are from `git show <sha> --pretty=format: | git patch-id
--stable`. Commit-local line accounting is `git diff --numstat <sha>^ <sha>`;
production means non-test, non-`FINDINGS*` paths.

| advertised head | parent | patch-id | changed paths and commit-local lines | result |
|---|---|---|---|---|
| `d5d036b9dd94c59a9ee3da2da8fb8d1039cb671d` | `2bb219f8aeb412dbf9add6fe691cf606ad8805f1` | `804bbd4fb74175854b4a824ff154b4b5724e62f6` | `internal/index/consolidated_logging_test.go` `0/+57` (net test `-57`) | NARROWED |
| `ecf21a76ebe932915323f85e41105c6734fa9c22` | `8947c217e1c9c980d5956159c38583fce23bfe9a` | `4d46c60d767a0b989aa8f566cf2fde56e87cb569` | `consolidated_fault_test.go` `0/-85`; `consolidated_test.go` `+65/0`; `leak_test.go` `+3/0` (production `0`, test net `-17`) | CONFIRMED DEDUCTION |
| `cece0a5956fd7692746415ffe67b1db25e093bff` | `da40a8e9f4016cc52a2b26e22d8f7700a73ebd0d` | `873b0363760decf8f302d7547dd1554276b2a9aa` | `internal/index/consolidated_fault_test.go` `+28/-2` (production `0`, test net `+26`) | NO DEDUCTION |
| `c88bc4664c4050082abfa635ee8b7600107b2e1f` | `1c9c48a932569dfad60263bfb6cef002e5d17012` | `3b8aa7132b22000184faf2134882efdd9ca1948c` | `cmd_ingest_test.go` `+3/-4`; `setup.go` `+14/-14` (production net `0`, test net `-1`) | NO DEDUCTION |
| `4b32d95e04fc8fc093d9ad1a1445e88a5a780727` | `8dfa1ca95cc4fb719ee07b2135fc10814740230d` | `c943b45299eb099ee864b4798e6b11d5d1988086` | `internal/cli/cmd_tag_onestore_test.go` `+65/-0` (production `0`, test net `+65`) | CONFIRMED DEDUCTION |
| `54bf2b03d3b32bf639924ff0a1f8f6885772eb81` | `2a97a7d582b8591e1c87dc23a3b7651392edbb69` | `d7c22ba9b5bf9b41eb8b473bd1e48227e4fe3a28` | `internal/index/containers.go` `0/-42`; `containers_test.go` `0/-119` (production `-42`, test `-119`) | CONFIRMED DEDUCTION |

Identity command/output:

```text
d5d036b... 804bbd4fb74175854b4a824ff154b4b5724e62f6
ecf21a7... 4d46c60d767a0b989aa8f566cf2fde56e87cb569
cece0a5... 873b0363760decf8f302d7547dd1554276b2a9aa
c88bc46... 3b8aa7132b22000184faf2134882efdd9ca1948c
4b32d95... c943b45299eb099ee864b4798e6b11d5d1988086
54bf2b0... d7c22ba9b5bf9b41eb8b473bd1e48227e4fe3a28
```

The assigned worktree has exact refs for `d5d036b`, `cece0a5`, and `4b32d95`;
the other three were resolved and inspected in their named standalone clones.
`git merge-base --is-ancestor <head> 5b9756b` returns `1` for all six.

## Range-diff and duplicate receipts

Against the current base, the 31-log candidate is not part of base:

```text
$ git range-diff 5b9756b^..5b9756b 2bb219f^..d5d036b
1: 5b9756b < -: ------- refactor(agentproto): inline catalog project filter
-: ------- > 1: 2bb219f test(index): capture consolidated phase timing logs
-: ------- > 2: d5d036b test(index): keep failed fence timing proof
```

The two issue-32 candidates are distinct sibling patches, not duplicates:

```text
$ git range-diff c14e806..cece0a5 c14e806..8947c21
1: da40a8e < -: ------- test(index): verify issue 32 kill retry state
2: cece0a5 < -: ------- test(index): make issue 32 retry perform merge work
-: ------- > 1: 8947c21 test(index): isolate consolidate fault repro
```

The resolution test is an exact duplicate of `bf7cdd0`, including patch ID:

```text
$ git show 4b32d95 --pretty=format: | git patch-id --stable
c943b45299eb099ee864b4798e6b11d5d1988086
$ git show bf7cdd0 --pretty=format: | git patch-id --stable
c943b45299eb099ee864b4798e6b11d5d1988086
```

The full-tree diff is not expected to be empty because the two commits have
different parents; patch ID is the relevant normalized-diff comparison.

The container deletion is an exact duplicate of `21ece6f`, already in the
base history:

```text
$ git show 54bf2b0 --pretty=format: | git patch-id --stable
d7c22ba9b5bf9b41eb8b473bd1e48227e4fe3a28
$ git show 21ece6f --pretty=format: | git patch-id --stable
d7c22ba9b5bf9b41eb8b473bd1e48227e4fe3a28
$ git merge-base --is-ancestor 21ece6f 5b9756b; echo $?
0
```

No patch-identical sibling was found for `d5d036b`, `ecf21a7`, `cece0a5`, or
`c88bc46` in the inspected candidate refs. The advertised `ecf21a7` and
`c88bc46` objects are absent from the assigned repository, so their base
comparison was performed from the named standalone clones; this is a receipt
boundary, not an invented ancestry result.

## Findings and reproduced gates

### 1. CONFIRMED DEDUCTION — `ecf21a7` receipt says green after a logged failure

The worker's own run log records the focused package failure:

```text
/Users/jay-m4/code/rawclaw-luna-conor-32a/.codex-run.log:16893
--- FAIL: TestConsolidate_RetryAfterAbruptPostMergeExit (0.58s)
/Users/jay-m4/code/rawclaw-luna-conor-32a/.codex-run.log:16908
query ... unable to open database file: out of memory (14)
/Users/jay-m4/code/rawclaw-luna-conor-32a/.codex-run.log:17297-17299
FAIL
FAIL github.com/MoonCaves/rawclaw/internal/index 172.083s
FAIL
```

The same log later contains the receipt claim `package race ... passed`
(`.codex-run.log:17302`) and a second pass from a later invocation. The first
failure is real evidence that the advertised gate did not establish a clean
green result; the later pass does not erase it. The commit also deletes the
standalone 85-line fault test and moves it into `consolidated_test.go`, so the
reported `+11` branch-line claim is not commit-local accounting.

### 2. CONFIRMED DEDUCTION — `4b32d95` is duplicate work

The exact patch ID and empty `git diff --stat bf7cdd0 4b32d95` above prove the
65-line test was already committed as `bf7cdd0`. It adds no novel source or
test behavior relative to that sibling.

### 3. CONFIRMED DEDUCTION — `54bf2b0` is duplicate work already in base

The exact patch ID and ancestor check above prove the 42-line production and
119-line test deletion already landed as `21ece6f` in base. Counting this head
as a fresh container result double-counts an existing change.

### `d5d036b`: NARROWED, not a clean novel win

The patch deletes the 57-line `TestConsolidate_LogsFoldPhaseStartsAndDurations`
and retains only the timeout test. The retained focused command was observed:

```text
$ CGO_ENABLED=0 go test -race -count=1 ./internal/index -run TestConsolidatedFence_LogsAcquireDurationOnTimeout
ok github.com/MoonCaves/rawclaw/internal/index 1.763s
```

That proves the retained test runs, not that the deleted nine-phase fold-log
contract is covered elsewhere by this head. The worker's focused receipt did
not include `-shuffle=on`, so shuffle coverage is UNVERIFIED.

### `cece0a5`: NO DEDUCTION, bounded positive evidence

This is a real test strengthening: it mutates the same source after the
fault-injected child and asserts two messages after retry (`consolidated_fault_test.go:90-127`). Reproduction was observed:

```text
$ go test -race -count=1 ./internal/index -run TestConsolidate_KillThenRetryLeavesUsableStore -v
--- PASS: TestConsolidate_KillThenRetryLeavesUsableStore (0.07s)
issue #32 reproduction: does not reproduce the multi-second stall
PASS
ok github.com/MoonCaves/rawclaw/internal/index 0.718s
```

The gate covers retry merge work and message preservation. It does not prove
the original multi-second stall impossible under all contention, so the claim
is bounded to the assertion actually present.

### `c88bc46`: NO DEDUCTION

The `setup.go` hunk changes indentation and brace placement only; shell control
flow remains the same `if [ -f "$entry" ]; then exit 0; fi` before the detached
ingest. The test hunk shortens a polling timeout and keeps the same assertion.
The focused shuffled race gate was observed:

```text
$ go test -race -count=1 -shuffle=on ./internal/cli -run 'TestPrimeScripts_StopLaunchDetachedPrewarm|TestClaudePrimeScript_CreatesSessionCatalogEntry|TestCodexPrimeScript_CreatesSessionCatalogEntry_FullPayload'
ok github.com/MoonCaves/rawclaw/internal/cli 3.060s
```

The supplied diff does not prove the previously alleged duplicate-ingest bug;
that allegation is rejected for this exact head.

## Dirty-state receipt boundary

Each standalone clone was checked with `git status --short --branch`; each
reported its named branch plus an untracked `.agent-mailbox/` directory. This
is worker/runtime residue, not a product-path modification, but it means a
claimed clean worker tree was not independently clean at audit time. The
assigned report worktree remained clean except for this report.

## Final scoring rows

| head | classification | proved basis |
|---|---|---|
| `d5d036b9dd94` | NARROWED | deletes 57-line fold-log coverage; retained timeout gate green; shuffle omitted |
| `ecf21a76ebe9` | CONFIRMED DEDUCTION | own run log has `FAIL` and `out of memory (14)` before green receipt |
| `cece0a5956fd` | NO DEDUCTION | same-source retry mutation and two-message assertion pass |
| `c88bc4664c40` | NO DEDUCTION | semantic shell behavior unchanged; focused shuffled race gate pass |
| `4b32d95e04fc` | CONFIRMED DEDUCTION | exact patch-id duplicate of `bf7cdd0` |
| `54bf2b03d3b3` | CONFIRMED DEDUCTION | exact patch-id duplicate of base ancestor `21ece6f` |
