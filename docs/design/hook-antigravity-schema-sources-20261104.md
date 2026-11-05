# Antigravity hook schema source delta

Date: 2026-11-04. This additive note records the official schema evidence relevant to the historical held candidate `7ba119abbdfc8d4258fc9a19599a8561e5027042`; that hash identifies the candidate under review, not a published current implementation.

Google Antigravity’s Hooks documentation identifies `workspacePaths` as “array of strings” and describes them as “Absolute directory paths representing your mounted workspaces.”[1] Its example contains one path. This establishes the field and broad value domain, but does not specify which entry an integration should select. It therefore does not establish the candidate’s mapping to the first `workspacePaths` entry.[1]

The same documentation defines `invocationNum` as an integer and states: “The 0-indexed sequence number of the current model invocation (the first invocation is 0).”[1] This establishes zero-based sequence semantics. It does not prescribe a decoder guard or handling for negative, malformed, or out-of-range input, so the candidate’s exact nonnegative-value acceptance rule remains a separate decision.[1]

The source is a mutable documentation page, retrieved 2026-11-04. SHA-256 of the fetched HTML: `d1ffa864a3231385db8dd8c59c119534e863a0e22d2851785d3cd4a684e3a44d`. No immutable upstream implementation pin or per-page license statement was identified. This schema evidence does not provide full hook protocol provenance. The prior bounded source review is recorded in [hook-assignment-protocol-sources-20261104.md](hook-assignment-protocol-sources-20261104.md).

## Sources

[1] https://antigravity.google/docs/hooks — Google Antigravity Docs: Hooks
