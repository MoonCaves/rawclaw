# Tick 42 Ozzy query-shape/performance falsification

## Verdict

**REBUTTED for the tested workload; no performance adoption claim is valid.**
The candidate is semantically effective, but its correlated `EXISTS` query shape is materially slower than the parent per-ID implementation on identical byte-equivalent fixtures. This is bounded evidence, not a claim about every possible database/index distribution.

Candidate: `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21`  
Parent: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`

## Fixtures and oracle

Disposable test fixture used the same schema/pragmas and six durable tables (`sessions`, `messages`, `session_sources`, `file_index`, `topic_segment`, `session_verdict`). Cases:

- A: one existing ID plus 600 missing IDs.
- B: 600 missing IDs.
- Rows were seeded identically before every old/new sample; the live row was expected to be deleted from all six tables.

The durable-work oracle required non-zero intended work and exact zero remaining rows in all six tables. Parent and candidate passed. Two candidate-only mutations were killed and then restored:

- no-op return: oracle failed with `sessions remains: 1`.
- skipped `session_verdict` delete: oracle failed with `session_verdict remains: 1`.

Therefore timing was not accepted as semantic proof by itself.

## Query plans

Captured with `EXPLAIN QUERY PLAN` on the fixture:

- Parent per-ID `DELETE ... WHERE session_id = ? OR session_id LIKE ?`: `SCAN messages` (and `SCAN sessions` for the session delete).
- Candidate correlated `EXISTS` delete: `SCAN messages`, `CORRELATED SCALAR SUBQUERY 1`, `SCAN t`.

The tested schema did not yield an indexed target-table lookup for these predicates; the candidate adds a correlated scan of the temp table to each target-row evaluation.

## Interleaved benchmark evidence

Ten old/new pairs were run in alternating order on Apple M4, darwin/arm64, `benchtime=1x`, `-benchmem`, with reset-and-reseed outside timed regions. `benchstat` was run after normalizing benchmark names:

| case | parent | candidate | delta |
|---|---:|---:|---:|
| 600 missing, ns/op | 1.319 ms ±36% | 2.307 ms ±11% | **+74.99%**, p=0.000, n=10 |
| 600 missing, B/op | 460.2 KiB ±1% | 4,111.9 KiB ±0% | **+793.43%** |
| 600 missing, allocs/op | 10,996 ±0% | 6,699 ±0% | -39.09% |
| one existing + 600 missing, ns/op | 1.563 ms ±3% | 2.569 ms ±5% | **+64.43%**, p=0.000, n=10 |
| one existing + 600 missing, B/op | 465.3 KiB ±5% | 4,118.6 KiB ±0% | **+785.09%** |
| one existing + 600 missing, allocs/op | 11,074 ±0% | 6,710 ±0% | -39.41% |

The original candidate benchmark’s 600-missing + 200-live shape was also preflighted once (`15.636 ms/op`); it is not used as comparative proof because it only times the new implementation.

## Patch identity and applicability

- Candidate whole patch-id: `356c1cb3878d142f910494843358b2737554dace`
- Product path patch-id (`consolidated.go`): `6b42e87e9d75eccc8a5527faa6c001653c15be82`
- Benchmark path patch-id: `ca293434ea17286206af6c8a8c97b00e393f1c4c`
- `git range-diff 0d1da19..386ec9d 0d1da19..386ec9d`: exact self-match; candidate applies to the verified parent.
- Direct candidate product delta: `+35/-36` lines. Added benchmark: `+77/-0` lines.

## Required action and Direction Lock

Smallest correction: Ozzy must either drop the speed claim, or supply a benchmark where old and new use the same real indexed schema and a semantically asserted durable-work oracle. Direction Lock impact: **technical HOLD / score 0 for performance adoption**; this report does not authorize a merge.

Raw evidence hashes:

```
plans       7914cabb725da1b0651ed27dc276aa322026394e4b3fd3ad823e7168ab81e027
missing     0b70ee8dfb7ec6d4aa34b4710f7de86472bac14e12194befffaddcd0c09202f4
existing    e99847137916122f0baf74041a63b1cb4a94f8823a9dbc0ed6172890ab51e5f6
noop-red    91b2693381cfe99757fb78ae2759aff9a1fbd01053677c69718461c9853e6111
verdict-red 99dd12b0fd160b3d71102520e1a45c37292bb5ce5fc3eb8c1a9f5ca1ee7b9c15
final-tests 89a175233833d930b6bc7c125d8ad14ad4771dabf0a2a1011df6698da9912a6c
```

Temporary benchmark/mutation files were removed; no product file was changed.
