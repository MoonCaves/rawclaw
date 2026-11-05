# Antigravity benchmark fixture ownership

Source: direct inspection of
`internal/source/antigravity/antigravity_bench_test.go` at
`d235ddc7ad3b9bb2575d648ee209ca462094b8e5`.

The external `antigravity_test` fixture helper has four benchmark call sites
and three `*testing.T` call sites in `journey_test.go`. The benchmark sites
construct `&testing.T{}` instead of using the active benchmark. The minimal
repair uses standard `testing.TB` and passes `b` at the four benchmark sites,
so `Helper` and fixture-write failures belong to the active benchmark while
existing tests retain the shared helper. The separate internal-package test
helper remains unchanged. No custom interface, helper, or dependency is needed;
the repair changes five lines, with no net line-count increase.

Official Go source, fetched before editing:
[testing.go lines 912–942](https://github.com/golang/go/blob/3901409b5d0fb7c85a3e6730a59943cc93b2835c/src/testing/testing.go#L912-L942)
defines the common testing methods, including `Helper` and `Fatal`, and asserts
that `*testing.B` implements them. Go is licensed under BSD-3-Clause.

The ownership repair did not change fixture bytes, workload, timer calls, result assertions,
the existing ignored Discover error, or runtime code. It establishes no speedup,
historical-branch adoption, timer correction, or issue closure.

Ownership-repair validation used package race tests and the existing Antigravity benchmarks
for one iteration with Go 1.27.3. HOME/cache/config/catalog were isolated;
XDG_DATA_HOME was unset so benchmark HOME fixtures retained their existing fallback.

```text
$ CGO_ENABLED=1 go test -race -count=1 ./internal/source/antigravity
ok  	github.com/MoonCaves/rawclaw/internal/source/antigravity	2.905s
```

```sh
CGO_ENABLED=0 go test -run '^$' -bench '^BenchmarkAntigravity' -benchtime=1x -count=1 ./internal/source/antigravity
```

The one-iteration benchmark command exited 0. Its final output was:

```text
PASS
ok  	github.com/MoonCaves/rawclaw/internal/source/antigravity	1.011s
```

These are execution checks, not a before/after performance comparison.

The repository contract and design-note index were checked and left unchanged;
this repair does not move a runtime contract.


## Follow-up: incremental fixture readiness

Source: direct inspection and an isolated diagnostic at
`e8165f7aab8e5d29756b49f424369957d38c8dc6`, created here as a follow-up to
failure ownership. Changing only the incremental fixture transcript basename
to an unrecognized name left zero discovered and indexed sessions, yet the
benchmark returned `IndexFresh` and `PASS`. The unmodified control discovered
and indexed the intended single session. This was a one-iteration behavior
probe, not a performance comparison.

The follow-up checks discovery error/count and the warmup session count
before `ResetTimer`, using the existing native `b.Fatalf` pattern. The fixture
already seeds exactly one session. These checks preserve fixture bytes/paths,
timer calls, the timed loop, shared helper and production code. A count of one
does not establish full message-content parity. No speedup, source adoption or
issue closure is claimed.


Follow-up validation used Go 1.27.3 darwin/arm64 with isolated HOME, cache,
config, Claude and catalog roots, XDG_DATA_HOME unset, and existing Go caches.

```text
$ CGO_ENABLED=0 go test -race -count=1 ./internal/source/antigravity
ok  	github.com/MoonCaves/rawclaw/internal/source/antigravity	2.857s
$ CGO_ENABLED=0 go test -run '^$' -bench '^BenchmarkAntigravity' -benchtime=1x -count=1 ./internal/source/antigravity
PASS
ok  	github.com/MoonCaves/rawclaw/internal/source/antigravity	0.949s
```

The benchmark output above includes its final lines only. The same single
basename mutation, applied through an isolated Go overlay, now fails before
timing with `discover failed: <nil> (got 0, want 1)` and exit 1. The original
fixture passes. These checks establish fixture readiness behavior, not a
performance improvement or full-suite result.


## Source attribution correction — 2026-11-05

The shared `testing.TB` API origin above is separate from readiness comparison
origins. Existing RawClaw tests at immutable revision
`024a68702cc9c72ef945350cd31e0d3cd0d5bf91` supply those patterns:

- [Goose discovery, lines 357–361](https://github.com/MoonCaves/rawclaw/blob/024a68702cc9c72ef945350cd31e0d3cd0d5bf91/internal/source/goose/goose_test.go#L357-L361)
  checks discovery error or a container count other than one. The benchmark
  substitutes its adapter, `cs`, receiver `b` and existing diagnostic.
- [Container indexing, lines 362–368](https://github.com/MoonCaves/rawclaw/blob/024a68702cc9c72ef945350cd31e0d3cd0d5bf91/internal/index/containers_test.go#L362-L368)
  checks the indexing error and session count separately. The benchmark now
  preserves those separate checks and source diagnostics, substituting its
  existing fixture arguments, `nSessions` and receiver `b`.

Both RawClaw sources use the repository's MIT license. The expected count of
one is the existing single-session fixture contract, not a tuning threshold. The earlier combined
error/index-count condition was not a verbatim donor unit; this correction
replaces it with the cited separated checks. No status assertion, new coverage,
fixture bytes, timed loop, timer calls or production behavior changes.

The root contract was checked and left unchanged: this attribution correction
does not move a runtime contract. Original-head validation does not establish
current-main composition, future CI, a speedup or full message-content parity.


## Current-main composition validation — 2026-11-05

Supervisor command receipts validate composition
[`eb8593ed88449677deab89b1ae7a72a93dd8f78d`](https://github.com/MoonCaves/rawclaw/commit/eb8593ed88449677deab89b1ae7a72a93dd8f78d),
based on main `024a68702cc9c72ef945350cd31e0d3cd0d5bf91`.
Its benchmark blob `d7b0f0721fb90f81e4e680306f2db56c7cea79e0` is byte-identical
to the corrected original head `c389675b758173365a54efb575d4e71712e8de5c`.
These are composition checks with `CGO_ENABLED=0` and isolated data roots:

| Toolchain | Command | Recorded result |
| --- | --- | --- |
| Go 1.24.0 | `go build ./...` | Exit 0; empty output |
| Go 1.24.0 | `go vet ./...` | Exit 0; empty output |
| Go 1.24.0 | `golangci-lint run` with a fresh lint cache | Exit 0; `0 issues.` |
| Go 1.27.3 | `go test -race -count=1 ./...` | Exit 0 |
| Go 1.27.3 | `go test -run '^$' -bench '^BenchmarkAntigravity' -benchtime=1x -count=1 ./internal/source/antigravity` | All three existing cases ran; exit 0; `PASS` |

The full-race receipt includes this package-output excerpt:
```text
ok  	github.com/MoonCaves/rawclaw/internal/source/antigravity	6.210s
```
The benchmark receipt ends:
```text
PASS
ok  	github.com/MoonCaves/rawclaw/internal/source/antigravity	1.055s
```

A fresh basename-only negative control returned exit 0 at main `024a687`,
but exit 1 at corrected head `c389675`, before timing, with
`discover failed: <nil> (got 0, want 1)`. This tests fixture readiness;
it establishes no speedup, full message-content parity or issue closure.

Composition evidence is separate from head CI. Run
[`42030177328`](https://github.com/MoonCaves/rawclaw/actions/runs/42030177328)
started before PR #116 was retargeted to main. Its synthetic merge
`a1b1aded33c9b4f25de170bc13f2ddf776ced8ac` has parents `d235ddc` and
`c389675`, so it does not validate the current-main base even if the live
PR metadata now names main. A subsequent head CI run must be assessed
against its actual tested merge/base; this note claims no such result.
