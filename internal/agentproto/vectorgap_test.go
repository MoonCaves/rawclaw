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

func (f fakeCoverageEmbedder) Embed(text string) []float64 {
	if v, ok := f.vecs[text]; ok {
		return v
	}
	return []float64{0.1, 0.2, 0.3}
}

// TestVectorCoverage_FullCoverageReportsNoGap verifies that when 100% of candidate
// messages have embeddings in chunk_vec, the search envelope reports Ran=true,
// full coverage counts, MissingMsgs=0, and emits no WarnVectorGap warning or human note.
func TestVectorCoverage_FullCoverageReportsNoGap(t *testing.T) {
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

	if !env.VectorCoverage.Ran {
		t.Errorf("VectorCoverage.Ran = false, want true")
	}
	if env.VectorCoverage.CandidateMsgs != 3 {
		t.Errorf("CandidateMsgs = %d, want 3", env.VectorCoverage.CandidateMsgs)
	}
	if env.VectorCoverage.VectoredMsgs != 3 {
		t.Errorf("VectoredMsgs = %d, want 3", env.VectorCoverage.VectoredMsgs)
	}
	if env.VectorCoverage.MissingMsgs != 0 {
		t.Errorf("MissingMsgs = %d, want 0", env.VectorCoverage.MissingMsgs)
	}
	if hasCode(env.Warnings, WarnVectorGap) {
		t.Errorf("full coverage produced WarnVectorGap unexpectedly: %v", codes(env.Warnings))
	}

	var buf bytes.Buffer
	renderSearch(&buf, env, "calibration", "across all projects")
	out := buf.String()
	if strings.Contains(out, "partial coverage") || strings.Contains(out, "--reindex-vectors") {
		t.Errorf("human output contains vector gap note when coverage is complete:\n%s", out)
	}
}

