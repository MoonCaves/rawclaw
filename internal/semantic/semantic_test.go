package semantic

import (
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

// openTestDB opens a fresh writable production-schema db (via storetest), so
// chunk_vec can live alongside the messages/sessions tables semantic depends on.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	con, _ := storetest.NewDB(t)
	return con
}

// addMessage inserts a session (if new) + a message, matching the table layout
// the indexer populates. Returns the message id.
func addMessage(t *testing.T, con *sql.DB, sid, role, content, iso string, isSub int, parent string) int {
	t.Helper()
	storetest.InsertSession(t, con, storetest.Session{ID: sid, IsSubagent: isSub != 0, ParentID: parent})
	return storetest.InsertMessage(t, con, storetest.Message{SessionID: sid, Role: role, Content: content, ISO: iso})
}

// fakeEmbedder returns a fixed vector per text (looked up by exact text), or nil
// (the no-op signal) when the text is unknown. Lets a test control routing.
type fakeEmbedder struct {
	vecs map[string][]float64
}

func (f fakeEmbedder) Embed(_ context.Context, text string) []float64 { return f.vecs[text] }

// nilEmbedder always returns nil — the keyword-only baseline.
type nilEmbedder struct{}

func (nilEmbedder) Embed(context.Context, string) []float64 { return nil }

// unpackVec decodes little-endian float32 bytes back into a float64 slice for testing.
func unpackVec(blob []byte) []float64 {
	if len(blob)%4 != 0 {
		return nil
	}
	out := make([]float64, len(blob)/4)
	for i := range out {
		out[i] = float64(math.Float32frombits(binary.LittleEndian.Uint32(blob[i*4:])))
	}
	return out
}

func TestPackUnpackRoundTrip(t *testing.T) {
	cases := [][]float64{
		{},
		{1.0},
		{0.1, 0.2, 0.3},
		{-1.5, 0.0, 42.25, -0.0078125},
	}
	for _, in := range cases {
		got := unpackVec(packVec(in))
		if len(got) != len(in) {
			t.Fatalf("len mismatch: got %d want %d", len(got), len(in))
		}
		for i := range in {
			// float32 round-trip: the value must equal its float32 truncation.
			want := float64(float32(in[i]))
			if got[i] != want {
				t.Errorf("vec[%d] = %v, want %v", i, got[i], want)
			}
		}
	}
	// Mid-byte garbage (len not divisible by 4) yields nil.
	if unpackVec([]byte{1, 2, 3}) != nil {
		t.Error("unpackVec of len-3 blob should be nil")
	}
}

func TestContentHashStableAnd16Hex(t *testing.T) {
	h := contentHash("hello from the corpus")
	if len(h) != 16 {
		t.Fatalf("hash len = %d, want 16", len(h))
	}
	if contentHash("hello from the corpus") != h {
		t.Error("contentHash not deterministic")
	}
	if contentHash("different text entirely") == h {
		t.Error("distinct inputs collided")
	}
}

