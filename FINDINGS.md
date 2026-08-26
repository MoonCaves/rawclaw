# PA-CONSOLIDATED-SIDECAR-PRUNE-001 — Tick 26 selector mutation

## Verdict

**CONFIRMED.** The locked selector is behaviorally required. Two distinct
one-variable mutations both made the exact focused race test fail. No product or
test fix was made. The smallest missing assertion is none: the existing test
asserts both orphan deletion and co-contributor preservation for both sidecars.

## Preflight and baseline

Preflight before each mutation matched exactly one test:

```text
go test ./internal/index -list 'Consolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor'
TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor
```

Baseline focused race test: exit 0, PASS, real 2.73s.

```text
/usr/bin/time -p sh -c "CGO_ENABLED=0 go test -race -count=1 ./internal/index -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$' -v"
```

## Mutation 1 — broaden co-contributor deletion

Removed the final `topic_segment` deletion selector's
`NOT EXISTS main.session_sources` co-contributor guard.

Result: exit 1, FAIL, real 1.57s:

```text
consolidated_test.go:588: topic_segment co-contributor rows = 0, want 1
```

Restored byte-for-byte; focused rerun: exit 0, PASS, real 1.74s.

## Mutation 2 — skip session_verdict deletion

Removed only the final `session_verdict` deletion statement.

Result: exit 1, FAIL, real 1.55s:

```text
consolidated_test.go:585: session_verdict orphan rows = 1, want 0
```

Restored byte-for-byte; focused rerun: exit 0, PASS, real 1.73s.

An earlier exploratory source-topic-guard edit stayed green because the test
sources have no `topic_segment` table (`hasTopics == false`), making that edit
unreachable. It was an inert mutation, not evidence of a weak test; the two
direct selector mutations above both turned red.

## Immutable hashes

The test file was never edited. Hashes before mutation 1, after each restore,
and at final HEAD:

```text
internal/index/consolidated.go       ab2492f781ec62959fa189e4d38b7b3dee5a8347153b5490a479e8e9dd8e7676
internal/index/consolidated_test.go  c738d8be28b5972dc4684eb314ea3981de1f8f76d56815e8bd7aadc967d82665
```

Restored hashes matched the originals (`cmp` exit 0), and the source/test
diff was clean before this report update.
