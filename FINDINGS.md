# Atomic session-start claim review

Base: `e6855562`. Owner change: `92d00670fb87e7b1d0e86cff8eedf84247fc6eb0` (`fix(cli): atomically deduplicate session-start ingest`). Owned files: `internal/cli/setup.go`, `internal/cli/setup_test.go`.

## Confirmed finding

`internal/cli/setup.go:64-88` and `:155-179`: the generated Claude and Codex POSIX hooks initialize `claimed=0`, set it only when exclusive creation succeeds, exit only when an existing entry is observed, then launch `ingest` only under `if [ "$claimed" -eq 1 ]`. If catalog directory creation or claim persistence fails without an existing entry, execution reaches the end of the block with `claimed=0` and silently skips ingest. This regresses the prior fail-soft contract, which launched ingest before catalog persistence. Confirmed by direct control-flow tracing against the exact owner diff; a focused forced-persistence-failure test must pin runtime behavior.

Ponytail ruling: `shrink` — preserve atomic exclusive-create and existing-entry exit, but collapse the state gate so ingest proceeds whenever no existing claim was observed. No helper or abstraction: the two templates are intentionally parallel and the smallest safe correction is local to each generated script.

Four questions under oath: (1) existing code reused: the current `set -C` exclusive-create and `[ -e "$entry" ]` dedup check; (2) stdlib cannot do it: this is generated POSIX shell, so Go helpers are unavailable at hook runtime; (3) one line fails: the existing `claimed=1` gate is exactly the defect, while the two parallel templates each need their condition corrected; (4) net lines disappear: no helper or state variable is added, and the correction is two condition-line edits with zero production net growth.

Net accounting: owner production diff was +93/-39 lines (+54 net) across both templates relative to the parent. Proposed production correction: two condition-line edits, net 0 production lines; tests add only forced-failure and concurrent-dedup assertions.

Separate issue: the Stop-test polling `1s -> 5s` change has no reproduced flaky receipt in this review and must be restored if present. No such owned test change is present at this base.

Verdict: CONFIRMED; smallest correction required.