func TestVecIndexEmbedsPrunesAndRefreshes(t *testing.T) {
	con := openTestDB(t)

	long := "this is a sufficiently long prose message about embeddings"
	short := "ok" // < MinChars, must be skipped

	id1 := addMessage(t, con, "s1", "user", long, "2026-06-18T10:00:00Z", 0, "")
	addMessage(t, con, "s1", "user", short, "2026-06-18T10:01:00Z", 0, "")

	emb := fakeEmbedder{vecs: map[string][]float64{long: {1, 0, 0}}}

	added, err := VecIndex(context.Background(), con, emb, 0)
	if err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1 (short message skipped)", added)
	}
	if !store.HasVectors(con) {
		t.Fatal("HasVectors = false after indexing one vector")
	}

	// Re-run: nothing new to embed (resumable).
	added2, err := VecIndex(context.Background(), con, emb, 0)
	if err != nil {
		t.Fatalf("VecIndex (2nd): %v", err)
	}
	if added2 != 0 {
		t.Fatalf("added2 = %d, want 0 (already vectored)", added2)
	}

	// Simulate a reindex that churns the message id but keeps the same text:
	// delete + re-insert the long message under a new autoincrement id.
	storetest.DeleteMessage(t, con, id1)
	newID := addMessage(t, con, "s1", "user", long, "2026-06-18T10:00:00Z", 0, "")
	if newID == id1 {
		t.Fatalf("expected a churned id; got the same %d", newID)
	}

	added3, err := VecIndex(context.Background(), con, emb, 0)
	if err != nil {
		t.Fatalf("VecIndex (3rd): %v", err)
	}
	if added3 != 0 {
		t.Fatalf("added3 = %d, want 0 (id churn refreshes, does not re-embed)", added3)
	}
	vrows, err := store.VecAll(con)
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	storedMID := -1
	for _, r := range vrows {
		if r.SessionID == "s1" {
			storedMID = r.MsgID
		}
	}
	if storedMID != newID {
		t.Fatalf("chunk_vec.msg_id = %d, want refreshed %d", storedMID, newID)
	}

	// Now remove the source text entirely → the vector must be pruned.
	storetest.DeleteMessage(t, con, newID)
	if _, err := VecIndex(context.Background(), con, emb, 0); err != nil {
		t.Fatalf("VecIndex (prune): %v", err)
	}
	if store.HasVectors(con) {
		t.Fatal("stale vector not pruned after source text removed")
	}
}

func TestVecIndexNilEmbedderAddsNothing(t *testing.T) {
	con := openTestDB(t)
	addMessage(t, con, "s1", "user", "a long enough message to be embedded", "2026-06-18T10:00:00Z", 0, "")

	added, err := VecIndex(context.Background(), con, nilEmbedder{}, 0)
	if err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	if added != 0 {
		t.Fatalf("added = %d, want 0 with nil embedder", added)
	}
	if store.HasVectors(con) {
		t.Fatal("nil embedder must not write vectors")
	}
}

func TestVecIndexMaxNewCap(t *testing.T) {
	con := openTestDB(t)
	vecs := map[string][]float64{}
	texts := []string{
		"first long message about alpha topics here",
		"second long message about beta topics here",
		"third long message about gamma topics here",
	}
	for i, txt := range texts {
		addMessage(t, con, "s1", "user", txt, "2026-06-18T10:0"+string(rune('0'+i))+":00Z", 0, "")
		vecs[txt] = []float64{float64(i), 1, 0}
	}
	added, err := VecIndex(context.Background(), con, fakeEmbedder{vecs: vecs}, 2)
	if err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	if added != 2 {
		t.Fatalf("added = %d, want 2 (maxNew cap)", added)
	}
}

func TestVecKNNRanksNearestAndSkipsSubagents(t *testing.T) {
	con := openTestDB(t)

	mNear := addMessage(t, con, "s1", "user", "the closest message to the query vector", "2026-06-18T10:00:00Z", 0, "")
	mFar := addMessage(t, con, "s1", "user", "an orthogonal message far from the query", "2026-06-18T10:01:00Z", 0, "")
	mSub := addMessage(t, con, "s2", "user", "a subagent message also near the query vec", "2026-06-18T10:02:00Z", 1, "s1")

	emb := fakeEmbedder{vecs: map[string][]float64{
		"the closest message to the query vector":    {1, 0, 0},
		"an orthogonal message far from the query":   {0, 1, 0},
		"a subagent message also near the query vec": {0.9, 0.1, 0},
	}}
	if _, err := VecIndex(context.Background(), con, emb, 0); err != nil {
		t.Fatalf("VecIndex: %v", err)
	}

	qvec := []float64{1, 0, 0}

	// Subagents excluded by default: mNear first, mFar present, mSub absent.
	hits := VecKNN(con, qvec, 5, false)
	if len(hits) != 2 {
		t.Fatalf("default hits = %d, want 2 (subagent excluded)", len(hits))
	}
	if hits[0].ID != mNear {
		t.Fatalf("nearest = %d, want %d", hits[0].ID, mNear)
	}
	for _, h := range hits {
		if h.ID == mSub {
			t.Fatal("subagent hit leaked into default result")
		}
		if h.ID == mFar {
			// far is allowed, just must rank after near
			if h.Dist >= hits[0].Dist {
				t.Fatalf("far dist %v >= near dist %v", h.Dist, hits[0].Dist)
			}
		}
	}
	if math.Abs(hits[0].Dist-1.0) > 1e-6 {
		t.Fatalf("near cosine = %v, want ~1.0", hits[0].Dist)
	}

	// With include_subagents the subagent surfaces.
	hitsSub := VecKNN(con, qvec, 5, true)
	if len(hitsSub) != 3 {
		t.Fatalf("include-subagent hits = %d, want 3", len(hitsSub))
	}
}

