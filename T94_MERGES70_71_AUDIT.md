# T94 post-merge audit: PRs #70 and #71

Date: 2026-08-28 WITA (UTC+8)

Scope: audit the claims represented by merged PR #70 (`ebd72b7`) and PR #71
(`59101d4`) on `main`. This is a report-only audit; no product files or
mailbox cursors were changed.

## Baseline and ancestry

- Audited checkout: `main` at `3b46d4564ab5252bbe91344f47cb6bee62a5f131`.
- `3b46d45` is also `origin/main`; the checkout was clean before this report.
- PR #70 merge: `0e0581e29086458b18d2c8a7674e4fbb472c6ec5`, parents
  `ae8703b` and `ebd72b7`. The PR commit is the second parent and is an
  ancestor of the merge.
- PR #71 merge: `3b46d4564ab5252bbe91344f47cb6bee62a5f131`, parents
  `0e0581e` and `59101d4`. The PR commit is the second parent and is an
  ancestor of `main`; its own parent is the common pre-PR base `ae8703b`.

Stable patch IDs independently match each merged payload to its PR commit:

| Claim | Merged payload | PR payload | Stable patch ID | Net diff |
| --- | --- | --- | --- | ---: |
| #70 shared home expansion | `ae8703b..0e0581e` | `ae8703b..ebd72b7` | `ea81c025ccab213cfe0ea5e2952de6318905a25b` | -14 lines |
| #71 shared SQL placeholders | `0e0581e..3b46d45` | `ae8703b..59101d4` | `85fa48ca63280567ef111f5ddca55d8222ebb37d` | -3 lines |

The merge trees contain only the claimed files: #70 changes
`internal/lifecycle/lifecycle.go`, `internal/paths/paths.go`, and its test;
#71 changes only `internal/store/messages.go`. `git diff --check` is clean.

## PR #70 — reuse shared home path expansion

Claim: export and reuse the existing `internal/paths` home-expansion helper,
remove the duplicate lifecycle implementation, and preserve `~` / `~/...`
behavior.

Evidence:

- `internal/lifecycle/lifecycle.go` now calls `paths.ExpandHome` at every old
  `expandHome` call site and deletes the duplicate 22-line implementation.
- `internal/paths.ExpandHome` retains the same predicates, `os.UserHomeDir`
  fallback, and `filepath.Join` behavior. The only implementation spelling
  change is `strings.TrimPrefix(path, "~/")` for the old `path[2:]` slice;
  those are equivalent after the same prefix guard.
- Existing path and lifecycle behavior tests pass.
- Hostile equivalence oracle: 10,013 inputs, including malformed tilde forms,
  separators, empty/relative paths, and 10,000 deterministic random strings;
  old and new expansion models were equal for every input.
- Focused race gate: `CGO_ENABLED=0 go test -race -count=1 ./internal/paths ./internal/lifecycle` — PASS (`paths` 2.150s, `lifecycle` 1.585s).
- Focused lint: `golangci-lint run ./internal/paths/... ./internal/lifecycle/...` — 0 issues.

Verdict: **SHIP**. This is a safe shrink with no semantic drift or post-merge
blocker.

## PR #71 — reuse placeholder helper for project messages

Claim: reuse `store.placeholders` in `MessagesForProjects`, preserve the SQL
and argument order, and remove three production lines.

Evidence:

- The old local slice plus `strings.Join` is replaced by
  `placeholders(len(projects))`; the caller still returns early for an empty
  project list, so the helper's `n <= 0` branch is unreachable here.
- For every positive project count, the shared helper emits the same
  `?,?,...` text. The `args` loop and project order are unchanged, so SQL
  parameter binding is unchanged.
- Hostile equivalence oracle: counts from -10 through 100 plus 1,000 and
  10,000; old and shared placeholder generation matched exactly, with no
  extra text and the expected number of `?` markers.
- Focused race gate: `CGO_ENABLED=0 go test -race -count=1 ./internal/store` — PASS (18.477s).
- Downstream semantic caller gate: `CGO_ENABLED=0 go test -race -count=1 ./internal/semantic` — PASS (10.399s).
- Focused lint: `golangci-lint run ./internal/store/...` — 0 issues.

Verdict: **SHIP**. This is a safe three-line shrink with no SQL, ordering, or
behavioral drift and no post-merge blocker.

## Immediate challenge status

None. Both claims are present on current `main`, match their merge payloads by
ancestry and stable patch ID, pass focused race/lint gates, and survive hostile
equivalence checks. No PATCH or REJECT ruling is warranted.
