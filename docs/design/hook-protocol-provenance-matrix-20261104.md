# Issue 89 hook protocol provenance delta

Date: 2026-11-04
Candidate: `7ba119abbdfc8d4258fc9a19599a8561e5027042`
Scope: immutable hook-input and sourceable-assignment partials found during the 2026-11-04 follow-up, applied to the existing provenance review. This is a delta; the prior rejection matrix is not repeated.

## Delta matrix

| Candidate decision | New evidence | What it supports | What remains open |
| --- | --- | --- | --- |
| POSIX single-quote value encoding in `shellSingleQuote` (`internal/cli/setup.go:457-459` at the candidate) | Infisical CLI `posixShellQuote` and `formatAsDotEnvEval` at pinned commit [`49634ac7f94c9ea02eb7d571fc64577cefe199e5`](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/cmd/export.go#L323-L341). The parent review found the returned expression matches after parameter renaming. | A function-level content match and prior art for POSIX quoting values into `export` statements. It narrows the value-quoting provenance gap. | It does not establish historical copying, authorization, the full RawClaw protocol, output variable names/order, or the JSON-to-assignment composition. |
| Typed records formatted as shell exports | Infisical's typed `Key`/`Value` records and exporter loop [`models/cli.go`](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/models/cli.go#L105-L117), [`export.go`](https://github.com/Infisical/cli/blob/49634ac7f94c9ea02eb7d571fc64577cefe199e5/packages/cmd/export.go#L323-L341). | Generic typed-record-to-export formatting, a partial analogue for the candidate's assignment emission in `internal/cli/hookinput.go:39-75`. | Infisical formats secret records; it does not decode hook JSON, select editor-specific fields, or define RawClaw's fixed assignment schema. Its formatter also interpolates keys directly. |
| JSON input decoded into fixed hook assignments | No exact end-to-end match surfaced. New partials are summarized below; the prior immutable partials remain: Go JSON decoder semantics (`165bc6e7b86782c04ed7475199fe093b2f0f22fc`), Codex SessionStart schema (`0585dcb5dbb276a78909fa8fd0b36310f16973f9`), and Cobra exact argument count (`adbc8813901bba65827259daa8e22ff94ec1f30e`). | These cover generic decoder behavior, specified Codex field names/types, and argument arity respectively. | No cited source composes stdin JSON decoding, source selection, per-hook mappings, and fixed shell assignments. |

Candidate references above use paths and line numbers from the immutable candidate commit, not the current checkout. Relevant candidate ranges: decoder and assignment emission `internal/cli/hookinput.go:39-75`; field mappings `:78-115`; hook call sites and `eval` `internal/cli/setup.go:82-85`, `:144-147`, `:220-223`; quote function `internal/cli/setup.go:457-459`.

## Smallest remaining open set

- **Input acceptance and source selection:** strict one-object-plus-EOF behavior and the allowed `source` values.
- **Per-hook mapping:** selected fields, Antigravity `workspacePaths[0]`, and the `invocationNum >= 0` rule.
- **Output schema:** fixed assignment names/order and JSON companion fields.
- **Failure and trust behavior:** fail-soft defaults, error suppression, and the exact generated-text/`eval` composition and trust boundary.

These are the remaining protocol choices not established by the new function-level and formatter analogues. This list is a compact delta, not a restatement of the broader source-map rejection findings.

## NUL: runtime behavior vs provenance

The Infisical probe demonstrates that raw NUL does not round-trip through shell-variable transport. That probe is a serialization boundary, not evidence that this candidate has a runtime NUL defect. In the candidate, raw shell fields containing NUL are cleared; transcript path and cwd are JSON-marshaled before shell transport. Candidate tests cover NUL session-ID rejection and catalog-path escaping (`internal/cli/hookinput_test.go:89-113`). The implementation rationale for those candidate-specific NUL choices remains a provenance question; the observed probe does not answer it.

## Additional immutable sources checked 2026-11-04

### Fixed sourceable assignment from JSON stdin: Trail of Bits

At `trailofbits/skills` commit [`711af095215ad4ecde5b90cd1d0124b3f1c4ca9e`](https://github.com/trailofbits/skills/tree/711af095215ad4ecde5b90cd1d0124b3f1c4ca9e), [`persist-session-id.sh` lines 9-32](https://github.com/trailofbits/skills/blob/711af095215ad4ecde5b90cd1d0124b3f1c4ca9e/plugins/gh-cli/hooks/persist-session-id.sh#L9-L32) reads JSON from stdin with `jq`, allowlists one `session_id`, and writes a fixed `export CLAUDE_SESSION_ID=...` line to the configured `CLAUDE_ENV_FILE`. It checks existing `.sh` files to avoid writing a duplicate. [`session-hooks.bats` lines 11-50](https://github.com/trailofbits/skills/blob/711af095215ad4ecde5b90cd1d0124b3f1c4ca9e/plugins/gh-cli/hooks/session-hooks.bats#L11-L50) covers malformed/empty JSON, missing environment-file configuration, successful assignment output, and metacharacter rejection. The commit resolved as `711af095215ad4ecde5b90cd1d0124b3f1c4ca9e HEAD` from `git ls-remote`.

This is the closest immutable protocol precedent found in this pass: JSON stdin to one fixed, sourceable export. Its producer is Bash (`[[ ]]`, `=~`, `pipefail`), depends on `jq`, allowlists a single field, and writes to a provider-managed environment file. It does not establish RawClaw's POSIX producer, zero extra runtime dependencies, multi-provider field mapping, arbitrary JSON string quoting, or generated assignment output evaluated in the same hook.

### Provider input examples: Anthropic and OpenAI Codex

At [`anthropics/claude-code` commit `26382c08758260c9f5deb2019d8fe851f89fa9ae`](https://github.com/anthropics/claude-code/blob/26382c08758260c9f5deb2019d8fe851f89fa9ae/examples/hooks/bash_command_validator_example.py#L56-L74), the Python example loads JSON from stdin and reads `tool_name`, `tool_input`, and `command`. It establishes a provider-specific JSON input example, but emits no fixed shell assignment.

At [`openai/codex` commit `b34808060a51e98b8cb3dcf77824703757737cda`](https://github.com/openai/codex/blob/b34808060a51e98b8cb3dcf77824703757737cda/codex-rs/core/tests/suite/hooks.rs#L400-L411), a SessionStart test hook loads JSON stdin and reads `hook_event_name` and `source`. The same pinned test file [lines 1624-1636](https://github.com/openai/codex/blob/b34808060a51e98b8cb3dcf77824703757737cda/codex-rs/core/tests/suite/hooks.rs#L1624-L1636) demonstrates reading `source` and emitting a JSON hook response. The pinned [`SessionStartCommandInput` schema, lines 497-510](https://github.com/openai/codex/blob/b34808060a51e98b8cb3dcf77824703757737cda/codex-rs/hooks/src/schema.rs#L497-L510), documents `session_id`, `transcript_path`, `cwd`, `hook_event_name`, and `source`. These examples support provider input fields and JSON response behavior, not POSIX assignment output or shell evaluation.

No immutable Antigravity source repository or example was identified in this bounded search. The queried GitHub API endpoint returned `HTTP 404`; that is a search limit, not global absence evidence.

### Bounded result

The immutable sources add one close sourceable-export analogue and provider-specific JSON input examples. None covers the complete RawClaw chain of JSON decoding, Claude/Codex/Antigravity mappings, fixed assignment names/order, fail-soft defaults, and safe evaluation from a POSIX producer without an added JSON runtime dependency. This search does not establish global absence.

## Verification receipt

```text
$ git ls-remote https://github.com/trailofbits/skills.git HEAD
711af095215ad4ecde5b90cd1d0124b3f1c4ca9e	HEAD
$ curl -sS --max-time 25 -o /dev/null -w 'HTTP %{http_code}\n' https://api.github.com/repos/google/antigravity/commits/HEAD
HTTP 404
```

## Conclusion

The new evidence narrows the provenance gap for quoting and adds a close sourceable-export analogue plus provider input examples. It does not source RawClaw's full POSIX, no-extra-runtime, multi-provider JSON-to-fixed-assignment protocol or resolve the remaining decisions above. **No adoption verdict.**

This report also makes no publication disposition; it records provenance coverage only.
