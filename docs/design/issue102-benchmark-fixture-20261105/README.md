# Antigravity benchmark fixture ownership

Source: direct inspection of
`internal/source/antigravity/antigravity_bench_test.go` at
`d235ddc7ad3b9bb2575d648ee209ca462094b8e5`.

The external `antigravity_test` fixture helper has four benchmark call sites.
They construct `&testing.T{}` instead of using the active benchmark. The minimal
repair is a concrete `*testing.B` helper parameter and passing `b` at those
four sites, so `Helper` and fixture-write failures belong to that benchmark.
The separate internal-package test helper remains unchanged. No new interface,
helper, or dependency is needed; the repair changes five lines, with no net
line-count increase.

Official Go source, fetched before editing:
[testing.go lines 912–942](https://github.com/golang/go/blob/3901409b5d0fb7c85a3e6730a59943cc93b2835c/src/testing/testing.go#L912-L942)
defines the common testing methods, including `Helper` and `Fatal`, and asserts
that `*testing.B` implements them. Go is licensed under BSD-3-Clause.

This does not change fixture bytes, workload, timer calls, result assertions,
the existing ignored Discover error, or runtime code. It establishes no speedup,
historical-branch adoption, timer correction, or issue closure.

Validation will use package race tests and the existing Antigravity benchmarks
for one iteration with Go 1.27.3. HOME/cache/config/catalog are isolated;
XDG_DATA_HOME is unset so benchmark HOME fixtures retain their existing fallback.
The repository contract and design-note index were checked and left unchanged;
this repair does not move a runtime contract.
