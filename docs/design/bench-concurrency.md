# Concurrency benchmark

This document records the first smoke run of `scripts/bench-concurrency.sh`.
It measures root-query search wall latency while concurrent `ingest` writers
update the same isolated SQLite store. RawClaw's CLI has no separate
`search` verb; the timed operation is the documented `rawclaw --json "query"`
form.

## Harness

The harness builds no assumptions about the user's machine paths. It creates a
temporary corpus and exports `HOME`, `CLAUDE_CONFIG_DIR`, `XDG_DATA_HOME`,
`XDG_CACHE_HOME`, and `RAWCLAW_CATALOG_DIR` into that temporary directory. It
seeds small Claude-shaped JSONL transcripts, primes the index, and verifies
each search exit status. Search samples are written as CSV with one row per
successful invocation. Ingest failures also fail the run and are printed.

## Smoke run

The binary was built from this worktree with `CGO_ENABLED=0` and the harness
was run with its defaults: 20 search workers, 3 ingest writers, and 60 seconds.
The exact invocation was:

```sh
CGO_ENABLED=0 go build -o ./rawclaw ./cmd/rawclaw
./scripts/bench-concurrency.sh --binary ./rawclaw --output ./bench-concurrency.csv
```

| search workers | ingest writers | duration | samples | p50 | p95 | max |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 20 | 3 | 60 s | 891 | 1130.841 ms | 1832.013 ms | 2514.539 ms |

This run is a harness smoke-proof, not a speedup claim. Other agents are
building and testing on this machine, so machine load is uncontrolled and the
numbers must not be compared with a baseline. A clean run on a quiet machine
is still required for any performance conclusion. If the write flock causes
latency spikes, those spikes are the result to report rather than a harness
failure. This run recorded a substantial write-contention tail (p95 about
1.62 times p50; max about 2.22 times p50), but it does not establish how much
of that tail is caused by the writers versus the busy machine.
