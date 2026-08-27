# Tick 45: stress audit of Ozzy 386ec9d

## Verdict

The speed claim is **REBUTTED as universal**, but the earlier Furiosa report's
blanket regression claim is not reproduced across workloads. The semantic path
is confirmed. Score disposition is **UNCERTAIN / no point**: the live ledger
requires external withdrawal/adoption or an actual stopped merge, neither of
which was observed.

## Identity

- Base: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`
- Candidate: `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21`
- Candidate parent equals the base.
- Whole patch-id: `356c1cb3878d142f910494843358b2737554dace`
- Product path patch-id: `6b42e87e9d75eccc8a5527faa6c001653c15be82`
- Benchmark path patch-id: `ca293434ea17286206af6c8a8c97b00e393f1c4c`
- `git range-diff 0d1da19..386ec9d 0d1da19..386ec9d`: exact self-match.
- Product delta: +35/-36 lines; committed benchmark: +77 lines.

## Benchmark method and observed result

I copied the committed benchmark into the disposable baseline worktree and
parameterized that disposable test for six same-schema cases:

| case | missing | existing |
|---|---:|---:|
| small_missing | 10 | 0 |
| medium_missing | 100 | 0 |
| large_missing | 600 | 0 |
| small_mixed | 10 | 5 |
| medium_mixed | 100 | 50 |
| large_mixed | 600 | 200 |

Each side ran 10 samples with `-benchtime=1x -benchmem -count=10`, in
alternating order (base A, candidate A, candidate B, base B). Reset and
reseed occurred outside timing. Every output reported Go 1.26.3, darwin/arm64,
Apple M4, 10 CPUs. `benchstat` was run for both A and B pairs.

Representative A-pair candidate deltas versus base:

| case | ns/op | B/op | allocs/op |
|---|---:|---:|---:|
| small missing | +332% | +556% | -7.7% |
| medium missing | +28.7% | +965% | -35.4% |
| 600 missing | +48.7% | +1008% | -38.1% |
| small mixed | -44.5% | +110% | -62.4% |
| medium mixed | -70.3% | +109% | -72.0% |
| 600 mixed | -48.0% | +161% | -68.5% |

All deltas were `p=0.000, n=10`. The reverse pair was consistent: 600-missing
`+46.8%`, medium-missing `+27.0%`, and 600-mixed `-46.8%`. Missing-only
bytes/op were about 8.9x-10.0x higher; mixed bytes/op about 2.1x-2.6x higher.

This disproves “batch pruning is faster” for the 600-missing workload. It also
disproves the prior blanket statement that the candidate is slower for both
missing-only and mixed workloads. The sign reversal is explained by the base's
per-ID existence check (which skips missing IDs) versus the candidate's temp
table plus correlated `EXISTS` deletes; mixed inputs amortize candidate setup.

## Semantic reach and mutations

Observed personally:

```text
CGO_ENABLED=0 go test -race -count=1 ./internal/index -run \
  'TestPruneTombstonedIDs|TestPruneTombstoned_UnderscoreIdDoesNotDeleteNeighbour'
```

- Base: PASS, 11.531s.
- Candidate: PASS, 11.706s.

A disposable early-return mutation failed both tests with surviving rows. A
disposable mutation replacing the `session_verdict` delete with `DELETE ...
WHERE 0` failed with two surviving verdict rows. The benchmark therefore does
not pass merely by timing a semantic no-op.

## Score-rule conflict

The generic charter says `SUPERVISOR_CHARTER.md:88` gives +1 when a finding
survives a rebuttal through stronger evidence, while `SUPERVISOR_CHARTER.md:68-79`
requires immutable identity, commands/output, mutation result, and base evidence.

The later authoritative records narrow this event:

- `state/PRIOR_ART_LOG.md:885`: 386ec9d remains score 0 pending external
  withdrawal/adoption or an actual merge decision.
- `state/PRIOR_ART_LOG.md:890`: withdrawal/adoption requests had no response.
- `state/PRIOR_ART_LOG.md:905`: no score change pending those same events.
- `state/ROTATION_LOG.md:802-803`: rebutted/unsupported, no new score.
- `state/PRIOR_ART_LOG.md:789`: totals remain Furiosa +9, Han +2, Ozzy +3.

No Ozzy reply, adoption receipt, or stopped-merge decision was observed (the
parent supervisor reports no reply as of `20260827T023914Z`). Therefore the
technical finding is REBUTTED, but score eligibility is UNCERTAIN and earns no
point under the live ledger. No merge authorization is granted.

## Cleanliness

Only `TICK45_REBUTTAL_STRESS.md` is committed. Disposable benchmark and
mutation artifacts were removed. No Go file remains edited.
