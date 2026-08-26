# Conor `fb893ed7` Range-Resolver Audit

Audit branch: `norm/ozzy-spy`.

Target: `fb893ed7ae8a1da95f3bbb5b651176cfb2275f6a` (`refactor(cli): shrink segment range bounds`).
Base: `b944d082e9b8d02611b018a25ce9a049066629fc` (`refactor(cli): share topic segment range resolution`).
The target is directly based on the stated base. Its diff is one file, `internal/cli/cmd_tag.go`, with `+1/-7`, net `-6` production lines and no test or documentation changes.

## Verdict: SAFE TO ADOPT

The six deleted lines are defensive bounds checks and clamps that cannot be reached through the current production callers. `resolveSegmentRange` now rejects missing anchors and reversed ranges, then returns the indices produced either by a map built directly from `displayable` or by loops whose indices are necessarily valid positions in `displayable`. No production behavior changes for valid or invalid stored segment anchors.

This is safe as a production cleanup under the current private-helper contract. If a future caller passes independently constructed maps, the helper no longer defends against malformed map values; that is outside the current call graph and should be treated as a new contract requiring a focused test or validation.

## Exact patch and prior-art evidence

The target changes only the resolver tail at target `internal/cli/cmd_tag.go:296-299`:

```diff
- if !stOK || !endOK || st > end || st >= len(displayable) || end < 0 {
+ if !stOK || !endOK || st > end {
     return 0, 0, false
 }
- if st < 0 {
-     st = 0
- }
- if end >= len(displayable) {
-     end = len(displayable) - 1
- }
 return st, end, true
```

Stable patch IDs, computed independently for each parent-to-child diff:

- `b944d08..fb893ed7`: `cea8cc66c09632db4cd9980063e2e69a3646260c`
- `539de03..b944d08`: `0c8b28032a1f8baf7a6a076ac6205e47d753f476`

The IDs differ. `git log -S` also shows that the exact bounds/clamps were introduced by `fb893ed7`, so this is a novel cleanup following the shared-resolver refactor, not a duplicate patch.

## Semantic trace

`computeUntaggedWindow` constructs `uuidToDispIdx` by ranging over `displayable` and passes it to the resolver at `internal/cli/cmd_tag.go:164-175`. `findPrevSegment` constructs the same kind of map and calls the resolver at `internal/cli/cmd_tag.go:250-251`.

- Direct start/end UUID hits return map values assigned only as `i` from `range displayable`; therefore successful direct values are in `[0, len(displayable)-1]`.
- A missing start UUID can fall back only through `for i, dm := range displayable` at `:268-275`; a successful `st` is likewise in range.
- A missing end UUID can fall back only through `slices.Backward(displayable)` at `:285-293`; its yielded index is also a valid displayable index. The reverse scan changes selection order, not index bounds.
- A missing start or end anchor, including an anchor present in `msgs` but with no qualifying displayable neighbor, leaves its `*OK` false and returns `(0, 0, false)` at `:296-298`.
- An empty `displayable` is handled before resolver use by `computeUntaggedWindow` at `:160-162`. Even if the helper were called directly with an empty slice, both fallback loops have no successful iteration; direct maps built by the callers are empty.
- Reversed ranges (`st > end`) remain rejected at `:296`; this condition was retained.
- Anchors outside the message-ID range cannot satisfy the fallback comparison, so they remain rejected. Anchors in `msgs` but omitted from `displayable` may intentionally resolve to the nearest displayable boundary; that behavior is unchanged.
- The old `st < 0`, `end >= len(displayable)`, `st >= len(displayable)`, and `end < 0` cases require malformed externally supplied map entries or indices not yielded by the shown loops. No current production caller supplies either.

The shared helper is used only by the two callers above in the target tree. The target therefore preserves the behavior of tagging existing ranges and finding the previous segment while removing unreachable normalization.

## Test and contract evidence

The target tree was independently tested with:

```text
env CGO_ENABLED=0 go test -race -count=1 -v ./internal/cli -run 'Tag|tag'
```

Result: **PASS**, `ok github.com/MoonCaves/rawclaw/internal/cli 23.063s`.

The suite exercises incremental tagging closeout/growth, chunked rerun behavior, fully-tagged no-op behavior, out-of-window rejection, unknown and ambiguous anchors, and tag-write round trips. It does not contain a dedicated direct unit test for `resolveSegmentRange`; the bounds proof above is from the target source and its callers. This is a focused race gate, not a repository-wide green claim.

No target production or test file was edited during the audit. `gofmt -w` is not applicable to the report-only change. The report itself must pass `git diff --check` before commit.

## Net lines and adoption ruling

| Area | Target delta |
|---|---:|
| Production | `+1/-7`, net `-6` |
| Tests | `0` |
| Documentation | `0` |

Ponytail ruling: **delete** the unreachable bounds/clamps. **Adopt `fb893ed7`**. Preserve the caveat that the helper is safe because its maps are locally derived from `displayable`; if that input contract changes, add validation and direct boundary tests before relying on the cleanup.
