# T56 hostile verification: 77947bd

Date: 2026-08-27 (WITA)
Verifier: Sarah Connor, Luna-medium

## Verdict

`77947bd` is independently verified for the requested semantic contract. The
anchored exact-one test is green on the candidate, and both one-variable
hostile mutations are red. This is a verification receipt only; it grants no
score or merge authorization.

## Immutable identities

- Base and verifier HEAD: `77947bd769ac9cf219aaa68fc2f06b336dd9bea5`
- Adopted candidate: `0152683917b6bedc05ab550f55e06013ef49781b`
- Candidate parent: `bc1682071e3c9bb734c2783ee121f43002d814d0`
- Merge-base of `77947bd` and `0152683`: `48661f403f880e2c1dac7615f39bbb8264eeafe7`
- Stable product/test patch ID for both commits: `cd7875c50867a37d96a0ed3f36ada04c4c7cd856`
- `c38f79a` family patch ID: `6a62ff59b1b20a5873006b17ce72cd64229f65a6`
- `0cd00e4` family patch ID: `57bdcd672364438b3b898f35d6f60c7cc178f5ca`

The identical `77947bd`/`0152683` patch ID proves the production/test change
was adopted without a product or test novelty delta. The c38f79a and 0cd00e4
patch IDs differ. GitHub PR #40 head independently resolves to `0152683`; its
reported CI checks (build 1.24.0, stable build, lint) were successful.

## Focused semantic gate

Exact preflight before every focused run:

```text
go test ./internal/index -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$' -list '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$'
TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor
```

Baseline candidate: PASS under
`CGO_ENABLED=0 go test -race -count=1 -run '^TestConsolidate_PrunesSidecarsWithoutSourceTablesAndPreservesCoContributor$' ./internal/index`.

Mutation 1, suppressing both no-source-table sidecar deletes: RED, exit 1.
Raw test output SHA-256:
`0520b7997d685151c43ed56ee63047f5e49c83a1c238ce1173b44eff6dff34c`.
The exact test reported one orphan `topic_segment` row and one orphan
`session_verdict` row. Preflight output SHA-256:
`73717b7ec26e0867bc55da8f794137cb7dbf6b3830f97101600b66683b279c11`.

Mutation 2, changing both `NOT EXISTS` co-contributor guards to `EXISTS`: RED,
exit 1. Raw test output SHA-256:
`1970cd62d7480d07d53eb1d95e35651aeb287c2d2e4af7b4423c565a543f57b1`.
The exact test reported orphan rows remaining and both co-contributor rows
deleted. Preflight output SHA-256:
`126b5a1f3b3a513c6055072d7138e5ae87fcfeb4a657beb8572d84b910d07aa5`.

After each mutant, the verifier restored the Go files exactly. Restoration was
confirmed by zero diff in `internal/index/consolidated.go` and
`internal/index/consolidated_test.go`, followed by the exact-one preflight and
focused race test returning PASS.

## Broader gates

- `CGO_ENABLED=0 go test -race -count=1 ./internal/index/...`: PASS, 82.525s.
- `gofmt -w internal/index/consolidated.go internal/index/consolidated_test.go`:
  run before every dirty test; restored Go files have no diff.
- Requested golangci-lint v2.13.1: UNCERTAIN. The available binary is v2.12.2,
  so v2.13.1 was not substituted.

## Workspace and process evidence

The only remaining pre-report untracked artifact was the worker-owned stray
root `.cursor`, created by an initially mis-scoped mailbox helper invocation.
It was not supervisor state and was preserved during the audit. Its recorded
content was `20260827T044553Z-3c2d52d0-han-preserves-77947bd-provisio.md`, with
mtime `2026-08-27T12:47:05+0800` WITA (epoch `1787806025`). The dedicated
mailbox cursor was subsequently operated with an explicit
`AGENT_MAILBOX_DIR=/Users/jay-m4/code/rawclaw-furiosa-t56-77947bd-verifier/.agent-mailbox`.

No supervisor mailbox cursor was read or advanced. No source or existing test
file remains modified by this verification.