func TestVecKNNExistenceCheck(t *testing.T) {
	con := openTestDB(t)
	id := addMessage(t, con, "s1", "user", "a message that will be removed after indexing", "2026-06-18T10:00:00Z", 0, "")
	emb := fakeEmbedder{vecs: map[string][]float64{
		"a message that will be removed after indexing": {1, 0, 0},
	}}
	if _, err := VecIndex(context.Background(), con, emb, 0); err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	// Delete the message row but leave the vector orphaned (no reindex run).
	storetest.DeleteMessage(t, con, id)
	hits := VecKNN(con, []float64{1, 0, 0}, 5, false)
	if len(hits) != 0 {
		t.Fatalf("hits = %d, want 0 (orphan vector existence-checked out)", len(hits))
	}
}

func TestVecKNNMissingTable(t *testing.T) {
	con := openTestDB(t) // chunk_vec never created
	if got := VecKNN(con, []float64{1, 0, 0}, 5, false); len(got) != 0 {
		t.Fatalf("VecKNN on missing table = %d hits, want 0", len(got))
	}
}

func TestVecKNNDimMismatchSkipped(t *testing.T) {
	con := openTestDB(t)
	addMessage(t, con, "s1", "user", "a three dim message stored in the index", "2026-06-18T10:00:00Z", 0, "")
	emb := fakeEmbedder{vecs: map[string][]float64{
		"a three dim message stored in the index": {1, 0, 0},
	}}
	if _, err := VecIndex(context.Background(), con, emb, 0); err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	// Query with a 4-dim vector: the 3-dim stored vector is skipped.
	if got := VecKNN(con, []float64{1, 0, 0, 0}, 5, false); len(got) != 0 {
		t.Fatalf("dim-mismatched query returned %d hits, want 0", len(got))
	}
}

func TestVecKNNNegativeK(t *testing.T) {
	con := openTestDB(t)
	addMessage(t, con, "s1", "user", "a three dim message stored in the index", "2026-06-18T10:00:00Z", 0, "")
	emb := fakeEmbedder{vecs: map[string][]float64{
		"a three dim message stored in the index": {1, 0, 0},
	}}
	if _, err := VecIndex(context.Background(), con, emb, 0); err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	got := VecKNN(con, []float64{1, 0, 0}, -1, false)
	if got == nil || len(got) != 0 {
		t.Fatalf("VecKNN with k=-1 = %v (len %d), want empty non-nil result", got, len(got))
	}
}

