# Hook Candidate Shootout

Audit date: 2026-08-26. Common reference: 0d1da19c4c21961b86cb3ca84ed047d941c83ed3.
Candidates:

- b0d9e0fc5890f653fb17aefa66917c5800a87f26, current lenny/raid-hooks-20260826
- c39872650a3ded47c7777e3ffad0ae3739b16f6b, product candidate

## Winner / hold verdict

Winner: b0d9e0f, conditionally. It is exactly c398726’s production implementation plus
a test consolidation: the production tree is identical, while b0 removes 83 test lines and
15 review-document lines. Both focused hook suites passed race detection three times.

Hold: do not claim the candidate is repository-wide green. Neither focused command runs
the complete CLI or repository suite. The b0 worktree is clean and its SHA is published on
origin/lenny/raid-hooks-20260826, but its local branch incorrectly tracks
origin/integrate/tagwrite-closeout-wave1 (7 ahead, 21 behind); fix that branch metadata
before using ahead/behind as release evidence.

## Exact diff and line accounting

merge-base(b0d9e0f, c398726) = c39872650a3ded47c7777e3ffad0ae3739b16f6b.

The only b0-vs-product changes are:

| category | b0 delta versus c398726 | interpretation |
|---|---:|---|
| production | 0 / 0 | identical setup.go behavior |
| tests, internal/cli/cmd_ingest_test.go | +12 / -95 = -83 | folds the standalone injected-directory test into the hostile matrix |
| docs, FINDINGS.md | +13 / -27 = -14 | merges findings 8 and 9; removes repeated review prose |

The product implementation c398726 changes setup.go at lines 82–115 and 165–198:
pre-check existing entry, create a private temporary directory, write the candidate file,
publish with ln "$tmp_entry" "$catalog_dir", clean up, then launch detached ingest only
for the winner or fail-soft path. The same implementation appears in b0.

The product test keeps a matrix at internal/cli/cmd_ingest_test.go:136–296 and a separate
directory-injection test at :298–388. b0 moves the injection into the matrix at
internal/cli/cmd_ingest_test.go:136–304, adding the injected-directory kind and retaining
the no-ingest, no-nested-artifact, and temp-cleanup assertions. This is a genuine
test-preserving shrink, not a production transplant.

Ponytail ruling: shrink — select b0’s test consolidation; it deletes duplicated harness
setup without deleting a covered case. No stdlib/native replacement is indicated.

## Behavior review

### POSIX sh and hostile paths — CONFIRMED PASS for covered cases

Both candidates use POSIX constructs ([ ], mkdir, ln, rmdir, nohup, redirections)
and the b0 matrix runs Claude/Codex under sh/dash across 36 combinations:
new, regular, FIFO, directory, injected-directory, symlink, dangling symlink, socket, and
missing parent. The test uses a 3-second command context and checks no hangs, no duplicate
ingest, no nested directory artifacts, and no leaked .tmp directories.

This does not cover a session_id containing /, .., or shell metacharacters. The code
uses the raw value in tmp_dir and tmp_entry names (setup.go:91–92 and 174–175), so arbitrary
IDs remain an unverified input-boundary risk. Hook providers normally supply opaque IDs;
classify this as PLAUSIBLE, not a reproduced defect.

### Executable resolution — UNVERIFIED in this shootout

Neither candidate changes the baked absolute-path resolver. The relevant hook invariant
therefore remains inherited from the common implementation, not newly proven by these
commits. The focused command does not exercise a missing interactive PATH with the
candidate’s full setup installation flow.

### Spawn/reap lifecycle — HOLD

The candidates launch nohup "$RAWCLAW" ingest "$session_id" </dev/null >/dev/null 2>&1 &
after the catalog claim. The candidate tests use a stub and wait for its log, but do not
wait on or inspect the actual detached child process. This proves dispatch and dedup, not
child reaping or ingest error visibility. No production code in either candidate adds a
wait/reap path; this is an explicit hold, not a claimed failure.

### Error visibility and idempotence — CONFIRMED for tested shell outcomes

The hook remains fail-soft: catalog setup and publication errors are swallowed, and the
ingest fallback is detached. Existing entries (including symlinks) return without launching
ingest. The b0 matrix asserts these outcomes for regular files, FIFOs, directories,
symlinks, sockets, and missing parents. It does not test an ingest process that exits
non-zero after winning the marker, so retry-after-ingest-failure remains outside this
shootout.

### Unnecessary test bulk / prior-art credit — CONFIRMED b0 improvement

The c398 product commit claims the matrix plus a separate injected-directory test, then
b0 claims the same 36 cases after folding the latter into the former. The b0 diff removes
the duplicate 95-line test body and merges the two review findings. It preserves the
injected-directory replacement at cmd_ingest_test.go:181–188 and assertions at :274–291.
This is the safe net-negative transplant.

Prior-art credit is not independent between candidates: b0’s FINDINGS.md explicitly
replaces c398’s separate findings with the combined basename-isolation ruling, and b0’s
parent is c398. Credit the production mechanism once to c398; credit the test shrink to b0.

## Personally observed gates

Product c398726, run in a temporary detached worktree (removed after the test):

    CGO_ENABLED=0 go test -race -count=3 ./internal/cli -run 'TestPrimeScripts_SessionStartHostilePathMatrix|TestPrimeScripts_SessionStartDirectoryInjectedBeforeLinkDeduplicatesWithoutNesting|TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest'
    ok github.com/MoonCaves/rawclaw/internal/cli 15.687s
    wall: 20.891s

B0d9e0f, in rawclaw-lenny-raid-hooks:

    CGO_ENABLED=0 go test -race -count=3 ./internal/cli -run 'TestPrimeScripts_SessionStartHostilePathMatrix|TestPrimeScripts_SessionStartDeduplicatesConcurrentIngest'
    ok github.com/MoonCaves/rawclaw/internal/cli 14.012s
    wall: 14.705s

Both returned exit code 0. No full CLI or ./... gate was run in this shootout. No Go files
were changed in the spy worktree; gofmt -w was therefore N/A.

## Scorecard

| candidate | production | tests/docs | gate | decision |
|---|---:|---:|---|---|
| b0d9e0f | same as c398 | -83 tests, -14 docs versus c398 | focused race count 3, 14.705s | WINNER, conditional |
| c398726 | baseline production fix | baseline tests/docs | focused race count 3, 20.891s | superseded by b0 shrink |

## Three sharp findings for the supervisor

1. b0 wins the shootout by deleting 83 test lines and 14 review lines while preserving
   c398’s identical production hook behavior and its injected-directory assertions.
2. The only personally observed gates are focused hook tests: b0 14.705s wall and c398
   20.891s wall; neither supports a full-suite green claim.
3. b0 is published, but its local branch tracks the wrong upstream (7 ahead, 21 behind);
   branch metadata must be corrected before treating its ahead/behind numbers as release
   evidence.
