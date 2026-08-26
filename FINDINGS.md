# Detached publication best-effort contract

Base: `8c8216e`; challenge: `987c6a3`.

## Ruling

The receipt gap is real, but the safe scope is contract wording only. `publication queued` currently sounds stronger than `exec.Cmd.Start` plus detached ownership proves. It must explicitly say that publication is best-effort detached work and that the terminal child receipt can be absent if the process environment disappears before child entry.

Do not add fake terminal receipts, parent waits, in-memory supervisors, or a new queue. No recovery/retry command exists in this candidate, so wording must not promise one.

## Net-line target

Near-zero product delta: replace the single foreground receipt wording and pin it with the smallest output/help regression test. No new runtime dependency or architecture.

## Bounded prior-art ruling

The cumulative ledger was read through watermark `20260827T000000Z`; its existing recommendations for durable terminal state remain partial/pending and were not rewritten from this lane. Additional sources inspected:

- `https://microservices.io/patterns/data/transactional-outbox.html` — Chris Richardson, Transactional Outbox, accessed 2026-08-27 — commit an event with the business transaction, then a separate relay retries delivery — relevant mechanism, but it requires a durable queue/relay absent from this binary.
- `https://www.freedesktop.org/software/systemd/man/latest/systemd-run.html` — systemd-run, systemd 261.2 docs, accessed 2026-08-27 — transient service units retain manager-owned lifecycle/status — durable terminal ownership, but requires an external daemon and is outside RawClaw’s core.
- `https://www.sqlite.org/lang_transaction.html` — SQLite Transactions, Last-Modified 2026-08-24, accessed 2026-08-27 — transaction commit/rollback serializes local writes — protects publication atomicity, but cannot report a process that vanished before opening its transaction.
- `https://docs.temporal.io/workflows` — Temporal Workflow docs, accessed 2026-08-27 — workflow history is the source of truth and workers replay durable state — strongest terminal-history precedent, but requires a service/worker runtime.

Ruling: every mature solution that guarantees a terminal result adds durable ownership beyond a detached child. Under the one-static-binary/no-daemon constraint, wording is the smallest honest correction; no runtime mechanism should be invented here.
