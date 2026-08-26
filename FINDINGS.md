# Tick 26 focused-filter and patch-identity audit

## Verdict

The old focused command is false-green when the named test is absent: Go exits 0 and prints `[no tests to run]`. This happened for a near-miss regex and after temporarily renaming the exact test. A `go test -list` preflight that counts exact matches and requires one match rejects this case.

## Exact filter evidence

Target: `TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor`.

```text
$ go test ./internal/index -list 'TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor'
TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor
ok   github.com/MoonCaves/rawclaw/internal/index 0.455s

$ go test -race -count=1 ./internal/index -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$'
ok   github.com/MoonCaves/rawclaw/internal/index 1.724s
exit=0

$ go test -race -count=1 ./internal/index -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributo$'
ok   github.com/MoonCaves/rawclaw/internal/index 1.262s [no tests to run]
exit=0

$ go test -race -count=1 ./internal/index -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor_typo$'
ok   github.com/MoonCaves/rawclaw/internal/index 1.344s [no tests to run]
exit=0
```

Mechanical preflight (the final `test` is the rejection gate):

```sh
matches=$(go test ./internal/index -list "$filter" | awk '$1=="TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor"{n++} END{print n+0}')
test "$matches" -eq 1 || { echo "expected one exact test match, got $matches" >&2; exit 1; }
go test -race -count=1 ./internal/index -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$'
```

The exact filter produced `matches=1` and preflight exit 0. The near-miss produced `matches=0` and preflight exit 1.

## Temporary mutation

Only `internal/index/consolidated_test.go` was temporarily changed: the target function was renamed with `_MUTATED`. The unchanged old exact command then gave `ok ... [no tests to run]`, exit 0. The target was restored and the exact race-filtered test passed again.

Restoration checks:

```text
$ git diff --exit-code -- internal/index/consolidated_test.go
exit=0
$ shasum internal/index/consolidated_test.go
fc85a46cd349dc9da6506c2e4dbf3d390aca9d19
$ git show HEAD:internal/index/consolidated_test.go | shasum
fc85a46cd349dc9da6506c2e4dbf3d390aca9d19
$ go test -race -count=1 ./internal/index -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$'
ok   github.com/MoonCaves/rawclaw/internal/index 1.491s
exit=0
```

## Stable patch IDs

Commands:

```sh
git show --format= --no-ext-diff "$commit" | git patch-id --stable
git diff "$commit^" "$commit" -- internal/index/consolidated.go | git patch-id --stable
```

The first ID is whole-commit payload; the second is only `internal/index/consolidated.go` (the trailing all-zero field is the normal unused commit-ID field).

| commit | whole stable patch-id | consolidated.go stable patch-id | classification |
|---|---|---|---|
| c38f79a | 6a62ff59b1b20a5873006b17ce72cd64229f65a6 | 41b270da6a33147a5e89f959cf14cb2441128ddb | distinct source-without-sidecar-tables adaptation |
| 0cd00e4 | 57bdcd672364438b3b898f35d6f60c7cc178f5ca | ab5ee7d69f18a12786a85166f6dec53c32caedd6 | distinct orphan-sidecar adaptation |
| a78b39b | b47c5d83ef7a9a57b42f8a20f47c19f9ec4eb821 | ac2dbbbf06cdc226ade47b39b1e636a48f3cbdab | same production hunk; adapted test payload |
| 96aa522 | d54fa75907a2cb2b5bb823d101fe3d385ac6c775 | ac2dbbbf06cdc226ade47b39b1e636a48f3cbdab | same production hunk; adapted test payload |
| a62ab05 | b0c65fc6cfa0b3938d4a27b80108e609ba24fab5 | ac2dbbbf06cdc226ade47b39b1e636a48f3cbdab | same production hunk; adapted test payload |
| 0814bdc | d54fa75907a2cb2b5bb823d101fe3d385ac6c775 | ac2dbbbf06cdc226ade47b39b1e636a48f3cbdab | exact whole/path duplicate of 96aa522 |
| 5c0eefe | b7aaaee70fe88073287bb0fecc0c9b81beb80368 | 57c6d00909abd5096d81b94102fde342f1d38079 | exact whole/path duplicate of 537641b |
| 537641b | b7aaaee70fe88073287bb0fecc0c9b81beb80368 | 57c6d00909abd5096d81b94102fde342f1d38079 | exact whole/path duplicate of 5c0eefe |

The shared `ac2dbb...` path ID means production-hunk identity, not whole-commit equality: tests differ. `0814bdc`/`96aa522` and `5c0eefe`/`537641b` are actual whole-commit duplicates.

Ancestry-confused comparisons used:

```sh
git range-diff 9068aff..a78b39b fb99037..96aa522
git range-diff 9068aff..a62ab05 fb99037..0814bdc
git range-diff 593b16e..537641b f436f22..5c0eefe
```

The first showed only test adaptation around the same production hunk. The final showed `537641b` and `5c0eefe` as the same patch. The `a62ab05` versus `5c0eefe` range was not a duplicate and represents a distinct later topic-pruning change.

No product code was changed. The only committed file is this report.
