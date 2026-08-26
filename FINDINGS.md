# Ponytail shrink findings

- `internal/cli/tagrefresh.go:overlayAuthoritativeTopics`: **delete/shrink** the copied derived slice, position map, replacement loop, and key helper. The function unconditionally returns a copy of `authSegs`, so the preceding 16 lines are unreachable in observable behavior. Target: -16 production lines; preserve complete authoritative replacement semantics and ordering.
- `internal/cli/tagrefresh.go:topicSegmentKey`: **delete** with the dead overlay bookkeeping; no remaining caller after the shrink.
- `internal/cli/tagpublish.go`: **no ruling**. Detached process setup, log ownership, and timeout behavior are in the active terminal-receipt lane; do not duplicate or simplify without a durable-receipt proof.
- `internal/index/consolidated.go`: **no ruling**. Context checks and source-topic pruning are correctness contracts; preserve as-is.
- Tests: **no deletion ruling**. They pin cancellation, overlay deletion, source deletion, co-contributor preservation, origin authority, and stable publication behavior; keep them as independent guards.

Target net: -16 production lines, -16 total lines; zero added lines.
