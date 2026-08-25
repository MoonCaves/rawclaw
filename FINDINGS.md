# Ponytail Review Findings: internal/archive/** and internal/paths/**

internal/archive/foreign.go:L26-40,L79-93: shrink: ForeignProjectMatches and ForeignSessionMatches duplicate clone check and loop. Extract a.filterForeignMachines helper and reuse cloneUsable.
internal/archive/foreign.go:L88-120: stdlib: foreignMachineHasSession manual search loops over slices. slices.ContainsFunc.
internal/archive/layout.go:L37-56: stdlib: sanitizeMachineName manual strings.Builder and rune loop. strings.Map.
internal/archive/push.go:L576-583: shrink: isRejectedPush iterates newly allocated slice of 4 strings. Direct boolean disjunction without allocation.
internal/archive/push.go:L602-608,internal/archive/pull.go:L97-103,internal/archive/tagconflict.go:L107-114: shrink: stampPush, stampPull, stampTagIngest duplicate stamp writing logic. [RESTORED: split back per review ruling to preserve distinct Chtimes/write semantics]
internal/archive/scopes.go:L258-263,338-343: stdlib: manual map key extraction loop + sort.Strings. slices.Sorted(maps.Keys(byCWD)).
internal/archive/scopes.go:L294-305: stdlib: hasTopLevelJSONL manual loop over directory entries. slices.ContainsFunc.
internal/archive/scopes.go:L400-419: stdlib: sanitizeDBSegment manual strings.Builder and rune loop. strings.Map.
internal/archive/tagconflict.go:L30-39: stdlib: writeTagConflicts manual map + slice + sort.Strings to deduplicate and sort. slices.Clone + slices.Sort + slices.Compact (0 map allocs).
internal/archive/tagconflict.go:L62-74: stdlib: readTagConflicts manual strings.Split + strings.TrimSpace loop. [RESTORED: reverted to line-based parsing per review ruling to preserve state-file grammar]
internal/archive/tagingest.go:L112-119: stdlib: hasRealSegment manual loop over slice. slices.ContainsFunc.
internal/archive/tagingest.go:L126-128: stdlib: segHash make + copy + sort.Slice reflection. slices.Clone + slices.SortFunc.
internal/paths/paths.go:L59-78: shrink: TranscriptsRoot and CatalogDir duplicate XDG data home fallback logic. Shared rawclawDataDir helper.
internal/paths/paths.go:L150,156,276,396,453,471: stdlib: sort.Strings on slice. slices.Sort, removes sort import.
internal/paths/paths.go:L237-243,448-456: shrink: firstTopLevelJSONL returns a 1-element slice for a 1-iteration loop in DirCWD. Return string and check directly.
internal/paths/paths.go:L501-513: stdlib: expandHome uses slice indexing path[2:]. strings.TrimPrefix(path, "~/").

## Accepted Deviations
None. All merge-gate findings were ruled FIX-RESTORE and restored.

## Dupl Clones in Test Files (Outside Fence / Report-Only This Wave)
- internal/archive/ssh_test.go:26-43 vs 45-62 (dupl clone in test fixtures)
- internal/paths/paths_test.go:611-647 vs 649-685 vs 755-771 vs 773-789 (dupl clones in catalog test fixtures)

## Cross-Package Dedup Opportunities (Outside Fence - Reported Only)
- isDir helper is duplicated across internal/paths, internal/archive, and internal/cli.
- Test fixture setup (home/env isolation) is repeated across internal/archive/*_test.go and internal/paths/paths_test.go.

net: -110 lines possible.
