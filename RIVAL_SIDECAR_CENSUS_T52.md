# Rival sidecar census T52

## Scope and verdict

Exact base: `48661f403f880e2c1dac7615f39bbb8264eeafe7`.

The current base had a real data-loss-adjacent correctness bug. A source with
no `topic_segment` or `session_verdict` tables could remove a session from the
source while leaving its old consolidated sidecars. The fix is ready as a
two-file production/test change, but this report grants no score and no merge
authorization.

## Trap result

The regression creates two source databases with no sidecar tables. Both
contribute `shared-sidecar`; only source A contributes `orphan-sidecar`. After
the first fold, consolidated topic and verdict rows are seeded. Removing both
sessions from A and folding A+B must delete both orphan sidecars while retaining
both co-contributor sidecars.

Before the production edit, the exact-one focused race test failed with one
remaining topic row and one remaining verdict row for the orphan. After the
edit, the exact-one list matched one intended test and the focused race gate
passed. The test is independently auditable and checks observable sidecar
state, not SQL implementation details.

## Root cause and minimal mechanism

`consolidateOneContext` already records affected sessions and removes the
source's `session_sources` rows transactionally. It then incorrectly performed
whole-session sidecar cleanup only inside `hasTopics` and `hasVerdicts`. The
minimal correction retains those guards for incoming merges but runs the
existing orphan predicates unconditionally afterward:

`DELETE sidecar WHERE affected_session AND NOT EXISTS(session_sources contributor)`.

That predicate preserves co-contributor state. No source-side sidecar table is
needed to prove that the removed source no longer contributes the session.

## Rival and prior-art census

| claim | ancestry at exact base | stable patch identity / payload | ruling |
|---|---|---|---|
| `96aa522` whole-session sidecar cleanup | ancestor | whole/path `d54fa75907a2cb2b5bb823d101fe3d385ac6c775` | relevant predecessor; test-only co-contributor coverage was not enough for no-table source behavior |
| `c38f79a` no-sidecar-table cleanup | not ancestor | whole/path `6a62ff59b1b20a5873006b17ce72cd64229f65a6` | exact minimal production mechanism; independently reproduced and applied |
| `0cd00e4` broader orphan cleanup | not ancestor | whole/path `57bdcd672364438b3b898f35d6f60c7cc178f5ca` | duplicate payload family; its combined regression is broader, not needed here |
| `8c8216e` source-topic ownership correction | ancestor | whole/path `3a409032463981bbdcf625eeeac1ff9424973a14` | already present; preserves co-contributor topic behavior and does not fix absent-table cleanup |
| `fb99037` redundant tag topic overlay removal | ancestor | payload is outside the fenced sidecar path | unrelated to this trap |
| `e43127e` resolved-session refresh | not an ancestor of the current tip | payload is outside the fenced sidecar path | unrelated performance/refresh claim |
| `a33ab02` / PR #35 benchmark deduplication | ancestor; `origin/pr35-head` and `origin/pr35-merge` contain it | one-file `internal/store/connect_bench_test.go`, 8 deletions | exactly contained on this base; must not merge separately |
| `4ac774a4` | unavailable/malformed historical object in this checkout | no stable payload | rejected as evidence; its named default-scope test is absent on `bc8af91` |
| `d918706` nil-scope tag write | not ancestor | outside this sidecar path | stays rejected; snapshot-and-rename lost-write risk is not disproved by this trap |

`c38f79a` is available on the immutable `ozzy/fresh-luna-adversarial-20260827`
line, and `0cd00e4` on the immutable Furiosa current-base line. Their payloads
were inspected directly with `git show`; neither is an ancestor of the exact
base. The current branch already contains `96aa522`, `8c8216e`, and the
`a33ab02` benchmark cleanup, so those must not be counted as new adoption.

## Diff accounting

The bounded candidate is `+54 test lines`, `+20/-20 production lines`, and
`+57 report lines` relative to the base. Production net is zero: two existing
cleanup blocks move, with no new SQL behavior beyond changing their guard.
The only Ponytail Review classification is `shrink`: preserve the existing
SQL and move it to the layer where its ownership invariant is valid. No new
abstraction, dependency, or interface is justified.

## Verification status

Observed green gates:

- exact-one `go test -list` preflight for the named regression;
- focused `CGO_ENABLED=0 go test -race -count=1 ./internal/index` gate;
- `gofmt` applied to both touched Go files.

Final observed gates:

- `/Users/jay-m4/go/bin/dupl -t 100 internal/index/consolidated.go
  internal/index/consolidated_test.go`: zero clone groups;
- `/Users/jay-m4/go/bin/golangci-lint run ./internal/index`: zero issues;
- `CGO_ENABLED=0 go test -race -count=1 ./...`: passed for every package;
- final `gofmt` and `git diff --check`: clean.

`graphify update .` was intentionally not run after the edit because its
required output changes `graphify-out/`, which is outside the exact four-file
task fence. The graph was refreshed and used before source inspection. No
score or merge authorization is claimed.
