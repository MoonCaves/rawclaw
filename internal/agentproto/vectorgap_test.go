package agentproto

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/semantic"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// fakeCoverageEmbedder provides fixed vector embeddings for test messages.
type fakeCoverageEmbedder struct {
	vecs map[string][]float64
}

func (f fakeCoverageEmbedder) Embed(_ context.Context, text string) []float64 {
	if v, ok := f.vecs[text]; ok {
		return v
	}
	return []float64{0.1, 0.2, 0.3}
}

// TestSearch_NeverRunsExpensiveCoverageScan verifies that Search does not invoke
// MeasureCoverage during queries, keeping interactive search fast and free of
// corpus-wide transcript scans and SHA hashing.
func TestSearch_NeverRunsExpensiveCoverageScan(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)

	writeRichSession(t, proj, "s1", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f0000000aaa1", content: "first long message about widget calibration and telemetry"},
		{role: "assistant", uuid: "f0000000aaa2", content: "second long message explaining the calibration telemetry results"},
	})
	writeRichSession(t, proj, "s2", "2026-06-02", []msgSpec{
		{role: "user", uuid: "f0000000bbb1", content: "third long message regarding billing system synchronization"},
	})

	dbp, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer con.Close()

	emb := fakeCoverageEmbedder{}
	if _, err := semantic.VecIndex(context.Background(), con, emb, 0); err != nil {
		t.Fatalf("VecIndex: %v", err)
	}

	env := Search("calibration", scope, SearchOpts{Limit: 5}, emb)

	if env.VectorCoverage.Ran {
		t.Errorf("VectorCoverage.Ran = true, want false (search must not run expensive coverage scan)")
	}
	if hasCode(env.Warnings, WarnVectorGap) {
		t.Errorf("unexpected WarnVectorGap in search: %v", codes(env.Warnings))
	}

	var buf bytes.Buffer
	renderSearch(&buf, env, "calibration", "across all projects")
	out := buf.String()
	if strings.Contains(out, "partial coverage") || strings.Contains(out, "--reindex-vectors") {
		t.Errorf("human output contains vector gap note when coverage scan was deleted from search:\n%s", out)
	}
}

// TestVectorCoverage_NoVectorReportsNotRun verifies that when nil embedder
// is passed, the search envelope explicitly reports the tier as not run (Ran=false).
func TestVectorCoverage_NoVectorReportsNotRun(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)

	writeRichSession(t, proj, "s1", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f0000000aaa1", content: "first long message about widget calibration and telemetry"},
		{role: "assistant", uuid: "f0000000aaa2", content: "second long message explaining calibration in detail"},
	})

	dbp, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("connect DB: %v", err)
	}
	defer con.Close()

	emb := fakeCoverageEmbedder{}
	if _, err := semantic.VecIndex(context.Background(), con, emb, 0); err != nil {
		t.Fatalf("VecIndex: %v", err)
	}

	env := Search("calibration", scope, SearchOpts{Limit: 5}, nil)

	if env.VectorCoverage.Ran {
		t.Errorf("VectorCoverage.Ran = true with nil embedder, want false")
	}
	if hasCode(env.Warnings, WarnVectorGap) {
		t.Errorf("nil embedder produced WarnVectorGap: %v", codes(env.Warnings))
	}
}

// TestVectorCoverage_ExplicitSortReportsNotRun verifies that an explicit --sort mode
// disables the semantic tier (Ran=false) and does not emit WarnVectorGap.
func TestVectorCoverage_ExplicitSortReportsNotRun(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)

	writeRichSession(t, proj, "s1", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f0000000aaa1", content: "first long message about widget calibration and telemetry"},
	})

	if _, _, _, err := index.EnsureIndexed(proj, false); err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}

	emb := fakeCoverageEmbedder{}
	env := Search("calibration", scope, SearchOpts{Sort: "newest", Limit: 5}, emb)

	if env.VectorCoverage.Ran {
		t.Errorf("VectorCoverage.Ran = true with --sort newest, want false")
	}
	if hasCode(env.Warnings, WarnVectorGap) {
		t.Errorf("explicit sort produced WarnVectorGap: %v", codes(env.Warnings))
	}
}
