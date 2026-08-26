# Catalog modularity raid

Base: `0d60b4c` on `conor/raid-norm-catalog`.

## Ruling

`internal/agentproto/agentproto.go:1799-1804` — `yagni:` the five-line
`allowed` closure has one caller and only forwards the nil-scope check plus
`slices.Contains`. Inline the predicate at its only call site. This preserves
the load-bearing `projects == nil` meaning (an unlabeled scope means no
project filter) while deleting a shallow local seam. **Take over.**

The surrounding catalog guard must remain unchanged: `tdir == ""` returns
`nil`, so foreign or mixed-source catalog hits fall through to the
source-aware resolver instead of being reconstructed as Claude scopes.

## Rival review

- `8be07d3` replaced the older 13-line linear-search closure with
  `slices.Contains`, but retained the one-caller closure. The stdlib choice is
  correct; the remaining closure is unnecessary.
- `54afa70` used `continue` for foreign catalog paths. That is a semantic
  defect: a mixed Claude/foreign prefix could discard the foreign hit and
  resolve as Claude. The current base's `return nil` behavior is retained.
- `10572cf`/`fc1a075` show the smaller inline predicate shape in rival history;
  only the fenced `agentproto.go` hunk is applicable here.

Observed production delta: `-6` lines (`1` added, `7` deleted), no test changes.
