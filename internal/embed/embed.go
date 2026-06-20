// Package embed defines the optional vector-channel PORTS — Embedder and
// VectorStore — that the keyword engine degrades around gracefully.
//
// Ship-empty rule: the keyword engine works with NO embedder and NO vector
// store configured. When neither port is wired, searches stay keyword-only —
// no error, no warning, no missing dependency. Adapters (Ollama, OpenAI,
// Voyage, sqlite-vec, …) plug in later.
package embed

// Embedder turns a text string into a dense float vector.
//
// Contract: Embed returns nil to mean "no embedding for this call" — the
// defined no-op signal. Callers MUST handle nil without erroring; the keyword
// path covers the gap. A non-nil return MUST be a non-empty []float64. An
// adapter that always returns nil (the null adapter) is conformant; one that
// sometimes returns nil (backend down) is too. The routing decision must be
// stable for identical input.
type Embedder interface {
	Embed(text string) []float64
}

// VectorStore stores and retrieves dense float vectors keyed by opaque string
// ID. The caller owns ID generation and dedup. When no VectorStore is wired,
// the keyword engine skips the vector path entirely — a missing store is never
// fatal.
type VectorStore interface {
	// Upsert inserts or replaces the vector for id (last write wins, no error).
	// The store does not validate dimensions.
	Upsert(id string, vector []float64)
	// KNN returns up to k nearest-neighbour IDs to vector, nearest first. May
	// return fewer than k (or an empty slice) when the store holds fewer
	// vectors.
	KNN(vector []float64, k int) []string
}
