# Ozzy claim spy — Furiosa Tick 32

## Cutoff and scope

The authoritative cutoff is the Tick 31 `ROTATION_LOG.md` completion at
`2026-08-27T00:22:37Z` (the matching `SCORECARD.md` entry records no score
claim and unchanged totals Furiosa `+9`, Han `+2`, Ozzy `+3`). I audited
Ozzy's material benchmark, sidecar, composite, adoption, rebuttal, and branch
activity after the Tick 28 completion at `2026-08-26T23:54:03Z`, through that
cutoff. The Tick 31 challenge receipt
`20260827T001707Z-677a3755-tick31-harvest-challenge-bench.md` is the decisive
request in this window.

## Verdicts

### `386ec9d03bc4b4ae77ef8238d06e0f8b0782de21` — UNCERTAIN

The challenge is not rebutted. Immutable identity is:

```text
parent: 0d1da19c4c21961b86cb3ca84ed047d941c83ed3
diff:   2 files, 112 insertions, 36 deletions
whole patch-id: 356c1cb3878d142f910494843358b2737554dace
code patch-id:  6b42e87e9d75eccc8a5527faa6c001653c15be82
range-diff:     386ec9d maps to itself on 0d1da19..386ec9d
```

The current product base in the inspected refs is the same `0d1da19`; the
candidate is therefore not stale by ancestry. Its benchmark blob is
`d3e72efd4163322294f4d6793bf8d1cfa0798105d`; the parent has no corresponding
benchmark file. The fixture creates 600 missing and 200 live IDs, seeds six
tables for the live IDs, and the timed call performs the new batch prune.
That confirms the checked-in candidate does execute deletes, but it is still
only a new-implementation benchmark.

Furiosa's immutable mutation receipt (`5878c48064a797314986a884e10163b086a84c5c`)
changed both live-ID bounds from `i < 200` to `i < 0`, then restored the source
byte-for-byte. The no-live-ID mutation still compiled, passed, and emitted
apparently valid timing output (`1.630 ms` median versus `15.142 ms` restored;
three samples per arm). Because the benchmark has no deleted-row, remaining-row,
or other semantic assertion, timing alone cannot prove that the intended work
occurred. This directly satisfies the challenge trap: a fast number after
removing the cargo is not semantic proof.

The earlier Ozzy report on `73171fd448fbe2622ed39c8e58f090172587771e`
(`FINDINGS_PRUNE.md`, report hash
`91e034969973666076e447af1e8675c103cad94feb5aae4e3be07e7ca34bff4e`) likewise
called for a before/after baseline, but supplied only three old/new samples,
not the challenged six paired samples plus semantic assertion, benchstat
confidence, pragma/hardware packet, and concurrent lock-busy result. No
immutable Ozzy response before the cutoff supplies the requested adopt/rebut
packet. Therefore there is no performance score and no current-base readiness
claim.

### Ozzy sidecar/composite activity — NO SCORE CLAIM

No Ozzy branch moved after the cutoff. The latest relevant refs are all
earlier (timestamps shown in WITA): `c38f79acf9c9ae43ebd091a95f36837f43c0e423`
at `05:38:03`, `bc8af914d7d5736a8155929e0d81c998a4be5efc` at `06:28:19`, and
the no-fold referee refs through `06:27:13`; these are before the UTC cutoff
(18:28–22:28Z on Aug 26). `c38f79a` is the already-scored sidecar adoption
event, not a new post-Tick-28 claim. `bc8af914` remains docs-only: one file,
`+12/-10`, stable patch-id
`1b46d699f573efb107f8825f983771b4c9161d61`; inherited ancestry is not new
implementation. No adoption, rebuttal, score event, or merge authorization
is evidenced in the post-Tick-28 window.

The benchmark worktree `ozzy/flash-prune-benchmark` is dirty (`cdc063d`, one
uncommitted `internal/index/consolidated_test.go` change); it has no upstream.
It is not the pushed `386ec9d`/`73171fd` payload. The pushed sidecar worktree
is clean and at `0/0` (`c38f79a`); the docs-only composite worktree is clean
but has no configured upstream. These state distinctions do not create a
score claim.

## Final ruling

`386ec9d`: **UNCERTAIN** — real deletes are present, but the required semantic
benchmark guard and fair evidence packet are absent. All other Ozzy activity
in the audited post-Tick-28 window: **NO SCORE CLAIM**. Totals remain Ozzy
`+3`; no merge authorization follows.
