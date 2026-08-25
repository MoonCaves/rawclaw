# Ingest-on-write performance report

## Result

The before/after comparison is inconclusive as a speedup measurement on this
machine. The shared corpus was being updated by concurrent agents, and the
all-project operations frequently reached their timeout. The read path was
fast and stable; search and browse were dominated by live indexing and
contention.

| Operation | Old: commit `3bea613` | New: current main | Trials and median |
| --- | ---: | ---: | --- |
| Cold search for `ingest-on-write` | 69.82 s | 90.33 s | 3 fresh processes; 90 s per-run cap |
| Warm search, 10 runs | 41.00 s | 53.50 s | 10 fresh processes; 60 s per-run cap |
| `browse --all` | 90.39 s, timeout | 90.30 s, timeout | 3 fresh processes; every trial hit the 90 s cap |
| One session read | 0.01 s | 0.01 s | 3 fresh processes; median rounded to hundredths |

The timeout values are wall-clock measurements of a run stopped by RawClaw's
configured deadline, not successful operation times. The warm-search medians
include timeout-capped runs, so they should not be interpreted as normal
steady-state latency.

## Method

- The old executable was verified against the clean predecessor clone at
  commit `3bea613`.
- The new executable was verified to expose the current `ingest` command and
  was compared against the current main worktree.
- Both executables were run against the same live, consolidated corpus on the
  same machine. No synthetic transcripts or isolated test database were used.
- The search query was the distinctive phrase `ingest-on-write`, with all
  projects selected and the current session excluded.
- Cold search used three independent process launches with a 90-second
  deadline. Warm search used ten independent process launches with a
  60-second deadline. Browse used three independent process launches with a
  90-second deadline. Read used the same valid search reference for three
  independent process launches.
- `/usr/bin/time -p` supplied wall-clock `real` time because `hyperfine` was
  not installed.
- The corpus search output reported 5,524 sessions and 86,309 candidate
  messages. It also reported 16 non-consolidated project databases and
  incomplete vector coverage, so the reported search universe is explicitly
  not a complete all-source corpus.

## Caveats

- This was a shared, busy machine. Other agents were ingesting and searching
  concurrently, and the consolidated store changed during measurement.
- RawClaw live-indexed on invocation. That work is part of the measured user
  command and is the behavior the ingest-on-write restructure is intended to
  change, but concurrent writers make the before/after comparison noisy.
- The all-project browse and stats probes reached the deadline for both
  versions. The report therefore establishes a timeout boundary, not a
  successful browse latency.
- The old and new binaries reported development build metadata rather than a
  reproducible embedded version string. Artifact identity was therefore
  checked through the predecessor commit and command-surface expectations.
- These results describe this corpus and machine only. They do not establish
  a general performance regression or improvement.
