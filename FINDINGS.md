# Codex append-tail metadata parity (2026-11-06)

Source created here: a public-safe source and reproduction record.

- `shrink:` `internal/index/tail.go` rejects valid Codex metadata and empty
  normalized messages, forcing full reindexing. The full reader skips both.
- Donor: [MoonCaves/rawclaw, immutable 38a52389987296ba5ca6ca76813fe515534c382b,
  codex.go L245-L248](https://github.com/MoonCaves/rawclaw/blob/38a52389987296ba5ca6ca76813fe515534c382b/internal/source/codex/codex.go#L245-L248), MIT.
  Fresh HTTPS bytes have SHA256
  `8942c05bb123eee3a4bcb18fee72647d66d9522689154aa114ecc1a8533104a7`.
  Copy its `!ok || text == ""` skip guard; adapt loop `continue` to callback
  `return nil`. Leave the other source branches and strict legacy helper alone.
- Regression method: synthetic JSONL only. Compare ordered append results with
  the full Codex reader, including UUIDs; assert metadata-only watermark advance,
  incomplete-record no-op/completion, and complete invalid-JSON fallback.
- Test scaffolding and count origins: same pinned commit,
  `tail_edge_test.go` L205-L255 (Codex incomplete fixture, zero/one ingest counts),
  `incremental_test.go` L173-L243 (append indexing, count/ordered UUID assertions).
  Derive message and byte totals from the fixture/full reader rather than thresholds.
- Disagreement: a reproducible metadata fallback does not establish the cause or
  duration of an earlier live timeout. That historical attribution remains open.

The existing adapter owns normalization and UUID ordinals; this change only
aligns the incremental callback with its full-reader acceptance behavior.
