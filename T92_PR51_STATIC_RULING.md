# T92 PR51 Static Viper ruling

**PATCH** — PR #51 head `45259be3166df5d5b6642873d3f96d40b11676bb` against public main `029f60d77e7e03192bc966de3a835a4a32a00fe2`; immutable fetch and ancestry verified (merge-base `9fd82d3bf6ba0ce1027cdf84cec51efe3ba87b5c`). Aggregate stable patch ID: `7380f0b45a2ff2dd7750141e07914672c3d66632`; final commit `45259be3` patch ID: `ef0f8f5523d70df5494bac05df42e6803da289a3`.

Public-main scope: 18 files, 319 additions/1466 deletions. Effective delta after merge-base: FINDINGS.md (+1), internal/index/containers.go (+54/-1), internal/index/containers_test.go (+171). Graphify-first (`/Users/jay-m4/code/rawclaw-khan-graph`): reflect was current; literal query `database sql connection transaction begin immediate rollback unlink` surfaced ConnectRW, ConsolidateFrom, SyncConsolidatedFrom, OpenConsolidated, AcquireConsolidatedFence, and the known single-connection-discipline finding. `explain RefreshDBPath` mapped containers.go L30; new evictStaleRefreshDB had no public-main node and was corroborated on the immutable head.

At containers.go:69-87 the PR opens local *sql.DB, sets SetMaxOpenConns(1), executes BEGIN IMMEDIATE, unlinks db/-wal/-shm, then ROLLBACK and Close. This is implicit pool reuse rather than explicit *sql.Conn pinning; rollback/unlink errors are ignored, unlink precedes rollback, and there is no probe.

Observed gate: `CGO_ENABLED=0 go test -race -count=1 -run 'Test(EvictStaleRefreshDB|RefreshDBPath)' ./internal/index` failed at containers_test.go:786 with `database is locked (5) (SQLITE_BUSY)`. It passed at count=3 and failed again at count=20, so the added regression is intermittently failing under the required race gate.

Successor must make connection scope explicit (or prove pool contract), handle rollback/unlink results, and add stable production-path release-gap/unreadable-cache mutation coverage. No merge authorization.
