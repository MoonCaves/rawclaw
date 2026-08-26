# Furiosa Tick 22: terminal publication mutation

## Verdict

**CLEAN RULING: retain candidate `34d2fb05161b1be7819b80804fca2e3a576243cf`.**
The bounded matrix killed both distinct terminal-publication mutations. No
product correction is indicated. The contract remains explicitly best effort;
these tests do not prove durable terminal ownership.

## Candidate and filter proof

- Worktree branch: `worker/furiosa-mutation-t22-20260827`, based exactly on
  `34d2fb0` (`34d2fb0` parent is `878f631`; `878f631` is an ancestor).
- Baseline command:
  `CGO_ENABLED=0 go test -race -count=1 -v ./internal/cli -run '^(TestTagWriteQueuesDerivedPublication|TestTagWriteAuthoritativeOverlaySurvivesDelayedPublication|TestRunTagPublishChildHonorsContextTimeout)$'`
- The `-v` output enumerated and executed all three named tests; all passed.
  Baseline wall time: 4.933s (package reported 2.245s).
- The queue test's seam assertion exercises the foreground handoff. The
  delayed-publication test exercises authoritative visibility while the child
  is held, and the child test exercises cancellation at the real fence.

## Mutation matrix

| Mutation | Disposable change | Exact killing gate | Result |
|---|---|---|---|
| M1: suppress handoff | Replace `spawnTagPublish(dbp)` with a no-op function returning nil in `internal/cli/cmd_tag.go` | `... -run '^TestTagWriteQueuesDerivedPublication$'` | **RED**, 2.247s wall: `tag-write did not request detached derived publication` |
| M2: synchronous publication | Replace `spawnTagPublish(dbp)` with `runTagPublishChild(context.Background(), io.Discard, dbp)` | `... -run '^TestTagWriteQueuesDerivedPublication$'` | **RED**, 2.258s wall: `tag-write did not request detached derived publication` |

M1 is the no-publication/process-interleaving failure. M2 reconnects the
foreground command directly to a blocking publication path and bypasses the
detached seam. Both mutations reached observable contract code and were not
synthetic unreachable edits.

## Restore proof

Restored `internal/cli/cmd_tag.go` byte-for-byte to candidate content. The same
three-test exact race filter then passed: package reported 1.971s, wall 2.364s.
`git diff --exit-code -- internal/cli/tagrefresh.go internal/cli/tagrefresh_test.go internal/cli/cmd_tag.go` returned 0.

## Duplicate/convergence audit

Whole-commit stable patch IDs:

- `3b641ce`: `b2280fc0a2baaa89474730e0a9c22128dab10b4e`
- `96aa522`: `d54fa75907a2cb2b5bb823d101fe3d385ac6c775`
- `c38f79a`: `6a62ff59b1b20a5873006b17ce72cd64229f65a6`
- `fb99037`: `172b017850112fba5c5a4d9d1a8e735c964789a2`
- `88e73b5`: `73f5dd69a25ee9f6e39bcd2036397b46661d741b`
- `878f631`: `b2d5b3e2afbfef8ef404e95e356b38f5d8d35bcc`
- candidate `34d2fb0`: `21194acb3fe3836226a00e72273de0a5bf47ac0f`

Path-scoped IDs show `3b641ce` and `878f631` are identical on both
`internal/cli/cmd_tag.go` (`f50ccbe88fa29f9e0d88ce98c42e8944e4a9a545`) and
`internal/cli/tagrefresh_test.go`
(`bdc8e8f91f1d50f0e67c1ac6703cafb0f429b686`). `88e73b5` is distinct and
solely removes overlay bookkeeping. `fb99037` is distinct tag-refresh overlay
work; `96aa522` and `c38f79a` are distinct consolidated pruning fixes. The
candidate is those product commits plus its documentation-only report commit,
not a new convergent product patch. `git range-diff 3b641ce^..3b641ce
878f631^..878f631` maps 1:1, confirming convergence of that prior contract
patch.

## Gates and remaining risk

- `git diff --check`: PASS.
- `gofmt -l internal/`: PASS (no output; product files unchanged).
- Interruption/process test: not added; existing `TestRunTagPublishChildHonorsContextTimeout` was exercised.
- No durable terminal ownership is claimed; a detached child can still be
  absent if the process environment disappears before start, as documented by
  the candidate.
