# Issue 102 FTS search measurements

These files preserve an exploratory comparison between two RawClaw snapshots. They are timing and allocation observations for those snapshots on one loaded machine. They do not show an optimization gain.

Source: created here from the included native Go benchmark logs and the two immutable snapshots.

## Snapshots and setup

| Label | Source snapshot | Go metadata |
| --- | --- | --- |
| `main` | `0afecdfab2321abb49733746f7250e6572d5ec5e` | Go 1.27.3, `CGO_ENABLED=0`, Darwin arm64, Apple M4 |
| `perf` | `134fdc6d4081c39b89af819b4272e7dc2756f43f` | Go 1.27.3, `CGO_ENABLED=0`, Darwin arm64, Apple M4 |

Each snapshot has ten Warm and ten Cold samples. The run used a one-second benchtime, `-cpu=1` (`GOMAXPROCS=1`); no CPU affinity was set. The parent runner reported successful compilation and execution for both exact snapshots, with exit status 0. The raw Go logs contain ten `PASS` blocks apiece. Independent artifact review verified build metadata and reproduced the saved benchstat output.

The runner alternated the two binaries in ABBA order across pairs. There is no retained per-sample timestamp or independent order ledger, so sample chronology cannot be reconstructed from these files. The host had ten CPUs and reported load averages of 10.17, 11.01, and 11.36 near the run. Those are point readings; this was not a quiet-host measurement.

For a per-snapshot reproduction, check out the listed SHA and run:

```sh
GOTOOLCHAIN=go1.27.3 CGO_ENABLED=0 go test -run '^$' -bench '^BenchmarkFTS5Search$' -benchmem -benchtime=1s -cpu=1 -count=10 ./internal/index
```

This batches the ten samples at one snapshot. The recorded comparison instead ran compiled snapshot binaries in alternating order; running the command above does not reproduce that interleaving.

## Comparison

`comparison.txt` is the output of `benchstat main.bench.txt perf.bench.txt`. It reports the sample distributions for the `FTS5Search` Warm and Cold cases. The displayed percentage changes describe those two artifacts only. Benchstat treats the samples as independent groups; the output is not a paired analysis. Its rounded `p=0.000` is not a probability of exactly zero.

The common benchmark fixture and query settings match, but the snapshots perform different work. Main's `SearchHit` includes ID, BM25 score, and rank. Its query selects ten columns and computes `RANK() OVER (ORDER BY bm25_score)`; the historical snapshot selects seven columns and does not compute or return those fields. The benchmark checks only that the call has no error and returns at least one hit. It does not compare result count, membership, ordering, snippets, IDs, scores, or rank.

Therefore the lower timings and allocation counts shown for `perf` cannot establish faster equivalent work, cannot isolate the cause to a particular commit, and cannot be attributed to agent-protocol enrichment or an end-to-end user operation. In particular, do not present the Warm `-37.18%` or Cold `-23.22%` figures as optimization gains. The extra ranking and result fields in main are a concrete difference in work, not a quantified explanation of the measured gap.

These measurements do not justify transplanting the historical branch or changing runtime code. A future comparison would first need an agreed output contract and comparable implementations, plus evidence that the compared results satisfy that contract.

## Files

- `main.bench.txt` — raw benchmark output for main snapshot `0afecdf`.
- `perf.bench.txt` — raw benchmark output for historical performance snapshot `134fdc6`.
- `comparison.txt` — benchstat comparison generated from those two local files.

The earlier issue #102 source audit at commit `2f06230` addresses the separate benchmark-file and module-layout allegations. This measurement does not alter that audit's bounded findings.