// TestVectorCoverage_PartialCoverageReportsRealNumbers verifies that when only a subset
// of candidate messages have vectors, the search envelope reports the exact candidate,
// vectored, and missing counts, emits WarnVectorGap with the exact facts, and renders
// an honest warning note in human text output.
func TestVectorCoverage_PartialCoverageReportsRealNumbers(t *testing.T) {
	proj := t.TempDir()
	scope := scopeFor(t, proj)

	writeRichSession(t, proj, "s1", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f0000000aaa1", content: "first long message about widget calibration in warehouse alpha"},
		{role: "assistant", uuid: "f0000000aaa2", content: "second long message explaining calibration in warehouse beta"},
		{role: "user", uuid: "f0000000aaa3", content: "third long message about widget sensors and accuracy"},
	})
	writeRichSession(t, proj, "s2", "2026-06-02", []msgSpec{
		{role: "user", uuid: "f0000000bbb1", content: "fourth long message regarding billing system reconciliation"},
		{role: "assistant", uuid: "f0000000bbb2", content: "fifth long message about financial ledger integrity"},
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
	// Index only 2 of the 5 candidate messages
	if added, err := semantic.VecIndex(context.Background(), con, emb, 2); err != nil {
		t.Fatalf("VecIndex: %v", err)
	} else if added != 2 {
		t.Fatalf("VecIndex added %d, want 2", added)
	}

	env := Search("calibration", scope, SearchOpts{Limit: 5}, emb)

	if !env.VectorCoverage.Ran {
		t.Errorf("VectorCoverage.Ran = false, want true")
	}
	if env.VectorCoverage.CandidateMsgs != 5 {
		t.Errorf("CandidateMsgs = %d, want 5", env.VectorCoverage.CandidateMsgs)
	}
	if env.VectorCoverage.VectoredMsgs != 2 {
		t.Errorf("VectoredMsgs = %d, want 2", env.VectorCoverage.VectoredMsgs)
	}
	if env.VectorCoverage.MissingMsgs != 3 {
		t.Errorf("MissingMsgs = %d, want 3", env.VectorCoverage.MissingMsgs)
	}
	if !hasCode(env.Warnings, WarnVectorGap) {
		t.Fatalf("partial coverage did not produce WarnVectorGap: got %v", codes(env.Warnings))
	}

	w := findWarning(env.Warnings, WarnVectorGap)
	if w == nil {
		t.Fatal("WarnVectorGap not found in Warnings")
	}
	if c, ok := w.Facts["candidate_msgs"].(int); !ok || c != 5 {
		t.Errorf("Facts[candidate_msgs] = %v, want 5", w.Facts["candidate_msgs"])
	}
	if v, ok := w.Facts["vectored_msgs"].(int); !ok || v != 2 {
		t.Errorf("Facts[vectored_msgs] = %v, want 2", w.Facts["vectored_msgs"])
	}
	if m, ok := w.Facts["missing_msgs"].(int); !ok || m != 3 {
		t.Errorf("Facts[missing_msgs] = %v, want 3", w.Facts["missing_msgs"])
	}

	var buf bytes.Buffer
	renderSearch(&buf, env, "calibration", "across all projects")
	out := buf.String()
	wantNote := "note: semantic tier has partial coverage (2 of 5 candidate messages vectored, 3 missing) — run `rawclaw --reindex-vectors`"
	if !strings.Contains(out, wantNote) {
		t.Errorf("human output missing expected note %q:\n%s", wantNote, out)
	}
}

// TestVectorCoverage_NoVectorReportsNotRun verifies that when --no-vector (or nil embedder)
// is passed, the search envelope explicitly reports the tier as not run (Ran=false)
// rather than as a zero-coverage gap.
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

	// Search with nil embedder (simulating --no-vector)
	env := Search("calibration", scope, SearchOpts{Limit: 5}, nil)

	if env.VectorCoverage.Ran {
		t.Errorf("VectorCoverage.Ran = true with nil embedder, want false")
	}
	if env.VectorCoverage.CandidateMsgs != 0 || env.VectorCoverage.VectoredMsgs != 0 || env.VectorCoverage.MissingMsgs != 0 {
		t.Errorf("VectorCoverage has non-zero counts when not run: %+v", env.VectorCoverage)
	}
	if hasCode(env.Warnings, WarnVectorGap) {
		t.Errorf("nil embedder produced WarnVectorGap: %v", codes(env.Warnings))
	}

	var buf bytes.Buffer
	renderSearch(&buf, env, "calibration", "across all projects")
	out := buf.String()
	if strings.Contains(out, "partial coverage") || strings.Contains(out, "--reindex-vectors") {
		t.Errorf("human output contains vector note when tier did not run:\n%s", out)
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

// TestVectorCoverage_ConsolidatedStoreCoverageReportsGap verifies that the one-store
// search path (scope == nil) measures candidate messages and reports any missing vectors.
func TestVectorCoverage_ConsolidatedStoreCoverageReportsGap(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	proj := t.TempDir()
	writeRichSession(t, proj, "s1", "2026-06-01", []msgSpec{
		{role: "user", uuid: "f0000000aaa1", content: "message about widget calibration and telemetry stream"},
		{role: "assistant", uuid: "f0000000aaa2", content: "response about widget calibration settings and diagnostics"},
	})

	db, _, _, err := index.EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("EnsureIndexed: %v", err)
	}
	if _, err := index.ConsolidateFrom([]string{db}, true); err != nil {
		t.Fatalf("ConsolidateFrom: %v", err)
	}

	emb := fakeCoverageEmbedder{}
	// Search one-store (scope == nil)
	env := Search("calibration", nil, SearchOpts{Limit: 5}, emb)

	if !env.VectorCoverage.Ran {
		t.Errorf("VectorCoverage.Ran = false for one-store semantic search, want true")
	}
	if env.VectorCoverage.CandidateMsgs != 2 {
		t.Errorf("CandidateMsgs = %d, want 2", env.VectorCoverage.CandidateMsgs)
	}
	if env.VectorCoverage.VectoredMsgs != 0 {
		t.Errorf("VectoredMsgs = %d, want 0", env.VectorCoverage.VectoredMsgs)
	}
	if env.VectorCoverage.MissingMsgs != 2 {
		t.Errorf("MissingMsgs = %d, want 2", env.VectorCoverage.MissingMsgs)
	}
	if !hasCode(env.Warnings, WarnVectorGap) {
		t.Errorf("unembedded consolidated store did not produce WarnVectorGap: %v", codes(env.Warnings))
	}
}
