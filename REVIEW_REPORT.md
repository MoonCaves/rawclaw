# RawClaw post-A1 architectural review

Date: 2026-09-03 (WITA)

Review base: `a85ddc4..HEAD`

Reviewed commits: `9157774`, `3ab9f22`, `f967400`, `8c71cf2`

Reviewer: Luna

## Verdict

**HOLD.** The refactor is directionally strong and the pure-Go build, lint, formatting, diff hygiene, and benchmark checks are clean. It is not merge-ready because the A3/A4 helper consolidation changes the established `sid8` behavior for non-ASCII session IDs and fails an existing agentproto regression test:

```text
internal/agentproto/agentproto_test.go:144
got "αβγδ" want "αβγδεζηθ"
```

The new helper slices eight bytes (`internal/text/helpers.go:28-33`), while the pre-refactor agentproto implementation and test require eight runes. This is a user-visible formatting and reference-display regression, not a race or test-fixture issue.

## Scope and method

I read the named Ponytail, Ponytail Review/Audit, modular-refactor, codebase-design, and Go review/lint instructions. I oriented with Graphify before source search, refreshed its reflection lessons, and corroborated graph results against the checked-out source and Git diff. The graph exposed the new agentproto modules, but `ParseTranscript` resolution was ambiguous; source and commit evidence therefore controlled the conclusions.

The range contains 21 changed files, 2,236 insertions, 2,991 deletions, and a net reduction of 755 lines. No additional over-engineering deletion candidate was found in this range after applying the Ponytail ladder; the principal issue is behavioral correctness.

## Findings

### P1 — `Sid8` changes the established Unicode contract

`internal/text.Sid8` returns `sessionID[:8]`, which is a byte slice (`internal/text/helpers.go:28-33`). The old agentproto helper counted runes, and the existing test explicitly pins that behavior (`internal/agentproto/agentproto_test.go:142-147`). Eight Greek runes occupy 16 UTF-8 bytes, so the new implementation returns four runes rather than eight.

This helper is now used by agentproto display and reference formatting through `internal/agentproto/types.go`, and by other renderers through their delegating wrappers. The consolidation therefore spreads the changed behavior across several output surfaces. The fix should preserve the pre-existing agentproto contract, add the Unicode case to the shared helper test, and separately decide whether the older byte-prefix renderer contract is intentional. Do not silently change the public display contract while deduplicating.

Evidence:

- New implementation: `internal/text/helpers.go:28-33`.
- Existing pinned contract: `internal/agentproto/agentproto_test.go:142-147`.
- Delegation from agentproto: `internal/agentproto/types.go:167-173`.
- The pre-refactor helper counted runes: `git show a85ddc4:internal/agentproto/agentproto.go`, former `sid8` implementation around lines 334-342.

### P2 — Duplicate UUID replacement can leave stale session timestamps

The canonical A1 parser replaces an earlier message when it sees the same UUID (`internal/source/claude/claude.go:147-151`). That branch continues before updating `Started` and `Last` (`internal/source/claude/claude.go:155-163`). If a later record with the same UUID has a different timestamp, the returned message is the later record but the session watermarks still reflect the superseded record. This can misstate recency and affect browse/search ordering or freshness decisions.

This is an edge case rather than the observed release blocker, and normal Claude duplicate records may carry identical timestamps. Still, the new canonical function owns metadata as well as rows, so its contract should be internally consistent. Add a focused duplicate-UUID test with changed timestamps and update the watermark when replacing, or explicitly define and test first-record timestamp authority.

Evidence: `internal/source/claude/claude.go:109-115`, `147-163`; the index path consumes the returned metadata at `internal/index/index.go:736-745`.

## Change-by-change assessment

### A3/A4 — unused vector interface and helper consolidation

Deleting `embed.VectorStore` is sound: the current vector path is driven by `Embedder`/`BatchEmbedder`, and no production caller required the removed two-method interface. The change reduces speculative seam surface in line with the deep-module rule that one adapter does not justify a public seam.

Moving `CapRunes`, `IsWordByte`, `First10`, and `Sid8` into `internal/text` improves locality and removes repeated implementations. `CapRunes` preserves its negative-cap and rune-safe behavior (`internal/text/helpers.go:7-13`), `IsWordByte` preserves the ASCII predicate (`:15-18`), and `First10` remains an intentional byte slice for ISO-date strings (`:20-26`). The consolidation did, however, reveal that `Sid8` was not semantically identical across callers. The shared helper must follow the strongest existing contract or use distinct names for byte-prefix and rune-prefix behavior.

### A2 — agentproto split

The former 2,826-line file is decomposed into focused same-package modules: types, search, read, outline, locate, topics, and rendering. This is a low-risk seam because package visibility and function signatures remain local to the same Go package; callers do not learn a new interface. The split improves locality without introducing one-implementation interfaces, factories, or dependency layers. `search.go` remains the largest module at 706 lines, but it still groups one coherent search pipeline and is an acceptable next-stage boundary rather than evidence that this change failed.

