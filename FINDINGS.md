# Ponytail review: 8c8216e

Immutable production fence: `internal/index/consolidated.go` only.

- `internal/index/consolidated.go:L843-L859`: `delete` **DROP** — deleting the topic-prune block would restore the regression where refreshing one source removes a co-contributor's topic; the existing `consolidation_affected_sessions` set tracks session membership, not topic ownership.
- `internal/index/consolidated.go:L843-L859`: `shrink` **DEFER** — the `NOT EXISTS` contributor predicate plus incoming `(session_id,start_uuid)` identity is the minimum local SQL that preserves co-contributor topics while removing stale sole-source anchors; no repository primitive or stdlib replacement exists.
- `internal/index/consolidated.go:L843-L859`: `yagni` **DROP** — no new abstraction, interface, dependency, or recovery layer was added by 8c8216e; extracting this predicate would make the one call site shallower and larger.

Mutation/recovery ruling: **DROP production change; DEFER any rewrite.** Existing focused tests passed on current base. A production edit is not justified without a new mutation demonstrating an equivalent, smaller predicate and a test pinning deletion of a removed source session's topic under co-contributor presence.

net: +0 production lines possible; no safe shrink proven.
