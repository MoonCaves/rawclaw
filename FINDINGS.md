# Accepted ponytail findings

- Finding 10 — ACCEPT: `internal/durable/durable.go:sanitize` can use `strings.Map` with the same portable-rune allowlist and `'-'` replacement; preserve dot-only prefixing and path containment.
- Finding 18 — ACCEPT: `internal/durable/durable.go:recordType` can use `slices.Contains(parse.IndexableTypes, role)`; preserve the `"system"` fallback.

Both changes are localized to `internal/durable/durable.go`; existing durable path, rendering, and rebuild-contract tests are the guard.
