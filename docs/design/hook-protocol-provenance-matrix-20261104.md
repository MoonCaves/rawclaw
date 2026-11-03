# Issue 89 hook protocol provenance delta

Date: 2026-11-04
Candidate: `7ba119abbdfc8d4258fc9a19599a8561e5027042`
Scope: newly published POSIX quotation and Infisical exporter evidence, applied to the existing provenance review. This is a delta; the prior 13-item rejection matrix is not repeated.

## Delta matrix

| Candidate decision | New evidence | What it supports | What remains open |
| --- | --- | --- | --- |
| POSIX single-quote value encoding in `shellSingleQuote` (`internal/cli/setup.go:457-459` at the candidate) | Infisical CLI `posixShellQuote` and `formatAsDotEnvEval` at pinned commit [`49634ac7f94c9ea02eb7d571fc64577cefe199e5`](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/cmd/export.go#L323-L341). The parent review found the returned expression matches after parameter renaming. | A function-level content match and prior art for POSIX quoting values into `export` statements. It narrows the value-quoting provenance gap. | It does not establish historical copying, authorization, the full RawClaw protocol, output variable names/order, or the JSON-to-assignment composition. |
| Typed records formatted as shell exports | Infisical's typed `Key`/`Value` records and exporter loop [`models/cli.go`](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/models/cli.go#L105-L117), [`export.go`](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/cmd/export.go#L323-L341). | Generic typed-record-to-export formatting, a partial analogue for the candidate's assignment emission in `internal/cli/hookinput.go:39-75`. | Infisical formats secret records; it does not decode hook JSON, select editor-specific fields, or define RawClaw's fixed assignment schema. Its formatter also interpolates keys directly. |
| JSON input decoded into fixed hook assignments | No new matching source. The prior immutable partials remain: Go JSON decoder semantics (`165bc6e7b86782c04ed7475199fe093b2f0f22fc`), Codex SessionStart schema (`0585dcb5dbb276a78909fa8fd0b36310f16973f9`), and Cobra exact argument count (`adbc8813901bba65827259daa8e22ff94ec1f30e`). | These cover generic decoder behavior, specified Codex field names/types, and argument arity respectively. | No cited source composes stdin JSON decoding, source selection, per-hook mappings, and fixed shell assignments. |

Candidate references above use paths and line numbers from the immutable candidate commit, not the current checkout. Relevant candidate ranges: decoder and assignment emission `internal/cli/hookinput.go:39-75`; field mappings `:78-115`; hook call sites and `eval` `internal/cli/setup.go:82-85`, `:144-147`, `:220-223`; quote function `internal/cli/setup.go:457-459`.

## Smallest remaining open set

- **Input acceptance and source selection:** strict one-object-plus-EOF behavior and the allowed `source` values.
- **Per-hook mapping:** selected fields, Antigravity `workspacePaths[0]`, and the `invocationNum >= 0` rule.
- **Output schema:** fixed assignment names/order and JSON companion fields.
- **Failure and trust behavior:** fail-soft defaults, error suppression, and the exact generated-text/`eval` composition and trust boundary.

These are the remaining protocol choices not established by the new function-level and formatter analogues. This list is a compact delta, not a restatement of the broader source-map rejection findings.

## NUL: runtime behavior vs provenance

The Infisical probe demonstrates that raw NUL does not round-trip through shell-variable transport. That probe is a serialization boundary, not evidence that this candidate has a runtime NUL defect. In the candidate, raw shell fields containing NUL are cleared; transcript path and cwd are JSON-marshaled before shell transport. Candidate tests cover NUL session-ID rejection and catalog-path escaping (`internal/cli/hookinput_test.go:89-113`). The implementation rationale for those candidate-specific NUL choices remains a provenance question; the observed probe does not answer it.

## Conclusion

The new evidence narrows the provenance gap for the quoting function and adds a generic typed-record-to-export analogue. It does not source the full JSON-input-to-fixed-hook-assignment protocol or resolve the remaining decisions above. **No adoption verdict.**

This report also makes no publication disposition; it records provenance coverage only.
