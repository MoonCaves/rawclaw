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

## Prospective test source clarification (2026-11-06)

The original two test citations above did not contain the staged append loop,
pending-dependent watermark expectation, or full-message DeepEqual comparison.
They do not establish that those constructs were copied. The following sources
describe the separately reviewed prospective re-port, not the original test's
history. All ranges pin MoonCaves/rawclaw commit
`38a52389987296ba5ca6ca76813fe515534c382b` (MIT), fetched over HTTPS for this review.

- `internal/index/tail_edge_test.go` L362-L431 supplies the sequential append
  table, named subtests, ingestion and per-stage assertions. Its Claude fixtures
  and expected fields adapt to the same six existing Codex tails and counters.
- `internal/index/incremental_test.go` L178-L181 supplies named JSONL input
  assembly; L493-L493 supplies `int64(len(initial))`. Explicit known-input
  prefixes bind each stage's expected offset, including the preceding complete
  prefix for an incomplete append. No runtime completeness decision is added.
- `internal/index/incremental_test.go` L65-L71 supplies the persisted-offset
  query and unconditional inequality. Its stored-size operand adapts to the
  declared fixture prefix; that donor itself tests a complete append, not a
  partial Codex watermark. Malformed complete bytes remain in the final prefix.
- `internal/index/tail_edge_test.go` L433-L456 supplies ordered collection,
  fatal length guard and positional comparison. `internal/index/testhelpers_test.go`
  L264-L299 supplies the ordered Role/Text/TS/TSISO/UUID projection and comparable
  full-message inequality. The real Codex reader remains the independent oracle;
  row errors and persisted session message count remain checked.
- `internal/source/codex/codex_test.go` L201-L228 supplies real-reader fixture
  and ordinal assertions; `internal/index/incremental_test.go` L402-L442 supplies
  malformed-complete fallback. Per-stage counter resets retain expected zero/one
  counts instead of that donor's cumulative full-reindex count.

The prospective port preserves the same six tail byte strings and all counter,
offset, full-message/order, row-error and session-count assertions. Its source
mapping requires an actual re-port and subsequent checks; this clarification
alone does not resolve the historical test-provenance finding.
