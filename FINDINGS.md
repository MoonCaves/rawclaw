# Ponytail findings: prewarm and tag refresh

- `internal/cli/cmd_prewarm.go:47-68` — `shrink`: the initial `dbPath` and `fullSID` values are overwritten by the same refresh call in both branches; call `refreshTagSession` once, then branch only on the initial locate error to fold. RULING: restore exactly the current locate/refresh/fold behavior while removing duplicate refresh error handling. Expected net: -8 lines.
- `internal/cli/tagrefresh_test.go:133-148` — `delete`: `_ = src` is dead because `src` is used to build the registration below it. RULING: delete only. Expected net: -1 line.

Finder results: `dupl` is unavailable in this environment; `/Users/jay-m4/go/bin/golangci-lint run ./internal/cli/...` reported the confirmed ineffectual assignment above plus an unrelated modernize finding in `cmd_tag_onestore_test.go`, outside the file fence.

No other confirmed net-negative simplifications were found in the fenced implementation and tests. In particular, repeated matching loops and source-resolution branches encode distinct exact/prefix/subagent behavior; adding helpers or abstractions would not shrink the code safely.

Expected total: -9 lines.
