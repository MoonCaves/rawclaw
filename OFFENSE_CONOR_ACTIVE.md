# Hostile evidence audit: Conor active heads

Date: 2026-08-26  
Audit worktree base: `5b9756b2200ff6bd670f07407407d84d9f42d84b`  
Scope: `aece813d81362be3a19801b544eab2ff82b697a1`,
`db981351666f2e6029563f603ecbb899baeda045`,
`0193241b6ce317ec0c931e6160b8e82b21f48161`, and
`ed1527ef8c8d7f8386b4908ef843fb9416535886`. Product and test files were not
edited.

## Verdict

Three heads are confirmed duplicate attribution already present in the audit
base: the benchmark refactor, phase-logger refactor, and failed-publish test.
The hook refactor is also an exact duplicate already in base, and its exact
script retains a confirmed path-traversal file-creation behavior because raw
`session_id` is interpolated into `entry` and `tmp_entry`. The container test
is bounded positive evidence only; it proves one injected publish failure leaves
one refresh row, not general retry correctness. No full-suite green claim was
made.

## Identity and accounting

Commit-local patch IDs (`git show <sha> --format= | git patch-id --stable`) and
line accounting (`git diff --numstat <sha>^ <sha>`):

| head | parent | patch-id | changed path / lines | result |
|---|---|---|---|---|
| `aece813d81362be3a19801b544eab2ff82b697a1` | `e19b80e324fc1b459d2f4d610602e9f58630fc4a` | `52f78fbdeb17bc2611f5aa3d32d151cf2749d5e2` | `FINDINGS.md` `+5/-2` | duplicate receipt only |
| `db981351666f2e6029563f603ecbb899baeda045` | `43b183a193d5228b47c5403dc48cb4ef5c284ef5` | `1b816cccb39367bc899903e01ce1cc3f3cee865e` | `FINDINGS.md` `+2/-3` | duplicate receipt only |
| `0193241b6ce317ec0c931e6160b8e82b21f48161` | `ed368fe687ba4579a592c0eb8ae491bad94253de` | `2aecad9542d47c189738a422ebccedbe0736a920` | `internal/index/containers_test.go` `+57/-0` | duplicate test |
| `ed1527ef8c8d7f8386b4908ef843fb9416535886` | `821b78d02f2227264cb7630933147775bf71c142` | `aac381e6408a359036ce248a73a04053b188fe64` | `internal/cli/setup.go` `+6/-20` | duplicate plus unsafe path handling |

The first two heads only revise audit artifacts; their product commits are
their parents (`e19b80e`, `43b183a`).

## Duplicate receipts

The benchmark product patch is already base commit `8e0dc0e7622b30eac5d60009ad4eb7eda653837d`:

```text
$ git show e19b80e --format= | git patch-id --stable
e329cf14aa2bbe6eee6fe1cccff791a7222561cf
$ git show 8e0dc0e --format= | git patch-id --stable
e329cf14aa2bbe6eee6fe1cccff791a7222561cf
$ git merge-base --is-ancestor 8e0dc0e 5b9756b; echo $?
0
```

The normalized source trees are equal (`internal/store/connect_bench_test.go`);
the advertised head adds only its `FINDINGS.md` receipt. The benchmark receipt
therefore cannot count a new product shrink.

The phase-logger product patch is already base commit `ae1ea13834e4c5b28d7273d48c008ef655e9b233`:

```text
$ git show 43b183a --format= | git patch-id --stable
5a8fb1f70d817f89e253153a1f015adcec2b345a
$ git show ae1ea13 --format= | git patch-id --stable
5a8fb1f70d817f89e253153a1f015adcec2b345a
$ git merge-base --is-ancestor ae1ea13 dd32c30; echo $?
0
```

The container test is already base commit `8824e256066518a685e685aa70eb2ed59019dfc`:

```text
$ git show 0193241 --format= | git patch-id --stable
2aecad9542d47c189738a422ebccedbe0736a920
$ git show 8824e25 --format= | git patch-id --stable
2aecad9542d47c189738a422ebccedbe0736a920
$ git merge-base --is-ancestor 8824e25 5b9756b; echo $?
0
```

The hook refactor is already base commit `d9474fb6cc076e55a13a2116705c225ca3b9f2a8`:

