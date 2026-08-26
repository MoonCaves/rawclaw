# Lenny b0d9e0f hook path-escape reproduction — wave 3

Verdict: **NOT REPRODUCED — CLEAR** for the specific claim that a slash or
dot-dot session ID creates an escaped catalog claim, clobbers a special file,
or leaves a nested path behind. `b0d9e0f` remains structurally risky: it
interpolates the raw session ID into `entry`, `tmp_dir`, and `tmp_entry`, and
the hostile traversal probe emitted a shell redirection diagnostic. The
observed runs fail soft and leave no escaped artifact.

## Isolation

- The rival worktree was not read or modified.
- Detached worktrees from immutable `b0d9e0f` and `bd8346c` were used under
  `/tmp`.
- Every probe used an isolated temporary root/catalog, `RAWCLAW_CATALOG_DIR`,
  a fake `rawclaw` that only logs arguments, and no real HOME state.
- Available shells were `sh` and `dash`; Codex used installed `python3` only
  for its JSON envelope.

## Commands and gates

Both commits passed the focused matrix under normal and race tests:

```text
CGO_ENABLED=0 go test -count=1 -run '^TestPrimeScripts_(SessionStartHostilePathMatrix|SessionStartCatalogClaimIsPathSafe)$' ./internal/cli
CGO_ENABLED=0 go test -race -count=1 -run '^TestPrimeScripts_(SessionStartHostilePathMatrix|SessionStartCatalogClaimIsPathSafe)$' ./internal/cli
```

Observed timings/results:

```text
b0d9e0f: ok 2.785s; race ok 3.400s
bd8346c: ok 1.367s; race ok 2.404s
```

The b0d9e0f matrix covered `sh`/`dash`, Claude/Codex, and new, regular,
FIFO, directory, injected-directory, symlink, dangling symlink, socket, and
missing-parent paths. Its separate test did not include traversal. The
bd8346c matrix added traversal and asserted no escaped root entries or leaked
temporary claims.

## Direct traversal execution

Rendered Claude and Codex scripts from b0d9e0f were run under both shells with
session ID `x/../../outside` and a pre-existing `catalog/.tmp.x` directory.
All four exited status 0. Claude emitted a redirection diagnostic; Codex
emitted the same diagnostic plus valid `hookSpecificOutput` JSON. The observed
path included:

```text
.../catalog/.tmp.x/../../outside.<pid>/x/../../outside
```

The isolated root still contained only `bin`, `catalog`, and the script; the
catalog still contained only `.tmp.x`; no outside artifact, catalog entry, or
special-file mutation was observed. The fake ingest log was empty because the
traversal write failed before a claim was established. The hook nevertheless
failed soft with status 0.

| commit | target | shell | status | result |
|---|---|---|---:|---|
| b0d9e0f | Claude | sh | 0 | diagnostic; no escape |
| b0d9e0f | Claude | dash | 0 | diagnostic; no escape |
| b0d9e0f | Codex | sh | 0 | diagnostic + valid JSON; no escape |
| b0d9e0f | Codex | dash | 0 | diagnostic + valid JSON; no escape |

## Special files and source delta

The b0d9e0f hostile matrix passed for existing regular file, FIFO, directory,
symlink, dangling symlink, Unix socket, and missing-parent cases under both
shells/scripts. Existing paths were not changed and no nested artifacts
remained. The integrated bd8346c matrix passed those checks plus traversal.

Between the commits, relevant source/test changes were:

```text
internal/cli/cmd_ingest_test.go  +169 / -132
internal/cli/setup.go             +178 / -90
```

The integrated fix validates flat catalog keys, routes invalid IDs to
fail-soft ingest, and uses a PID-only temporary claim directory. No full
repository gate was run because this was a report-only reproduction lane.

## Conclusion

**NOT REPRODUCED — CLEAR.** The requested escaped-catalog, special-file
clobber, and persistent nested-path claim was not observed on b0d9e0f with the
hostile IDs and shells above. b0d9e0f still has a hardening gap in source shape
and weak traversal coverage: malformed path construction reaches the shell and
produces diagnostics. bd8346c closes that tested path and passes its explicit
traversal assertion.
