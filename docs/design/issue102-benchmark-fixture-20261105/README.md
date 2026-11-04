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
