package agentproto

import (
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// countingEmbedder records how many times a search asked for an embedding, and
// returns a fixed vector so the fusion path is reachable when it IS asked.
type countingEmbedder struct{ calls int }

func (c *countingEmbedder) Embed(string) []float64 {
	c.calls++
	return []float64{0.1, 0.2, 0.3}
}

// TestSearchSkipsEmbeddingWhenStoreHasNoVectors pins the reason the query
// vector is resolved by a function instead of computed up front.
//
// Real case: embedding a query is a blocking network round-trip to the
// embedding endpoint — about 750ms, against a local search that finishes in
// about 50ms, so it was roughly nine tenths of what a search cost. It was paid
// before any store was opened, which meant it was paid even when the store held
// no vectors to compare the query against. That is exactly what consolidation
// produced: the one store carries no chunk_vec table, so every search bought a
// 750ms vector leg that could not change a single result. Fusing keyword
// anchors against an empty vector set reduces to keyword order, so the cost was
// pure loss, and nothing reported it.
func TestSearchSkipsEmbeddingWhenStoreHasNoVectors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	proj := t.TempDir()
	writeSession(t, proj, "5c2ba140-0000-4000-8000-00000000aa01",
		"9f3e1c20-aaaa-bbbb-cccc-00000000aa01", "a conversation about widget calibration")

	db, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	if _, err := index.ConsolidateFrom([]string{db}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}
	// Indexing never wrote vectors (no embedder was wired), so neither store
	// here has a populated chunk_vec — the live shape after consolidation.

	emb := &countingEmbedder{}
	env := Search("widget calibration", nil, SearchOpts{Limit: 5}, emb)

	if len(env.Results) == 0 {
		t.Fatal("keyword search returned nothing — the fixture never got indexed, so the embed count below proves nothing")
	}
	if emb.calls != 0 {
		t.Errorf("the query was embedded %d time(s) against a store with no vectors — that is a blocking network round-trip bought for nothing", emb.calls)
	}
}

// TestSearchEmbedsAtMostOncePerSearch is the other half: deferring the
// embedding must not turn into embedding once per database. The fan-out walks
// every project's index, and a per-database embed would multiply the one
// expensive call by the number of projects on the machine.
func TestSearchEmbedsAtMostOncePerSearch(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	scope := []view.Scope{}
	for _, text := range []string{"widget calibration in the ledger", "widget calibration for billing"} {
		proj := t.TempDir()
		writeSession(t, proj, "5c2ba140-0000-4000-8000-00000000aa02",
			"9f3e1c20-aaaa-bbbb-cccc-00000000aa02", text)
		if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
			t.Fatalf("EnsureIndexed: %v", err)
		}
		scope = append(scope, view.Scope{Project: paths.ProjectLabel(proj), TDir: proj})
	}

	emb := &countingEmbedder{}
	// A non-nil scope takes the per-project fan-out, not the one store.
	Search("widget calibration", scope, SearchOpts{Limit: 5}, emb)

	if emb.calls > 1 {
		t.Errorf("the query was embedded %d times across %d databases — the round-trip must be memoised, not repeated per database", emb.calls, len(scope))
	}
}
