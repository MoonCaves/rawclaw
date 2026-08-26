# b944d08 + fb893ed7 tag-range hostile audit

Audit date: 2026-08-26. This is a report-only review of the combined range
refactor. Candidate commits:

- `b944d082e9b8d02611b018a25ce9a049066629fc` (`+50/-65`, shared resolver)
- `fb893ed7ae8a1da95f3bbb5b651176cfb2275f6a` (`+1/-7`, bound-check shrink)
- Combined stable patch IDs: `0c8b28032a1f8baf7a6a076ac6205e47d753f476`,
  `cea8cc66c09632db4cd9980063e2e69a3646260c`

## Verdict: SAFE TO ADOPT, with a narrow helper-precondition note

I found no reproducible behavior regression for missing anchors, reversed
anchors, omitted/non-displayable messages, slice boundaries, or previous-topic
selection. `fb893ed7` removes checks that are redundant for the actual callers:
both maps are built from `displayable`, and both fallback loops return indices
inside that slice. The combined change is behavior-preserving against the
pre-refactor implementation on the committed call paths.

The only changed direct-helper behavior is for malformed caller-supplied maps
or an empty slice paired with fabricated in-range map entries: the old helper
returned `false` from `st >= len(displayable)` / `end < 0`, while `fb893ed7`
no longer guards those impossible states. `resolveSegmentRange` is unexported,
and current callers construct `uuidToDispIdx` from the same non-empty
`displayable` slice before calling it. This is a future-caller precondition,
not a current defect.

## Source and parity evidence

`b944d08` is directly based on `539de03d46e4c3f251f123a261045d5ceea7eb0c`.
The relevant file is byte-identical between that parent and both comparison
tips:

    git diff --quiet 539de03 0d1da19 -- internal/cli/cmd_tag.go
    git diff --quiet 539de03 5b9756b -- internal/cli/cmd_tag.go

Both commands exited successfully. Therefore the `cmd_tag.go` behavior under
review is the same on `0d1da19` and `5b9756b` before applying the candidate.

The extracted helper is used at `cmd_tag.go:175` by
`computeUntaggedWindow` and at `cmd_tag.go:250` by `findPrevSegment`.
`resolveSegmentRange` is at `:261–300` on the target:

- Direct UUID hits come from `uuidToDispIdx`, built at `:164–167` and
  `:239–241`; those values are indices of `displayable` by construction.
- Missing/non-displayable start anchors use the forward fallback at `:268–276`.
  It returns an index only when a displayable message has `ID >= anchor ID`.
- Missing/non-displayable end anchors use the reverse fallback at `:285–293`.
  It returns an index only when a displayable message has `ID <= anchor ID`.
- Missing anchors with no known message ID leave `stOK`/`endOK` false.
- Reversed ranges are rejected at `:296` (`st > end`).
- The target then returns the two indices directly at `:299`; no caller can
  index outside `displayable` under the construction above.

For `computeUntaggedWindow`, the range is consumed by `for k := st; k <= end`
at `:176–178`. For `findPrevSegment`, the range must also contain the target
at `:251`; this makes its old explicit bounds checks logically equivalent to
the shared helper's `st > end` rejection.

## Hostile probes and observed gates

I created a temporary package-local differential test in detached worktrees at
`b944d08` and `fb893ed7`, then removed it before finishing. It generated 10,000
deterministic cases covering:

- missing and known start/end UUIDs;
- anchors on omitted/non-displayable messages;
- unknown anchors;
- reversed anchors;
- empty/short displayable slices;
- unsorted synthetic IDs and arbitrary anchor combinations.

The test compared `fb893ed7`'s resolver with the pre-refactor resolver copied
from `539de03`. Observed command:

    CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run TestRangeProbe_FbMatchesPreRefactor
    ok github.com/MoonCaves/rawclaw/internal/cli 1.435s

The same probe plus the tag-prep/incremental tests was run three times:

    CGO_ENABLED=0 go test -race -count=3 ./internal/cli -run 'TestRangeProbe_FbMatchesPreRefactor|TestRunTagPrep|TestIncrementalTagging'
    ok github.com/MoonCaves/rawclaw/internal/cli 43.444s

The target's existing tag test surface also passed:

    CGO_ENABLED=0 go test -race -count=1 ./internal/cli -run 'Tag|tag'
    ok github.com/MoonCaves/rawclaw/internal/cli 17.147s

For comparison, the same existing gate on `b944d08` passed in 17.293s.
No full repository gate was run. No production or integration branch was
modified.

## Final ruling

Adopt `b944d08` + `fb893ed7` as a behavior-preserving, net `-72` line cleanup
on `cmd_tag.go` (`+51/-72` across the two commits). Preserve the current
missing-anchor, omitted-message, reversed-range, and slice-order contracts.
If `resolveSegmentRange` is later exported or called with externally supplied
maps/slices, restore explicit index validation or add a narrower contract test;
that condition is not present in `0d1da19`/`5b9756b` callers.
