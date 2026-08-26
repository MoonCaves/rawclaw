# Detached publication best-effort contract

Base: `8c8216e`; challenge: `987c6a3`.

## Ruling

The receipt gap is real, but the safe scope is contract wording only. `publication queued` currently sounds stronger than `exec.Cmd.Start` plus detached ownership proves. It must explicitly say that publication is best-effort detached work and that the terminal child receipt can be absent if the process environment disappears before child entry.

Do not add fake terminal receipts, parent waits, in-memory supervisors, or a new queue. No recovery/retry command exists in this candidate, so wording must not promise one.

## Net-line target

Near-zero product delta: replace the single foreground receipt wording and pin it with the smallest output/help regression test. No new runtime dependency or architecture.
