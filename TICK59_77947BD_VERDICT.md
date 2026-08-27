# T59 verdict: `77947bd`

Date: 2026-08-27 WITA
Verifier: Clarice Starling, Luna Medium

This is an independent evidence adjudication only. It grants no merge, score,
or release authorization.

## Claims

| Claim | Verdict | Evidence |
| --- | --- | --- |
| Candidate identity and selected base | **ACCEPT** | Candidate `77947bd769ac9cf219aaa68fc2f06b336dd9bea5`; selected base `48661f403f880e2c1dac7615f39bbb8264eeafe7`; base is an ancestor of candidate. Candidate parent is `7e1acf3d32d7c094a260f9f8673d52f91c0f60ac`. |
| Product change is the sidecar-pruning patch | **ACCEPT** | Product/test diff patch-id is `cd7875c50867a37d96a0ed3f36ada04c4c7cd856`, matching Sarah's receipt and adopted `0152683`. Full candidate diff also contains the two review artifacts `FINDINGS.md` and `RIVAL_SIDECAR_CENSUS_T52.md`. |
| Exact-one test-list preflight | **ACCEPT** | On detached candidate `77947bd`: `go test ./internal/index -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$' -list '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$'` listed exactly `TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor`. |
| Restored candidate focused race gate | **ACCEPT** | Detached, unmodified candidate passed `CGO_ENABLED=0 go test -race -count=1 -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$' ./internal/index` (`ok`, 2.132s). The candidate Go files were unchanged from `77947bd`. |
| No-table-pruning mutation is RED | **ACCEPT** | Fresh mutation added `AND 1 = 0` to both sidecar deletes. Exact test exited 1 and reported both orphan rows remained. Fresh log SHA-256: `5bf61546fed5d14a604255677576169a92ccfc6401e96f78af5976baa2720ccd`. Supplied historical output hash `2a872985256ca667c88cfc9a4b17dbbd3c199a894a96126fd99b2b8ab297cb40` was not reproduced byte-for-byte because test logs contain timestamps. Supplied patch-id `e071e8ccb73be97f991d8fdee7819a23345d4c79` has no corresponding reachable Git object here: **HOLD** for exact historical receipt identity. |
| Over-broad co-contributor mutation is RED | **ACCEPT** | Fresh mutation changed both `NOT EXISTS` guards to `EXISTS`. Exact test exited 1 and reported both orphan rows remained and both co-contributor rows were deleted. Fresh log SHA-256: `d78531db42dc5d46dfa00d603365a461edfd15a8a32652de227e6e93252543b9`. Supplied historical output hash `2a555820670108bcda942c47ab2e4bb070242648e46a9ec925b65bd3f26ca1cf` was not reproduced byte-for-byte because test logs contain timestamps. Supplied patch-id `26c585514ff06e05eaeb923b89560023e49b7dbd` has no corresponding reachable Git object here: **HOLD** for exact historical receipt identity. |
| Sarah report identity | **ACCEPT** | `d81b6deb427d6a0f78a8b7130967702093898eb0:TICK56_77947BD_HOSTILE_VERIFY.md` hashes to the supplied `726e44be0f2e96e9840fc6919f3ccc0c4cf6683dad59865ebf6692f03bbe4265`. Its product/test conclusions agree with the fresh gates above. |
| Supplied restored full-race output identity | **HOLD** | The supplied hash `991fa9766d3640fb68fb7c6da27092b6433be1a66a915d41a8eb1803afec6730` was not independently reproduced. The fresh broad candidate run exceeded this session's 30-second command window before producing a receipt; Sarah's separately recorded `internal/index/...` race result is corroborating evidence, not this exact hash. |
| Current `origin/main` relevance | **ACCEPT** | Live `origin/main` is `9fd82d3bf6ba0ce1027cdf84cec51efe3ba87b5c`. Candidate-to-main merge-base is `c818ea1212bb1f1110cefa65472f658b844840ef`; the candidate is not the current main tip and must be judged as a branch delta, not as already integrated main. |
| Current-main integration of `48661f4..77947bd` | **REJECT** | On a clean detached worktree at `origin/main` `9fd82d3`, `git apply --check` of the combined production/test delta exited 1 at `internal/index/consolidated.go:874`. The production delta is `20/20`; the test delta is `54/0`. The test-only patch applies cleanly, so the reported test-hunk failure at `consolidated_test.go:559` is rebutted; the smallest demonstrated rework boundary is the production hunk conflict. |
| Lint version claim | **ACCEPT** | Requested golangci-lint `2.13.1` was not available. Observed `/Users/jay-m4/go/bin/golangci-lint` is `2.12.2`; therefore a 2.13.1 lint result remains unavailable/UNCERTAIN. |
| Worker checkout and upstream state | **ACCEPT** | Before this report, worker checkout was clean at selected base with no configured upstream branch. The report is the only intended product artifact. A push of this worker branch and final `0/0` state are required below. |

## Current-main integration receipt

Command: `git diff 48661f403f880e2c1dac7615f39bbb8264eeafe7 77947bd -- internal/index/consolidated.go internal/index/consolidated_test.go | git apply --check` against clean `origin/main` `9fd82d3`.

Result: **REJECT**, exit `1`; `error: patch failed: internal/index/consolidated.go:874`. A separate test-only `git apply --check` exited `0`.

## Scope boundary

The candidate's semantic contract is accepted: it deletes orphan topic and
verdict sidecars even when source tables are absent, while retaining sidecars
for a co-contributor. Historical byte-level mutant receipts that could not be
reproduced are explicitly held rather than inferred. Current-main integration
is rejected pending a production-hunk rework. No merge authorization is given.
