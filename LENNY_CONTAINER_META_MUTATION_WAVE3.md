# Lenny `containerMeta` mutation wave 3

## Scope and baseline

Audited commit `d7106e9` (`refactor(index): delete helper-coupled test...`) in a
disposable worktree detached at that commit. The deleted parent test was
reconstructed exactly with:

```sh
git diff d7106e9 d7106e9^ -- internal/index/containers_test.go | git apply
```

The parent test covers ID, source, project, CWD, subagent flag, parent ID,
source size, non-empty fingerprint for an existing file, zero stats for a
missing file, and the returned backing path. Baseline surviving suite:

```sh
CGO_ENABLED=0 go test -race -count=1 ./internal/index
```

PASS, `71.973s` test output / `1:12.28` wall clock (Go reported `71.973s`).

## Mutation results

All mutations were made only in `/tmp/w3-*` disposable worktrees. “Current”
means the post-`d7106e9` surviving package tests; every command used
`CGO_ENABLED=0 go test -race -count=1 ./internal/index`.

| Mutation | Current tests | Restored direct test | Evidence |
| --- | --- | --- | --- |
| `backingFileState`: `size = st.Size() + 1` | SURVIVED (PASS, `70.894s`, wall `1:11.19`) | KILLED (source size `32`, wanted `31`) | Silent metadata corruption is unpinned after deletion. |
| `containerMeta`: `ParentID: ""` | SURVIVED (PASS, `71.262s`, wall `1:12.08`) | KILLED (subagent parent became empty) | Parent linkage is unpinned by surviving tests. |
| `backingFileState`: `fp = ""` | KILLED (FAIL, `42.007s`; incremental append panic) | KILLED (non-empty fingerprint assertion) | Existing incremental freshness tests still catch this stronger fingerprint mutation. |
| `containerMeta`: `rawPath := backingFilePath(c.Path) + ".wrong"` | KILLED (FAIL, `69.883s`, wall `1:10.16`; freshness/tail tests fail) | KILLED (stats zero and returned path wrong) | Existing refresh/tail tests catch an actually wrong backing path. |

For completeness, a wrong-but-non-empty fingerprint (`fp =
"wrong-fingerprint"`) passed both current tests and the restored direct test:
the deleted assertion only requires non-empty fingerprint, not its value. This
is a weakness in the restored test itself, not a false-green result attributable
to deletion.

## Verdict

**HOLD.** Two meaningful mutations (size and parent ID) survive the current
test suite and are killed by the restored direct test. The deleted test removed
real contract coverage. Fingerprint-empty and actual backing-path mutations
remain covered elsewhere. Net production lines: `0`; report-only change:
`+52/-0` lines.

Race detector was included in every run. No source files in the rival Lenny
worktree were edited, staged, or reset.
