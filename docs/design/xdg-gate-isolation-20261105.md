# XDG fixture gate isolation — 2026-11-05

Source: isolated synthetic probes against `1a82c5d`; this note owns the reproduction evidence. Runtime paths remain defined by `internal/paths/paths.go`.

## Finding

A temporary `HOME` does not isolate durable RawClaw data when `XDG_DATA_HOME` is inherited. `paths.TombstonePath()` honors XDG; `lifecycle.TombstoneIDs("", ...)` and `LoadTombstones("")` use it.

`archive.newTestHome` sets HOME but leaves XDG inherited. `TestPushLocal_TombstonedFileNotResurrected` writes the synthetic `sess-zombie` tombstone; `TestPushLocal_DeleteReportsOnce` reuses that ID. Its initial push skips the already tombstoned file, so the later propagation push removes zero files.

`TestCodexUnionsOrphanedDBs_TombstonedExcluded` indexes a synthetic session into the XDG vault, then writes a tombstone at the HOME fallback location. With XDG set, that fallback directory does not exist. Creating it alone would still write outside the actual tombstone path.

## Reproduction receipts

Go 1.27.3, darwin/arm64, `CGO_ENABLED=0`, race detector enabled. Each command used a fresh temporary outer HOME; each XDG-set command used a distinct fresh temporary XDG root. Paths below are normalized to `$TEST_TMP`; no real user vault was accessed.

With XDG unset:

```text
go test -race -count=1 -v -run '^TestCodexUnionsOrphanedDBs_TombstonedExcluded$' ./internal/scopes
=== RUN   TestCodexUnionsOrphanedDBs_TombstonedExcluded
--- PASS: TestCodexUnionsOrphanedDBs_TombstonedExcluded (0.35s)
PASS
ok  github.com/MoonCaves/rawclaw/internal/scopes 2.319s
exit=0
```

With a fresh XDG root:

```text
go test -race -count=1 -v -run '^TestCodexUnionsOrphanedDBs_TombstonedExcluded$' ./internal/scopes
=== RUN   TestCodexUnionsOrphanedDBs_TombstonedExcluded
    scopes_test.go:294: open $TEST_TMP/001/.local/share/rawclaw/.deleted: no such file or directory
--- FAIL: TestCodexUnionsOrphanedDBs_TombstonedExcluded (0.29s)
FAIL
FAIL github.com/MoonCaves/rawclaw/internal/scopes 1.111s
FAIL
exit=1
ambient_XDG_files=[rawclaw/transcripts/deadsess.jsonl, rawclaw/transcripts/deadsess.meta.json]
```

Archive control with a fresh XDG root:

```text
go test -race -count=1 -v -run '^TestPushLocal_DeleteReportsOnce$' ./internal/archive
=== RUN   TestPushLocal_DeleteReportsOnce
--- PASS: TestPushLocal_DeleteReportsOnce (3.72s)
PASS
ok  github.com/MoonCaves/rawclaw/internal/archive 5.895s
exit=0
ambient_XDG_files=[rawclaw/.deleted]
```

Archive pair with a fresh XDG root:

```text
go test -race -count=1 -v -run '^TestPushLocal_(TombstonedFileNotResurrected|DeleteReportsOnce)$' ./internal/archive
=== RUN   TestPushLocal_TombstonedFileNotResurrected
--- PASS: TestPushLocal_TombstonedFileNotResurrected (1.05s)
=== RUN   TestPushLocal_DeleteReportsOnce
    delete_test.go:267: propagation push Removed = 0, want 1
--- FAIL: TestPushLocal_DeleteReportsOnce (1.88s)
FAIL
FAIL github.com/MoonCaves/rawclaw/internal/archive 3.685s
FAIL
exit=1
ambient_XDG_files=[rawclaw/.deleted]
```

## Correction boundary

Use native `t.Setenv` to pin XDG to each temporary HOME's `.local/share` in the archive helper and scope fixtures that set HOME directly. Preserve assertions, fixtures, and runtime behavior. The owning AGENTS contract was checked and requires no change. These probes establish the two fixture failures; they do not establish full-suite containment.

## Focused correction check

After the fixture setup correction, the same Go 1.27.3 race run used a fresh outer HOME and a fresh inherited XDG guard:

```text
go test -race -count=1 -v -run '^Test(CodexUnionsOrphanedDBs_TombstonedExcluded|PushLocal_(TombstonedFileNotResurrected|DeleteReportsOnce))$' ./internal/scopes ./internal/archive
=== RUN   TestCodexUnionsOrphanedDBs_TombstonedExcluded
--- PASS: TestCodexUnionsOrphanedDBs_TombstonedExcluded (0.35s)
PASS
ok  github.com/MoonCaves/rawclaw/internal/scopes 2.754s
=== RUN   TestPushLocal_TombstonedFileNotResurrected
--- PASS: TestPushLocal_TombstonedFileNotResurrected (1.06s)
=== RUN   TestPushLocal_DeleteReportsOnce
--- PASS: TestPushLocal_DeleteReportsOnce (2.03s)
PASS
ok  github.com/MoonCaves/rawclaw/internal/archive 5.791s
exit=0
ambient_XDG_files=[]
```

This check covers the three named tests; independent package and full-suite gates remain separate.
