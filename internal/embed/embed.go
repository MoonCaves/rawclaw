// Package embed defines the optional vector-channel PORTS — Embedder and
// VectorStore — that the keyword engine degrades around gracefully.
//
// Ship-empty rule: the keyword engine works with NO embedder and NO vector
// store configured. When neither port is wired, searches stay keyword-only —
// no error, no warning, no missing dependency. Adapters (Ollama, OpenAI,
// Voyage, sqlite-vec, …) plug in later.
package embed

import (
	"context"
	"math"
)

// Normalize returns a unit-length copy of vec. Nil, empty, and zero vectors
// are returned unchanged because they cannot be normalized.
func Normalize(vec []float64) []float64 {
	out := append([]float64(nil), vec...)
	if len(out) == 0 {
		return out
	}

	length := 0.0
	for _, value := range out {
		length += value * value
	}
	if length == 0 {
		return out
	}

	length = math.Sqrt(length)
	for i := range out {
		out[i] /= length
	}
	return out
}

// Embedder turns a text string into a dense float vector.
//
// Contract: Embed returns nil to mean "no embedding for this call" — the
// defined no-op signal. Callers MUST handle nil without erroring; the keyword
// path covers the gap. A non-nil return MUST be a non-empty []float64. An
// adapter that always returns nil (the null adapter) is conformant; one that
// sometimes returns nil (backend down) is too. The routing decision must be
// stable for identical input. The caller's context controls cancellation and
// deadlines for any I/O performed by the embedder.
type Embedder interface {
	Embed(ctx context.Context, text string) []float64
}

// BatchEmbedder is an OPTIONAL capability an Embedder may also implement:
// embed many texts in one round trip. It exists because a bulk index over a
// large corpus is round-trip-bound, not compute-bound — one call per message
// against a remote endpoint is orders of magnitude slower than batching.
//
// Contract: a non-nil return MUST have exactly len(texts) entries, positionally
// aligned with the input; any individual entry MAY be nil (the same no-op
// signal as Embed). Returning nil means "batch unavailable or failed" — callers
// MUST fall back to per-item Embed, so a batch failure never loses vectors.
// Callers reach this by type assertion; an Embedder that does not implement it
// is fully conformant. The caller's context controls cancellation and deadlines.
type BatchEmbedder interface {
	EmbedBatch(ctx context.Context, texts []string) [][]float64
}
