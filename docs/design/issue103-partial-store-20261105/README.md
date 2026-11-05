# Issue #103: partial consolidated-store lookup evidence

Created here from a disposable diagnostic of exact revision
`767be51bcca2bdbccf2f63c5e95d3edcff147032` on 2026-11-05. This note and
[probe_test.go.txt](probe_test.go.txt) are evidence artifacts, not a runtime
change, regression specification, or approval of a lookup policy.

## Observed behavior

Two distinct Claude session IDs share prefix `aaaaaaaa`. One remains live; the
other is indexed, tagged, and then retained after its transcript and project
directory are removed. The explicit lookup scope contains both database paths.
The catalog contains only the live session. Exact retained-session lookup first
verifies its original topic, read reference, summary, and project.

Each control then removes the consolidated database and its WAL/SHM files and
constructs the named state before calling `Topics` with that shared prefix.

| Constructed consolidated state | Observed Topics result |
| --- | --- |
| Absent | Ambiguous prefix, two candidates |
| Live session only, one row | Live topic returned, nil error; retained collision not reported |
| Both sessions folded, two rows | Ambiguous prefix, two candidates |

The focused diagnostic command exited 0 using Go 1.26.3 darwin/arm64. Selected
verbatim diagnostic output (temporary-directory warning lines omitted):

```text
Topics(prefix): hits=[] error=ambiguous session prefix "aaaaaaaa" — 2 matches: aaaaaaaa (live), aaaaaaaa (retained); give a longer prefix ambiguous=true candidates=2
consolidated session count=1
Topics(prefix): hits=[{Topic:live topic Project:live ReadRef:aaaaaaaa:11111111 Routine:false Summary:live session summary}] error=<nil> ambiguous=false candidates=0
consolidated session count=2
Topics(prefix): hits=[] error=ambiguous session prefix "aaaaaaaa" — 2 matches: aaaaaaaa (live), aaaaaaaa (retained); give a longer prefix ambiguous=true candidates=2
```

This is an explicitly constructed partial-store state. It does not establish
how often that state occurs in deployments, its operational severity, SQLite
open counts, performance, or the desired authority policy. This was one focused
plain test, not a race run or full-suite release gate. The stored probe is not a
compiled repository test and its observations must not silently become policy.

## Source boundaries at the tested revision

- `internal/agentproto/locate.go`: `locateSessionWithCatalog` returns nonempty
  `oneStoreCands` before fallback. This explains the live-only result even when
  both explicit database scopes are provided.
- `internal/agentproto/lazyscope_test.go`:
  `TestLocateSessionSkipsScopeListWhenOneStoreAnswers` explicitly expects a
  positive consolidated hit to avoid constructing fallback scopes. The current
  fast path and complete cross-scope prefix ambiguity therefore need to be
  considered together before changing lookup behavior.
- `internal/source/source.go`: `Registration.Lookup` is a full-ID API, not a
  prefix-completeness API. In `internal/source/claude/claude.go`, `lookup` checks
  catalog plus direct `<project>/<id>.jsonl` paths; `Discover` uses
  `ContainedJSONL` and provenance to include subagents. The Topics fallback also
  tries subagent rows when no top-level candidate exists, so an exact adapter
  miss alone cannot replace that fallback unchanged.
- `internal/scopes/scopes.go`: `ArchiveLookup` avoids ingestion but covers
  existing replica caches. Its resume consumer in `internal/cli/cli.go`
  deliberately leaves never-indexed replica sessions unresolved. `Resolve`
  indexes lazy directory scopes, and orphan-scope discovery reconciles caches.
  These helpers do not establish a general non-ingesting, complete Topics lookup.

The probe reuses setup from
`internal/agentproto/topics_session_fallback_contract_test.go`, especially
`TestTopicsSessionPartialCatalogStillDetectsRetainedPrefixCollision`, and calls
existing `writeSession` and `tagInto` test helpers. Its new control loop compares
absent, partially folded, and fully folded consolidation. No production code is
modified. The existing root AGENTS design-note index covers this artifact;
contracts were checked and left unchanged because no runtime contract moved.

## Isolated reproduction

Run from a clone containing the exact tested commit and these evidence files.
The commands archive the tested revision into a new temporary directory and
copy only the diagnostic fixture there. They do not edit the clone, main branch,
or real transcript/data roots. Existing Go module/build caches are reused;
Go may populate those caches normally. The diagnostic itself creates fresh
HOME, Claude, cache, data, and catalog roots for its fixture.

```sh
probe_dir=$(mktemp -d)
module_cache=$(go env GOMODCACHE)
build_cache=$(go env GOCACHE)
git archive 767be51bcca2bdbccf2f63c5e95d3edcff147032 | tar -x -C "$probe_dir"
cp docs/design/issue103-partial-store-20261105/probe_test.go.txt "$probe_dir/internal/agentproto/issue103_partial_store_probe_test.go"
mkdir -p "$probe_dir/isolated/home" "$probe_dir/isolated/data" "$probe_dir/isolated/cache" "$probe_dir/isolated/config" "$probe_dir/isolated/claude" "$probe_dir/isolated/catalog"
(
  cd "$probe_dir" || exit 1
  env HOME="$probe_dir/isolated/home" \
    XDG_DATA_HOME="$probe_dir/isolated/data" \
    XDG_CACHE_HOME="$probe_dir/isolated/cache" \
    XDG_CONFIG_HOME="$probe_dir/isolated/config" \
    CLAUDE_CONFIG_DIR="$probe_dir/isolated/claude" \
    RAWCLAW_CATALOG_DIR="$probe_dir/isolated/catalog" \
    GOMODCACHE="$module_cache" GOCACHE="$build_cache" \
    go test ./internal/agentproto -run '^TestDiagnosticPartialPositiveConsolidated$' -count=1 -v
)
```

The intentionally absent consolidated database can emit SQLite's
`unable to open database file: out of memory (14)` warning. That driver text is
not a host-memory diagnosis. The three control results above are the evidence.
