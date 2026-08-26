# FIFO catalog-claim raid

Target: `d9474fb`, compared with `821b78d`, base `5b9756b`.

## Finding

CONFIRMED: `d9474fb` changed both generated SessionStart templates to

```sh
if (set -C; : > "$entry") 2>/dev/null || [ ! -e "$entry" ]; then
```

The shell redirection opens the existing path before the fallback test runs.
When `$entry` is a FIFO with no reader, the hook blocks in `: > "$entry"`; a
directory, symlink, or other special path can likewise produce unsafe or
implementation-dependent behavior. The criticism is not theoretical: the
existing-file branch is reached too late to protect non-regular paths.

## Minimal ruling

Keep the fail-soft semantics from `821b78d`, but claim with a same-directory
hard link instead of noclobber redirection: write the complete JSON to a
private temporary regular file, then `ln "$tmp_entry" "$entry"`. `ln` fails
without opening an existing FIFO, directory, symlink, socket, or other special
file. A successful link is the durable regular catalog entry; removing the
temporary name leaves the claimed entry in place. A failed link skips any
existing path and falls back to one best-effort ingest only when the entry is
absent/unreachable.

The same claim block must remain byte-compatible POSIX `sh` in both Claude and
Codex templates. Regression coverage belongs in `catalog_hook_test.go`: new
claim, existing regular entry, FIFO, directory, symlink, and Unix socket under
each available `/bin/sh`, `dash`, and `bash`, with a hard process timeout.
