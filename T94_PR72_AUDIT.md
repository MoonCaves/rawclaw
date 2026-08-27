# T94 / PR #72 claim audit

## Verdict

- Claim: `index: deduplicate incremental append fast path`
- Evidence status: `CONFIRMED` for the claimed refactor and observed behavior; `UNCERTAIN` for complete safety-test coverage.
- Disposition: `PATCH` before ship: add a same-length head-rewrite regression test. The production change itself is a behavior-preserving extraction.
- No product files were edited.

## Immutable identity and ancestry

- Audited head: `631897748703710f89200900e9145a8a1e37c71d` (`deduplicate incremental append fast path`).
- Parent/base: `ae8703bf5e0c4864d1af92ec6fb3e2d5eec8ed88`.
- Fresh `origin/main`: `3b46d4564ab5252bbe91344f47cb6bee62a5f131`.
- `git merge-base --is-ancestor origin/main HEAD`: exit `1`; PR is one commit ahead of its base and four commits behind current main.
- Exact payload is present remotely: `refs/pull/72/head` and `refs/heads/integrate/pony4-incremental-20260828` both equal `631897748703710f89200900e9145a8a1e37c71d`.
- Stable patch-id: `69e261625bcd3e7210fa9b44fecbc50e2b080110`.
- `git range-diff ae8703b..6318977 origin/main..6318977`: identical one-commit patch (`deduplicate incremental append fast path`).

## Exact payload and line accounting

The patch changes only:

| File | Added | Deleted | Meaning |
| --- | ---: | ---: | --- |
| `internal/index/containers.go` | 2 | 19 | Replace inline generic append-tail implementation with helper call |
| `internal/index/index.go` | 9 | 24 | Replace inline Claude append-tail implementation with helper call |
| `internal/index/tail.go` | 29 | 0 | Add shared `appendTailIfPossible` helper |
| **Total** | **40** | **43** | **net -3 production lines** |

No test or documentation lines changed. The helper preserves the old guards and order: growth check, prefix fingerprint equality, tail parse, incomplete-tail no-op, CAS append, stale-watermark no-op, increment counter only after successful append, and full-reindex fallback on all other errors. It receives the same `rawPath`, resolved path `rp`, `origin`, `sourceID`, `fileMeta`, mtime, and size values at both callers (`internal/index/containers.go:354-356`, `internal/index/index.go:940-950`, helper `internal/index/tail.go:154-181`).

## Checks and current-base relevance

1. `CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'TestIncrementalIngest_AppendFastPath|TestTailEdge_ContentAppendedAfterWatermark|TestAppendContainer_StaleWatermarkIsNoOp|TestIncrementalIngest_FallbackOn|TestEnsureFreshContainer_IncrementalEndToEnd'` — exit `0`, `13.104s`.
   - Covers Claude, Codex, and Antigravity append paths; incomplete tails; truncation; head rewrite; malformed complete tail; stale-watermark CAS; and consolidated publication.
   - Claude append test compares the incremental database against a clean full-reindex reference.
2. `CGO_ENABLED=0 go test -race -count=1 ./internal/index` — exit `0`, `155.510s`.
3. Disposable current-base worktree: cherry-pick `6318977` onto `origin/main` — exit `0` (no conflict); focused append/CAS oracle — exit `0`, `6.037s`.

## Mutation/deletion oracle

In a disposable worktree, mutated `appendTailIfPossible` to bypass `headFP != prev.fp` while retaining compilation:

```go
if headFP == "" { // MUTATION: accept changed prefixes
```

`CGO_ENABLED=0 go test -race -count=1 ./internal/index -run 'TestIncrementalIngest_FallbackOnHeadRewrite|TestIncrementalIngest_FallbackOnTruncation|TestIncrementalIngest_FallbackOnMalformedCompleteTail'` exited `0` (`3.149s`). This mutation surviving is a test-quality warning: the existing head-rewrite fixture changes the first line length, so the tail begins mid-record and parsing fails, forcing the safe full-reindex fallback even with the fingerprint guard removed. A same-length prefix rewrite followed by a valid newline-terminated append is needed to falsify the unsafe mutation. Therefore the implementation’s direct equivalence is confirmed, but full dedup safety is not fully pinned by current tests.

## Final ruling

`CONFIRMED` / `PATCH`: the PR correctly deduplicates two identical production fast-path bodies into one helper and is green on the focused race suite and current-base applicability check, with net `-3` production lines. Add the same-length head-rewrite regression test, then the safety claim can be promoted from `UNCERTAIN` to `CONFIRMED` without changing the helper design.

The nearest `AGENTS.md` chain and project docs were checked; intentionally unchanged because this is a report-only audit and no scope or contract moved.
