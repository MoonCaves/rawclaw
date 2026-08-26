# Locate and Tagging Salvage Findings

## Candidate 1: Direct Project Filter in catalogCands
- **Rival SHA**: `8be07d3a6794d0abfc5a043ad11c08e335787c20`
- **Location**: `internal/agentproto/agentproto.go:1797-1809`
- **Evidence**: `catalogCands` allocates a redundant 6-line local closure `allowed := func(project string) bool` which only calls `slices.Contains`. Inlining `if projects != nil && !slices.Contains(projects, hit.Project)` eliminates closure overhead and boilerplate without altering filtering semantics.
- **Ruling**: `TRANSPLANT_EXACTLY`

## Candidate 2: Direct LocateConsolidatedSession in refreshTagSession
- **Rival SHA**: `7a478345cb0d11907ea2575d25024e10d740762c`
- **Location**: `internal/cli/tagrefresh.go:112-118`
- **Evidence**: `refreshTagSession` called `agentproto.LocateSession(sessionArg, nil, func() []view.Scope { return nil })` with a dummy nil scope callback purely to avoid full-tree sweeps. `agentproto.LocateConsolidatedSession(sessionArg)` is the dedicated, clean API for probing consolidated storage directly without fallback sweeps.
- **Ruling**: `TRANSPLANT_EXACTLY`

## Candidate 3: Segment Range Resolution Deduplication
- **Rival SHA**: `d82d9de0d7f65b5fc9a5efec972c5857501884d5`
- **Location**: `internal/cli/cmd_tag.go:171-215`, `internal/cli/cmd_tag.go:282-320`
- **Evidence**: `computeUntaggedWindow` and `findPrevSegment` contain duplicate 40-line blocks resolving segment boundaries against `uuidToDispIdx` and `uuidToMsgID` with array bounds clamping. `dupl -t 25 internal/cli/cmd_tag.go` flags this duplicate clone. Extracting `resolveSegmentRange` deduplicates the logic and eliminates the clone group.
- **Ruling**: `TRANSPLANT_EXACTLY`

## Candidate 4: Simplify runTagWriteRoutine Verdict Write
- **Rival SHA**: `d82d9de0d7f65b5fc9a5efec972c5857501884d5`
- **Location**: `internal/cli/cmd_tag.go:554-568`
- **Evidence**: In `runTagWriteRoutine`, the check `if source == store.VerdictSourceFloor { return nil }` after calling `store.UpsertVerdict` is unreachable dead code because floor source handling occurs before the upsert. Returning `store.UpsertVerdict(...)` directly simplifies error handling.
- **Ruling**: `TRANSPLANT_EXACTLY`