func TestFuseRRF(t *testing.T) {
	con := openTestDB(t)

	// Two keyword anchors (ids 10, 20) and a vector hit overlapping id 20 plus a
	// vector-only id 30.
	mShared := addMessage(t, con, "s1", "user", "shared message present in both keyword and vector", "2026-06-18T10:00:00Z", 0, "")
	mVecOnly := addMessage(t, con, "s1", "user", "vector only message not matched by keyword path", "2026-06-18T10:01:00Z", 0, "")

	emb := fakeEmbedder{vecs: map[string][]float64{
		"shared message present in both keyword and vector": {1, 0, 0},
		"vector only message not matched by keyword path":   {0.99, 0.01, 0},
	}}
	if _, err := VecIndex(context.Background(), con, emb, 0); err != nil {
		t.Fatalf("VecIndex: %v", err)
	}

	kwRows := []retrieve.Anchor{
		{ID: 999, SessionID: "s1", Role: "user", ISO: "2026-06-18T09:00:00Z", Snip: "kw only top", Cov: 2},
		{ID: mShared, SessionID: "s1", Role: "user", ISO: "2026-06-18T10:00:00Z", Snip: "shared", Cov: 1},
	}

	merged := Fuse(con, kwRows, []float64{1, 0, 0}, 5, false)

	// The shared id appears once, carrying BOTH RRF contributions → highest fused.
	byID := map[int]retrieve.Anchor{}
	for _, a := range merged {
		byID[a.ID] = a
	}
	if _, ok := byID[mShared]; !ok {
		t.Fatal("shared id missing from merged")
	}
	if _, ok := byID[mVecOnly]; !ok {
		t.Fatal("vector-only id missing from merged")
	}
	if _, ok := byID[999]; !ok {
		t.Fatal("keyword-only id missing from merged")
	}

	// shared = kw rank1 (1/62) + vec rank0 (1/61); kw-only id 999 = kw rank0 (1/61);
	// vec-only = vec rank1 (1/62). Verify the exact RRF math.
	wantShared := 1.0/float64(RRFConstant+1+1) + 1.0/float64(RRFConstant+0+1)
	if math.Abs(byID[mShared].Fused-wantShared) > 1e-12 {
		t.Errorf("shared fused = %v, want %v", byID[mShared].Fused, wantShared)
	}
	wantKW := 1.0 / float64(RRFConstant+0+1)
	if math.Abs(byID[999].Fused-wantKW) > 1e-12 {
		t.Errorf("kw-only fused = %v, want %v", byID[999].Fused, wantKW)
	}

	// shared must sort first (largest fused).
	if merged[0].ID != mShared {
		t.Fatalf("merged[0] = %d, want shared %d", merged[0].ID, mShared)
	}

	// Vector-only row is synthesized: Role empty, Cov 0.
	if byID[mVecOnly].Role != "" {
		t.Errorf("vector-only Role = %q, want empty", byID[mVecOnly].Role)
	}
	if byID[mVecOnly].Cov != 0 {
		t.Errorf("vector-only Cov = %d, want 0", byID[mVecOnly].Cov)
	}
	// Keyword-only row keeps its fields.
	if byID[999].Snip != "kw only top" {
		t.Errorf("keyword-only row lost its Snip: %q", byID[999].Snip)
	}
}

// TestFuseKeywordOnly confirms Fuse with no vector rows returns the keyword
// anchors unchanged (topics are NOT part of the search ranking — they live in the
// separate on-demand `topics` command).
func TestFuseKeywordOnly(t *testing.T) {
	con := openTestDB(t)
	kwRows := []retrieve.Anchor{{ID: 1, Snip: "a"}, {ID: 2, Snip: "b"}}
	merged := Fuse(con, kwRows, []float64{1, 0, 0}, 5, false)
	if len(merged) != 2 {
		t.Fatalf("merged = %d, want 2", len(merged))
	}
	if merged[0].ID != 1 {
		t.Fatalf("merged[0] = %d, want 1", merged[0].ID)
	}
}

func TestFuseEmptyVectorPath(t *testing.T) {
	con := openTestDB(t) // no chunk_vec; vec path empty
	kwRows := []retrieve.Anchor{
		{ID: 1, Snip: "a"},
		{ID: 2, Snip: "b"},
	}
	merged := Fuse(con, kwRows, []float64{1, 0, 0}, 5, false)
	if len(merged) != 2 {
		t.Fatalf("merged = %d, want 2 (keyword-only when no vectors)", len(merged))
	}
	// id 1 (kw rank0) outranks id 2 (kw rank1).
	if merged[0].ID != 1 {
		t.Fatalf("merged[0] = %d, want 1", merged[0].ID)
	}
}

