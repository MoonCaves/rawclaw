# Hostile audit: Ozzy `37ec96b` session-catalog hook safety

## Verdict

**ACCEPT WITH ADVISORIES for independent transplant and gate.**

The candidate fixes the concrete path-safety defect in the `b944d08` parent:
catalog claim paths are now derived only from a restricted flat filename, while
the original session ID remains the argument to `ingest`. The claim publication
uses a same-directory hard link, so it does not overwrite an existing regular
file, FIFO, directory, or symlink. I found no counterexample that escapes the
catalog directory or mutates an existing entry.

The result is not a claim that arbitrary session IDs are deduplicated. IDs that
are empty, dot-prefixed, or contain characters outside
`[A-Za-z0-9._-]` deliberately bypass the catalog and launch fail-soft ingest on
each hook invocation. That is safe from path traversal but can duplicate work;
the product contract should keep this trade-off explicit.

## Immutable identity and scope

- Candidate: `37ec96bebb2a8317617544836ef9730149e1f0d4`
- Parent: `b944d082e9b8d02611b018a25ce9a049066629fc`
- Common ancestor with current tip: `5b9756b2200ff6bd670f07407407d84d9f42d84b`
- Current comparison tip: `0d1da19c4c21961b86cb3ca84ed047d941c83ed3`
- Candidate change: `internal/cli/setup.go` and `internal/cli/cmd_ingest_test.go`
- Candidate diff: `setup.go` +60/-28; tests +157/-0; net production +32,
  net tests +157, docs 0, relative to `b944d08`

The candidate is based on `b944d08`, not the current integration tip. A
transplant must therefore carry only the path-safety hunk and its tests after
resolving the surrounding hook changes; adopting the whole ancestor chain
would include unrelated refactors and documentation.

## Findings by requested attack surface

### Path safety and namespace collisions

`setup.go:50-53` copies `session_id` to `catalog_session_id` and clears it
unless it is nonempty, not dot-prefixed, and composed solely of ASCII letters,
digits, `.`, `_`, or `-`. `setup.go:67-70` then constructs the catalog entry
from that sanitized value. Traversal input such as `x/../../outside` therefore
cannot become a path component; it takes the direct-ingest branch instead.

The candidate also changes the temporary path from a session-ID-derived path
to `catalog/.tmp.$$` (`setup.go:76-77`). This removes traversal from the temp
path and preserves per-process isolation. The remaining caveat is that a
pre-existing `.tmp.$$` causes a safe fallback to ingest, not a claim; it does
not overwrite that directory.

### Existing special files and publication atomicity

The claim is created in a private temporary directory and published with
`ln "$tmp_entry" "$catalog_dir"` (`setup.go:79-92`). The destination is the
catalog directory, so the source basename is the sanitized session ID. The
hard link is an atomic create-if-absent operation on the same filesystem; an
existing FIFO, directory, regular file, or symlink makes `ln` fail, after which
the `[ -e "$entry" ] || [ -L "$entry" ]` check suppresses duplicate ingest.
The cleanup removes only the temporary source path. No `mv -f` overwrite
remains.

### Fail-soft behavior and detached children

Invalid IDs launch `nohup "$RAWCLAW" ingest "$session_id"` and continue to the
banner (`setup.go:64-66`). Valid IDs launch ingest only after winning the hard
link (`:94-100`), or after an absent/unclaimable entry is classified as a
nonexistent marker. All child output is discarded and hook failures remain
silent, matching the hook's fail-soft contract.

The new matrix test adds `trap 'wait' 0` to the generated scripts before
checking the stub log. This closes the known false-green window from rejected
`b0d9e0f`; `25b8d376` independently identified the same detached-child issue.
The trap is test-only and does not alter production hook detachment.

### Test coverage limits

The candidate matrix covers both Claude and Codex, `sh` and `dash`, and FIFO,
directory, and traversal cases. It does not cover Unicode IDs, backslash and
quote parsing, a dangling symlink with a matching name, an unwritable catalog,
or a pre-existing `.tmp.$$`; those are advisory gaps rather than observed
failures. The allowlist and non-overwriting hard-link path make the first four
safe by inspection, while the last two deliberately fall back to ingest.

## Verification performed

On a detached worktree at candidate `37ec96b`:

```sh
/usr/bin/time -p env CGO_ENABLED=0 go test -race -count=3 -shuffle=on ./internal/cli -run 'TestPrimeScripts_(SessionStartCatalogClaimIsPathSafe|SessionStartDeduplicatesConcurrentIngest|StopLaunchDetachedPrewarm)' -v
```

**PASS**. All three repetitions passed across Claude/Codex and sh/dash. Go
package time was `8.706s`; wall time was `11.51s`.

```sh
/usr/bin/time -p env CGO_ENABLED=0 go test -race -count=1 ./internal/cli/...
```

**PASS**. Package time was `65.448s`; wall time was `65.95s`.

`graphify reflect --if-stale` completed, but the worktree has no
`graphify-out/graph.json`; `graphify query` was therefore unavailable. No
Graphify claim was used as evidence.

## Disposition

Proceed to an independent transplant/gate, preserving the candidate's path
validation and hard-link publication together. Keep the invalid-ID repeated
ingest behavior documented, and add a targeted test only if the product later
requires deduplication for nonconforming provider IDs.