No race-specific concern was found in the moved code. The search connection cache remains request-scoped and closed by its owner; the full race command did not reach a passing conclusion, so this is a source review observation, not a claim of verified race freedom.

### A1 — canonical Claude parsing

The index path now delegates to `claude.ParseTranscript` (`internal/index/index.go:736-745`), while the Claude adapter also uses it (`internal/source/claude/claude.go:171-181`). This is the intended ingestion seam: normalization, text extraction, UUID handling, CWD discovery, and timestamps have one implementation. `paths.LineCWD` also centralizes top-level and nested CWD lookup, which keeps file discovery and parsing aligned.

The parser retains useful behavior: malformed JSONL lines are skipped and counted, non-indexable records are excluded, empty extracted text is excluded, and UUID duplicates are replaced. The P2 timestamp issue is the residual correctness risk from combining row deduplication with metadata aggregation. The parser still reads the complete file into memory (`claude.ParseTranscript` receives `[]byte`), so peak memory remains proportional to transcript size plus normalized rows. That is consistent with the existing atomic full-reindex design, but a future very-large-transcript optimization would need a streaming parser without splitting the normalization contract again.

### `8c71cf2` — Graphify in the harness

The harness now runs `graphify update .` as Gate 6 when the executable is available (`scripts/harness-gate.sh:32-40`). This keeps the local AST graph synchronized after the verification gates. The shell remains POSIX-compatible. The script still reports a dirty worktree as a warning rather than failing (`:24-30`), and the benchmark command has no numeric threshold despite its “within budget” message (`:32-34`); both are existing harness-design limitations, not regressions introduced by the Graphify addition.

## Verification receipts

Commands were run from `/Users/jay-m4/code/rawclaw-luna-review-post-a1` on 2026-09-03.

| Check | Result | Evidence |
|---|---|---|
| `CGO_ENABLED=0 go build -o /dev/null ./cmd/rawclaw` | **PASS**, exit 0 | Pure-Go build completed successfully. |
| `CGO_ENABLED=0 go test -race -count=1 ./...` | **FAIL**, exit 1 | `internal/agentproto/TestSid8` fails at `agentproto_test.go:144`. The same isolated run also reports unrelated failures in archive/CLI tests; those packages are outside this diff and were not attributed to these commits without a baseline run. |
| `CGO_ENABLED=0 go test -race -count=1 ./internal/agentproto` | **FAIL**, exit 1 | Reproduces only the `Sid8` failure in the changed agentproto area; no race report was emitted. |
| `/Users/jay-m4/go/bin/golangci-lint run ./...` | **PASS**, exit 0 | golangci-lint v2.12.2; output ends with `0 issues.` Bare `golangci-lint` was not on PATH, so the absolute installed path was used. |
| `sh scripts/harness-gate.sh` | **FAIL**, exit 1 | Gate 1 build passes; Gate 2 stops on the same `TestSid8` failure, so later gates do not run. |
| `CGO_ENABLED=0 go test -run=^$ -bench=BenchmarkSearch ./internal/agentproto/... -benchmem -count=1` | **PASS**, exit 0 | Benchmark package completed successfully. No threshold is enforced by the command or script. |
| `gofmt -l internal/ cmd/` | **PASS**, no output | No unformatted files. |
| `git diff --check a85ddc4..HEAD` | **PASS**, no output | No whitespace errors in the reviewed range. |

The worktree had two pre-existing/unrelated untracked entries, `graphify-out/` and `prompt.txt`; neither was staged or modified for this report.

## Architecture and performance conclusion

The changes make the codebase leaner and more navigable while preserving the sovereign-core shape: no new runtime dependency, no new service, and no new public abstraction required for the refactor. The A2 same-package split is a good deepening move because it improves locality without expanding the caller interface. A1 correctly places Claude normalization at the source-adapter seam and removes a duplicate indexer implementation.

The release gate must remain **HOLD** until the shared `Sid8` helper restores the pinned Unicode behavior and the focused plus full race suite are rerun without concurrent test processes. After that, address the duplicate-UUID timestamp test/decision. Longer-term opportunities are a typed store seam for the raw SQLite schema and clearer separation of scope/path identity, both carried forward from the previous architectural audit; they are outside this range and should not be smuggled into this corrective patch.

## Recommended next actions

1. Restore `Sid8` to the established eight-rune behavior, or split the byte-prefix caller under a distinct helper and pin both contracts with tests.
2. Add and resolve the duplicate-UUID timestamp case in `ParseTranscript`.
3. Rerun the full race suite and harness once, serially, after the correction; retain the exact exit codes and durations in the next report.
4. Keep the current A2/A1 structure. Do not add a new interface or generalized parser layer until a second concrete adapter requires it.

