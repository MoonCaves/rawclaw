# Detached tag publisher findings

## Existing seam to copy

`internal/cli/autosync.go` already provides the needed shape: resolve the
self-executable, open a bounded receipt log, invoke a hidden self-command with
`exec.Command`, call `detach`, close stdin, redirect stdout/stderr, start, and
release without waiting. `SyncConsolidatedFrom` already owns the consolidated
writer fence and is idempotent for a source database, so the child should call
that exact operation rather than introduce a new writer abstraction.

## Contract at risk

`runTagWriteCmd` makes the durable tag write authoritative, then attempts a
synchronous derived-store fold and silently ignores failure. Moving publication
out of the foreground must preserve the existing read-after-write behavior on
successful folds; when publication cannot complete, output and the receipt log
must say that consolidated visibility is pending and identify the exact
recovery command. The detached child must be bounded by the inherited CLI
watchdog, fenced by `SyncConsolidatedFrom`, and safe to rerun.

## Smallest test

Add a focused CLI test that swaps the detached publisher seam, runs a tag write,
and proves the foreground returns after the authoritative write while exactly
one publisher child is requested with the source database and a diagnosable
pending receipt. Add a child test only if the command wiring cannot be proven
through the existing command-tree helpers.

## Files

- `internal/cli/cmd_tag.go`: replace the synchronous best-effort fold with the
  publisher seam and honest output.
- `internal/cli/tagrefresh_test.go`: focused lifecycle/visibility contract test.
- `internal/cli/cli.go`: register the hidden publisher child and ensure the
  existing timeout watchdog bounds it.
- `internal/cli/tagpublish.go` (new only if needed): reuse detach, selfExe,
  receipt log, and the existing fold operation.
- `internal/cli/tagpublish_test.go` (new only if needed): child receipt and
  idempotence coverage.

## Estimated net lines

Production +35 / tests +35 / docs +30; target is the smallest implementation
that keeps the existing synchronous successful-read contract and makes failure
eventual visibility explicit.

