# Candidate 50c6d0d assertion mutation wave 3

Verdict: HOLD. The candidate's streamlined ingest test is false-green for a cache-isolation regression. A retained integrated journey assertion kills the same minimal mutant.

## Deleted assertions inventoried

- `internal/cli/cmd_ingest_test.go:268-271` in parent `50c6d0d^`: deleted `store.CacheDir()` containment check requiring the resolved cache directory to remain under the per-test `HOME`.
- `internal/cli/cmd_ingest_test.go:308-310` in parent `50c6d0d^`: deleted output contract requiring both `Ingested session` and `2 messages` in the ingest command output.

The candidate replacement at `internal/cli/cmd_ingest_test.go:295-302` centralizes setup and at `:304-350` retains database/search assertions, but contains neither deleted assertion.

## Mutation

Disposable worktree: `/tmp/rawclaw-50c6d0d-assertion-mutant`, detached at `50c6d0d`; candidate worktree was not modified.

- `internal/store/store.go:283`: changed `return filepath.Join(home, ".cache")` to `return filepath.Join(os.TempDir(), "rawclaw-mutant-cache")`.
- This violates the deleted cache-isolation contract while keeping the candidate's streamlined test body unchanged. The temporary Go mutation was formatted with `gofmt -w` and was not committed.

## Results

- SURVIVED candidate test: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^TestIngestCmd_IndexesFreshSession_EndToEnd$'` passed in 1.808s test time / 4.72s wall time.
- SURVIVED retained candidate ingest set: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^Test(IngestCmd_(IndexesFreshSession_EndToEnd|Idempotent_RepeatedRunIsNoOp|ConcurrentRuns)|ClaudePrimeScript_ExecutesDetachedIngest)$'` passed in 2.508s test time / 2.91s wall time.
- KILLED retained integrated assertion: `CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run '^TestCLIJourney_AntigravityEndToEnd$'` failed at `internal/cli/cmd_journey_test.go:38` in 0.00s test time / 0.73s wall time: resolved cache was `/var/folders/.../T/rawclaw-mutant-cache/session-search`, outside the test temp directory.

The stdout assertion was also deleted at `internal/cli/cmd_ingest_test.go:308-310`; no separate stdout mutation was needed because the cache mutant already demonstrates false-green coverage loss. Existing hook/integration tests retain independent banner checks, but they do not restore the deleted ingest-output contract.

Patch accounting for `internal/cli/cmd_ingest_test.go` versus parent: 47 insertions, 103 deletions, net `-56` lines in this file (commit stat including the report's earlier bookkeeping differed). The candidate commit reports 76 insertions and 130 deletions across its two files, with zero production-line changes.

net: -56 lines safely removable.
