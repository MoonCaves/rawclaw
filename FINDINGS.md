# Detached publication terminal-receipt attack

Base/candidate: `8c8216e` (winner). Challenge/report: `987c6a3`.

## Ruling

**VALID GAP; report-only, no product edit.** `spawnTagPublishChild` returns nil after `exec.Cmd.Start` and `Process.Release`; the foreground command then prints `publication queued`. The only terminal `published`/error line is emitted by the detached child. A child killed or lost after `Start` and before entering `runTagPublishChild` leaves no terminal result. This is an OS/process-ordering gap, not a flaky timing claim.

## Hostile contract and net-line target

The foreground receipt may claim only that launch was accepted. A durable terminal result requires an owner that survives the foreground process (persistent queue, retry worker, or equivalent). Adding another log line or waiting in the parent cannot guarantee completion while preserving detached return. Target net product lines: **0**; do not add a fake receipt or an unowned in-memory supervisor.

## Attack

Use the existing `spawnTagPublish` seam to model `Start == nil` followed by child disappearance, then inspect the append-only publication log. Verify that `queued` can exist with no terminal line, while ordinary child execution produces a terminal line. Compare patch identity against `987c6a3`; preserve the sovereign zero-dependency core.
