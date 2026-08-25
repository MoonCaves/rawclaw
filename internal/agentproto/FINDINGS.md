# Ponytail review findings

Review scope: `internal/agentproto/**`. The package's nil-vs-empty values, error
strings, ordering, JSON shape, and rendered text are observable contracts. The
findings below are limited to behavior-preserving local simplifications.

- `internal/agentproto/agentproto.go:L437`: stdlib: `len([]rune(s))` allocates only to count code points. `utf8.RuneCountInString(s)` preserves the count without the temporary slice. Net: -1 line after the import update.
- `internal/agentproto/agentproto.go:L2086-L2093`: yagni: `bookendRows` is a fixed-argument wrapper with one caller and no alternate implementation. Inline the existing `store.BookendMessages` calls in `outline`. Net: -7 lines.
- `internal/agentproto/agentproto.go:L2202-L2210`: shrink: `renderWarnings` accepts a variadic suppression list, but its only suppressed call supplies one code. Take one suppression string and compare directly, preserving warning order and output. Net: -4 lines.
- `internal/agentproto/agentproto.go:L2381-L2389`: yagni: `topicFetch` has one caller and only names a four-line arithmetic rule. Inline the calculation at that call site, preserving the existing floor and multiplier. Net: -8 lines.

Skipped deliberately:

- `sid8`, `uuid8`, `runeSlice`, and `runeRange`: their separate names and rune-safe behavior document and enforce ref/output contracts; combining them would trade clarity for fewer declarations.
- `resolveScope`, `firstRowPerSession`, `scopeProjects`, and the warning-input struct: these are small, but each names a semantic boundary used by tests or fallback behavior; no safe net simplification was clear.
- Per-project versus consolidated-store flow, search fusion, scope reporting, and rendering: cross-package or observable behavior; report-only outside this lane.
- Any test changes: none are needed for these syntax/structure-only edits; existing tests remain the referee.

net: -20 lines possible.