// TestFuseVectorOnlyCarriesOnlyCopySince is the F1 (D7) guard: a retained-but-
// only-copy session (only_copy_since > 0) matched ONLY by the vector path must keep
// its only_copy_since on the synthesized vector-only anchor — so the search
// envelope can still flag it (SearchRef.Missing = OnlyCopySince > 0).
func TestFuseVectorOnlyCarriesOnlyCopySince(t *testing.T) {
	con := openTestDB(t)

	// A session whose backing file is deleted upstream: the row is retained and flagged.
	mVec := addMessage(t, con, "smissing", "user", "vector only retained beacon message", "2026-06-18T10:00:00Z", 0, "")
	storetest.SetSessionField(t, con, "smissing", "only_copy_since", 1750000000)

	emb := fakeEmbedder{vecs: map[string][]float64{
		"vector only retained beacon message": {1, 0, 0},
	}}
	if _, err := VecIndex(context.Background(), con, emb, 0); err != nil {
		t.Fatalf("VecIndex: %v", err)
	}

	// No keyword anchors → this id is a PURE vector-only hit.
	merged := Fuse(con, nil, []float64{1, 0, 0}, 5, false)

	var got *retrieve.Anchor
	for i := range merged {
		if merged[i].ID == mVec {
			got = &merged[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("vector-only anchor for id %d missing from merged", mVec)
	}
	if got.Cov != 0 {
		t.Errorf("expected a vector-only (Cov 0) anchor, got Cov=%d", got.Cov)
	}
	// The fix: the synthesized vector-only anchor carries only_copy_since, so the
	// envelope's `Missing: OnlyCopySince > 0` fires for a vector-only retained hit.
	if got.OnlyCopySince <= 0 {
		t.Errorf("vector-only anchor dropped only_copy_since (got %v)", got.OnlyCopySince)
	}
	// NOTE: the synthesized vector-only anchor has no UUID (VecKNN does not hydrate
	// m.uuid). agentproto.Search drops uuid-less anchors BEFORE setting Missing, so
	// a PURE vector-only hit does not currently surface as a SearchRef at all — the
	// flag is carried defensively here at the fusion boundary. Making F1 observable
	// end-to-end additionally requires hydrating m.uuid onto the vector anchor.
	if got.UUID != "" {
		t.Errorf("unexpected UUID %q on a synthesized vector-only anchor", got.UUID)
	}
}

type mockBatchEmbedder struct {
	vecs        map[string][]float64
	failBatch   bool
	batchCalled bool
	itemCalled  bool
}

func (m *mockBatchEmbedder) Embed(_ context.Context, text string) []float64 {
	m.itemCalled = true
	return m.vecs[text]
}

func (m *mockBatchEmbedder) EmbedBatch(_ context.Context, texts []string) [][]float64 {
	m.batchCalled = true
	if m.failBatch {
		return nil
	}
	out := make([][]float64, len(texts))
	for i, t := range texts {
		out[i] = m.vecs[t]
	}
	return out
}

// TestVecIndex_BatchEmbedderFallback verifies that when EmbedBatch returns nil,
// VecIndex falls back to per-item Embed for that batch without losing vectors.
func TestVecIndex_BatchEmbedderFallback(t *testing.T) {
	con := openTestDB(t)

	msg1 := "first message for fallback testing long enough"
	msg2 := "second message for fallback testing long enough"

	addMessage(t, con, "s1", "user", msg1, "2026-06-18T10:00:00Z", 0, "")
	addMessage(t, con, "s1", "user", msg2, "2026-06-18T10:01:00Z", 0, "")

	emb := &mockBatchEmbedder{
		vecs: map[string][]float64{
			msg1: {1, 0, 0},
			msg2: {0, 1, 0},
		},
		failBatch: true, // Batch will return nil -> trigger per-item fallback
	}

	added, err := VecIndex(context.Background(), con, emb, 0)
	if err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	if added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}
	if !emb.batchCalled {
		t.Error("expected EmbedBatch to be called")
	}
	if !emb.itemCalled {
		t.Error("expected fallback to Embed per item when batch returned nil")
	}
	if !store.HasVectors(con) {
		t.Fatal("vectors should be present after fallback indexing")
	}
}

// TestVecIndex_BatchEmbedderSuccess verifies batch embedding end-to-end.
func TestVecIndex_BatchEmbedderSuccess(t *testing.T) {
	con := openTestDB(t)

	msg1 := "first message for batch success testing long enough"
	msg2 := "second message for batch success testing long enough"

	addMessage(t, con, "s1", "user", msg1, "2026-06-18T10:00:00Z", 0, "")
	addMessage(t, con, "s1", "user", msg2, "2026-06-18T10:01:00Z", 0, "")

	emb := &mockBatchEmbedder{
		vecs: map[string][]float64{
			msg1: {1, 0, 0},
			msg2: {0, 1, 0},
		},
		failBatch: false,
	}

	added, err := VecIndex(context.Background(), con, emb, 0)
	if err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	if added != 2 {
		t.Fatalf("added = %d, want 2", added)
	}
	if !emb.batchCalled {
		t.Error("expected EmbedBatch to be called")
	}
	if emb.itemCalled {
		t.Error("Embed should not be called when EmbedBatch succeeds")
	}
}

// cancellingEmbedder cancels the pass from inside the FIRST embed call, then
// serves every later call normally. That shape is what makes the assertion
// below meaningful: work is genuinely available to continue, so a pass that
// keeps going would embed all of it.
type cancellingEmbedder struct {
	cancel context.CancelFunc
	once   sync.Once
}

func (e *cancellingEmbedder) Embed(_ context.Context, text string) []float64 {
	e.once.Do(func() { e.cancel() })
	return []float64{1, 0, 0}
}

// TestVecIndexCancellationStopsWork pins the cancellation contract the ctx
// parameter exists for: a cancelled pass stops embedding instead of running the
// rest of the corpus against a remote endpoint whose caller has already walked
// away. It asserts the WEAK form — that the pass stops short — because exactly
// how many in-flight items land before the signal is observed is a race by
// design, and pinning an exact count would make this flap.
//
// The serial path is exercised deliberately: cancellingEmbedder implements only
// embed.Embedder, not BatchEmbedder, so this covers the fallback branch where a
// per-item loop must also honour the signal.
func TestVecIndexCancellationStopsWork(t *testing.T) {
	con := openTestDB(t)

	const n = 40
	for i := 0; i < n; i++ {
		addMessage(t, con, "s1", "user",
			fmt.Sprintf("a sufficiently long prose message number %d about embeddings", i),
			"2026-06-18T10:00:00Z", 0, "")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	added, err := VecIndex(ctx, con, &cancellingEmbedder{cancel: cancel}, 0)
	if err != nil {
		t.Fatalf("VecIndex under cancellation: %v", err)
	}
	if added >= n {
		t.Fatalf("cancellation did not stop the pass: embedded %d of %d", added, n)
	}

	// Whatever was committed before the signal STAYS committed: the pass is
	// resumable, so a cancelled run must never roll back landed work.
	var stored int
	if err := con.QueryRow("SELECT COUNT(*) FROM chunk_vec").Scan(&stored); err != nil {
		t.Fatalf("count chunk_vec: %v", err)
	}
	if stored != added {
		t.Fatalf("committed vectors %d != reported added %d", stored, added)
	}
}

func TestMeasureCoverage(t *testing.T) {
	t.Run("empty database", func(t *testing.T) {
		con := openTestDB(t)
		cov, err := MeasureCoverage(con)
		if err != nil {
			t.Fatalf("MeasureCoverage: %v", err)
		}
		if cov.Candidates != 0 || cov.Vectored != 0 || cov.Missing != 0 {
			t.Errorf("got %+v, want all 0", cov)
		}
	})

	t.Run("only short messages below MinChars", func(t *testing.T) {
		con := openTestDB(t)
		addMessage(t, con, "s1", "user", "ok", "2026-06-18T10:00:00Z", 0, "")
		addMessage(t, con, "s1", "user", "yes", "2026-06-18T10:01:00Z", 0, "")
		cov, err := MeasureCoverage(con)
		if err != nil {
			t.Fatalf("MeasureCoverage: %v", err)
		}
		if cov.Candidates != 0 || cov.Vectored != 0 || cov.Missing != 0 {
			t.Errorf("got %+v, want all 0 for short messages", cov)
		}
	})

	t.Run("candidate messages without chunk_vec", func(t *testing.T) {
		con := openTestDB(t)
		addMessage(t, con, "s1", "user", "first long candidate message for testing", "2026-06-18T10:00:00Z", 0, "")
		addMessage(t, con, "s1", "user", "second long candidate message for testing", "2026-06-18T10:01:00Z", 0, "")
		cov, err := MeasureCoverage(con)
		if err != nil {
			t.Fatalf("MeasureCoverage: %v", err)
		}
		if cov.Candidates != 2 || cov.Vectored != 0 || cov.Missing != 2 {
			t.Errorf("got %+v, want Candidates: 2, Vectored: 0, Missing: 2", cov)
		}
	})

	t.Run("candidate messages with partial vectors", func(t *testing.T) {
		con := openTestDB(t)
		msg1 := "first long candidate message for testing"
		msg2 := "second long candidate message for testing"
		msg3 := "third long candidate message for testing"
		addMessage(t, con, "s1", "user", msg1, "2026-06-18T10:00:00Z", 0, "")
		addMessage(t, con, "s1", "user", msg2, "2026-06-18T10:01:00Z", 0, "")
		addMessage(t, con, "s1", "user", msg3, "2026-06-18T10:02:00Z", 0, "")

		emb := fakeEmbedder{vecs: map[string][]float64{
			msg1: {1, 0, 0},
			msg2: {0, 1, 0},
			msg3: {0, 0, 1},
		}}
		// VecIndex maxNew=1 embeds 1 message
		if added, err := VecIndex(context.Background(), con, emb, 1); err != nil {
			t.Fatalf("VecIndex: %v", err)
		} else if added != 1 {
			t.Fatalf("VecIndex added %d, want 1", added)
		}

		cov, err := MeasureCoverage(con)
		if err != nil {
			t.Fatalf("MeasureCoverage: %v", err)
		}
		if cov.Candidates != 3 || cov.Vectored != 1 || cov.Missing != 2 {
			t.Errorf("got %+v, want Candidates: 3, Vectored: 1, Missing: 2", cov)
		}
	})

	t.Run("candidate messages with full vectors", func(t *testing.T) {
		con := openTestDB(t)
		msg1 := "first long candidate message for testing"
		msg2 := "second long candidate message for testing"
		addMessage(t, con, "s1", "user", msg1, "2026-06-18T10:00:00Z", 0, "")
		addMessage(t, con, "s1", "user", msg2, "2026-06-18T10:01:00Z", 0, "")

		emb := fakeEmbedder{vecs: map[string][]float64{
			msg1: {1, 0, 0},
			msg2: {0, 1, 0},
		}}
		if _, err := VecIndex(context.Background(), con, emb, 0); err != nil {
			t.Fatalf("VecIndex: %v", err)
		}

		cov, err := MeasureCoverage(con)
		if err != nil {
			t.Fatalf("MeasureCoverage: %v", err)
		}
		if cov.Candidates != 2 || cov.Vectored != 2 || cov.Missing != 0 {
			t.Errorf("got %+v, want Candidates: 2, Vectored: 2, Missing: 0", cov)
		}
	})

	t.Run("with project narrowing", func(t *testing.T) {
		con := openTestDB(t)
		storetest.InsertSession(t, con, storetest.Session{ID: "s1", Project: "proj-a"})
		storetest.InsertSession(t, con, storetest.Session{ID: "s2", Project: "proj-b"})
		storetest.InsertMessage(t, con, storetest.Message{SessionID: "s1", Role: "user", Content: "prose in project A about calibration", ISO: "2026-06-18T10:00:00Z"})
		storetest.InsertMessage(t, con, storetest.Message{SessionID: "s2", Role: "user", Content: "prose in project B about billing", ISO: "2026-06-18T10:00:00Z"})

		covA, err := MeasureCoverage(con, "proj-a")
		if err != nil {
			t.Fatalf("MeasureCoverage proj-a: %v", err)
		}
		if covA.Candidates != 1 || covA.Vectored != 0 || covA.Missing != 1 {
			t.Errorf("proj-a cov = %+v, want Candidates: 1, Vectored: 0, Missing: 1", covA)
		}

		covBoth, err := MeasureCoverage(con, "proj-a", "proj-b")
		if err != nil {
			t.Fatalf("MeasureCoverage proj-a+b: %v", err)
		}
		if covBoth.Candidates != 2 || covBoth.Vectored != 0 || covBoth.Missing != 2 {
			t.Errorf("proj-a+b cov = %+v, want Candidates: 2, Vectored: 0, Missing: 2", covBoth)
		}
	})
}

type errorOnUpsertEmbedder struct {
	con  *sql.DB
	once sync.Once
}

func (e *errorOnUpsertEmbedder) Embed(_ context.Context, text string) []float64 {
	return []float64{1.0, 0.0, 0.0}
}

func (e *errorOnUpsertEmbedder) EmbedBatch(_ context.Context, texts []string) [][]float64 {
	e.once.Do(func() {
		// Drop the chunk_vec table on the first batch so subsequent VecUpsert fails.
		_, _ = e.con.Exec("DROP TABLE chunk_vec")
	})
	out := make([][]float64, len(texts))
	for i := range texts {
		out[i] = []float64{1.0, 0.0, 0.0}
	}
	return out
}

func TestVecIndex_DBUpsertError_NoGoroutineLeak(t *testing.T) {
	con := openTestDB(t)

	// Insert 16 messages (>150k char boundary per pair) in a single transaction.
	storetest.InsertSession(t, con, storetest.Session{ID: "s1"})
	tx, err := con.Begin()
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	blob := fmt.Sprintf("message %s", strings.Repeat("x", 80000))
	for i := 0; i < 16; i++ {
		if _, err := tx.Exec("INSERT INTO messages(session_id, role, content, ts, ts_iso) VALUES (?, ?, ?, ?, ?)",
			"s1", "user", fmt.Sprintf("%d %s", i, blob), float64(1000+i), "2026-06-18T10:00:00Z"); err != nil {
			t.Fatalf("insert message %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit tx: %v", err)
	}

	emb := &errorOnUpsertEmbedder{con: con}
	_, err = VecIndex(context.Background(), con, emb, 0)
	if err == nil {
		t.Fatal("expected error from VecIndex when chunk_vec table is dropped, got nil")
	}
}

func TestVecIndex_BatchConstants(t *testing.T) {
	if DefaultBatchItems != 512 {
		t.Errorf("DefaultBatchItems = %d, want 512", DefaultBatchItems)
	}
	if DefaultBatchChars != 500000 {
		t.Errorf("DefaultBatchChars = %d, want 500000", DefaultBatchChars)
	}
	if DefaultMaxWorkers != 24 {
		t.Errorf("DefaultMaxWorkers = %d, want 24", DefaultMaxWorkers)
	}
}