```text
$ git show ed1527ef --format= | git patch-id --stable
aac381e6408a359036ce248a73a04053b188fe64
$ git show d9474fb --format= | git patch-id --stable
aac381e6408a359036ce248a73a04053b188fe64
$ git merge-base --is-ancestor d9474fb dd32c30; echo $?
0
```

The direct source comparisons for all four product paths are byte-identical
to the cited prior/base versions. `git range-diff 821b78d..ed1527ef
d9474fb^..d9474fb` reports `ed1527e = d9474fb`.

## Confirmed unsafe semantics — raw session ID escapes catalog path

**Head:** `ed1527ef8c8d7f8386b4908ef843fb9416535886`  
**Locations:** `internal/cli/setup.go:48`, `:63-64`, `:72`, and the duplicate
Codex block at `:137`, `:150-151`, `:159` in that immutable head.

The generated POSIX hooks parse an unvalidated JSON `session_id`, then build
filesystem paths directly:

```sh
session_id=...          # copied from hook JSON
entry="$catalog_dir/$session_id"
tmp_entry="$catalog_dir/.tmp.$session_id.$$"
```

An ID containing `../` therefore escapes `catalog_dir`. This is reproducible
with the exact claim expression, without touching any product file:

```text
$ tmpdir=$(mktemp -d); mkdir -p "$tmpdir/catalog" "$tmpdir/victim"
$ catalog_dir="$tmpdir/catalog"; session_id='../victim/pwn'
$ entry="$catalog_dir/$session_id"
$ (set -C; : > "$entry") 2>/dev/null || [ ! -e "$entry" ]
claimed=1
$ test -f "$tmpdir/victim/pwn" && echo traversal_file_created=yes
traversal_file_created=yes
```

Thus a malformed or hostile hook payload can create an empty file outside the
catalog root, and the later temporary-path interpolation has the same path
construction hazard. This is a confirmed filesystem-integrity/availability
finding for the advertised head, not an inference from the commit message.
The later `c39872650a3ded47c7777e3ffad0ae3739b16f6b` hard-link claim isolates
the candidate path; that fix is not present in `ed1527ef`.

## Bounded focused gates

These were independently run from this audit worktree. They are package-focused
race checks, not evidence of a full repository green run:

```text
$ CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestEnsureFreshContainer_PreservesRefreshDBOnPublishFailure$'
ok github.com/MoonCaves/rawclaw/internal/index 2.184s

$ CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'TestPrimeScripts_(SessionStartIngestsWhenCatalogUnavailable|SessionStartDeduplicatesConcurrentIngest)|TestClaudePrimeScript_CreatesSessionCatalogEntry|TestCodexPrimeScript_CreatesSessionCatalogEntry_FullPayload'
ok github.com/MoonCaves/rawclaw/internal/cli 4.121s

$ CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'TestConsolidate_LogsPhaseStartsAndDurations|TestConsolidatedFence'
ok github.com/MoonCaves/rawclaw/internal/index 2.036s

$ CGO_ENABLED=0 go test -race -count=1 ./internal/store -run '^$'
ok github.com/MoonCaves/rawclaw/internal/store 1.484s [no tests to run]
```

The focused container test seeds a refresh db, injects a consolidated-store
trigger, observes `EnsureFreshContainer` fail, and checks one refresh message
remains readable. It does not assert that the failed session is absent from the
consolidated store or that a subsequent retry succeeds; classification is
therefore **NO DEDUCTION / bounded evidence**, not a broad correctness claim.

## Final scoring

| head | classification | proved basis |
|---|---|---|
| `aece813d8136` | **CONFIRMED DUPLICATE** | benchmark product patch matches base `8e0dc0e`; head only edits `FINDINGS.md` |
| `db981351666f` | **CONFIRMED DUPLICATE** | phase-logger product patch matches base `ae1ea13`; head only edits `FINDINGS.md` |
| `0193241b6ce3` | **CONFIRMED DUPLICATE / NO NEW BUG** | exact test patch matches base `8824e25`; focused test passes with bounded scope |
| `ed1527ef8c8d` | **CONFIRMED DUPLICATE + CONFIRMED UNSAFE** | exact patch matches base `d9474fb`; raw IDs escape catalog path at both hook blocks |

No full-suite result was inferred from these focused passes.
