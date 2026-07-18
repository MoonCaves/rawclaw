package semantic

import (
	"database/sql"
	"math"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/retrieve"

	_ "modernc.org/sqlite"
)

// openTestDB opens a fresh writable db with the keyword schema ensured, so
// chunk_vec can live alongside the messages/sessions tables semantic depends on.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	con, err := sql.Open("sqlite", "file:"+t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	con.SetMaxOpenConns(1)
	t.Cleanup(func() { con.Close() })
	if err := index.EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	return con
}

// addMessage inserts a session (if new) + a message, matching the table layout
// the indexer populates. Returns the message id.
func addMessage(t *testing.T, con *sql.DB, sid, role, content, iso string, isSub int, parent string) int {
	t.Helper()
	if _, err := con.Exec(
		"INSERT OR IGNORE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id) VALUES(?,?,?,?,?,?)",
		sid, 0.0, 0.0, 0, isSub, parent); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	res, err := con.Exec(
		"INSERT INTO messages(session_id,role,content,ts,ts_iso) VALUES(?,?,?,?,?)",
		sid, role, content, 0.0, iso)
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("last id: %v", err)
	}
	return int(id)
}

// fakeEmbedder returns a fixed vector per text (looked up by exact text), or nil
// (the no-op signal) when the text is unknown. Lets a test control routing.
type fakeEmbedder struct {
	vecs map[string][]float64
}

func (f fakeEmbedder) Embed(text string) []float64 { return f.vecs[text] }

// nilEmbedder always returns nil — the keyword-only baseline.
type nilEmbedder struct{}

func (nilEmbedder) Embed(string) []float64 { return nil }

func TestEnsureVecSchemaAndHasVectors(t *testing.T) {
	con := openTestDB(t)

	// Before the schema exists, HasVectors must be false (missing table swallow).
	if HasVectors(con) {
		t.Fatal("HasVectors = true before EnsureVecSchema")
	}
	if err := EnsureVecSchema(con); err != nil {
		t.Fatalf("EnsureVecSchema: %v", err)
	}
	// Idempotent.
	if err := EnsureVecSchema(con); err != nil {
		t.Fatalf("EnsureVecSchema (2nd): %v", err)
	}
	if HasVectors(con) {
		t.Fatal("HasVectors = true on empty chunk_vec")
	}

	var v string
	if err := con.QueryRow("SELECT value FROM meta WHERE key='vec_schema_version'").Scan(&v); err != nil {
		t.Fatalf("read vec_schema_version: %v", err)
	}
	if v != "1" {
		t.Fatalf("vec_schema_version = %q, want \"1\"", v)
	}
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

	added, err := VecIndex(con, emb, 0)
	if err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	if added != 1 {
		t.Fatalf("added = %d, want 1 (short message skipped)", added)
	}
	if !HasVectors(con) {
		t.Fatal("HasVectors = false after indexing one vector")
	}

	// Re-run: nothing new to embed (resumable).
	added2, err := VecIndex(con, emb, 0)
	if err != nil {
		t.Fatalf("VecIndex (2nd): %v", err)
	}
	if added2 != 0 {
		t.Fatalf("added2 = %d, want 0 (already vectored)", added2)
	}

	// Simulate a reindex that churns the message id but keeps the same text:
	// delete + re-insert the long message under a new autoincrement id.
	if _, err := con.Exec("DELETE FROM messages WHERE id=?", id1); err != nil {
		t.Fatalf("delete msg: %v", err)
	}
	newID := addMessage(t, con, "s1", "user", long, "2026-06-18T10:00:00Z", 0, "")
	if newID == id1 {
		t.Fatalf("expected a churned id; got the same %d", newID)
	}

	added3, err := VecIndex(con, emb, 0)
	if err != nil {
		t.Fatalf("VecIndex (3rd): %v", err)
	}
	if added3 != 0 {
		t.Fatalf("added3 = %d, want 0 (id churn refreshes, does not re-embed)", added3)
	}
	var storedMID int
	if err := con.QueryRow("SELECT msg_id FROM chunk_vec WHERE session_id='s1'").Scan(&storedMID); err != nil {
		t.Fatalf("read msg_id: %v", err)
	}
	if storedMID != newID {
		t.Fatalf("chunk_vec.msg_id = %d, want refreshed %d", storedMID, newID)
	}

	// Now remove the source text entirely → the vector must be pruned.
	if _, err := con.Exec("DELETE FROM messages WHERE id=?", newID); err != nil {
		t.Fatalf("delete churned msg: %v", err)
	}
	if _, err := VecIndex(con, emb, 0); err != nil {
		t.Fatalf("VecIndex (prune): %v", err)
	}
	if HasVectors(con) {
		t.Fatal("stale vector not pruned after source text removed")
	}
}

func TestVecIndexNilEmbedderAddsNothing(t *testing.T) {
	con := openTestDB(t)
	addMessage(t, con, "s1", "user", "a long enough message to be embedded", "2026-06-18T10:00:00Z", 0, "")

	added, err := VecIndex(con, nilEmbedder{}, 0)
	if err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	if added != 0 {
		t.Fatalf("added = %d, want 0 with nil embedder", added)
	}
	if HasVectors(con) {
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
	added, err := VecIndex(con, fakeEmbedder{vecs: vecs}, 2)
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
	if _, err := VecIndex(con, emb, 0); err != nil {
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
	if _, err := VecIndex(con, emb, 0); err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	// Delete the message row but leave the vector orphaned (no reindex run).
	if _, err := con.Exec("DELETE FROM messages WHERE id=?", id); err != nil {
		t.Fatalf("delete: %v", err)
	}
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
	if _, err := VecIndex(con, emb, 0); err != nil {
		t.Fatalf("VecIndex: %v", err)
	}
	// Query with a 4-dim vector: the 3-dim stored vector is skipped.
	if got := VecKNN(con, []float64{1, 0, 0, 0}, 5, false); len(got) != 0 {
		t.Fatalf("dim-mismatched query returned %d hits, want 0", len(got))
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
	if _, err := VecIndex(con, emb, 0); err != nil {
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

// TestFuseVectorOnlyCarriesMissingSince is the F1 (D7) guard: a retained-but-
// missing session (missing_since > 0) matched ONLY by the vector path must keep
// its missing_since on the synthesized vector-only anchor — so the search
// envelope can still flag it (SearchRef.Missing = MissingSince > 0) instead of
// reading a stale session as current. Before the fix, VecKNN never hydrated
// missing_since and Fuse left it zero on the synthesized anchor.
func TestFuseVectorOnlyCarriesMissingSince(t *testing.T) {
	con := openTestDB(t)

	// A session whose backing file is gone: the row is retained and flagged.
	mVec := addMessage(t, con, "smissing", "user", "vector only retained beacon message", "2026-06-18T10:00:00Z", 0, "")
	if _, err := con.Exec("UPDATE sessions SET missing_since=1750000000 WHERE id='smissing'"); err != nil {
		t.Fatalf("flag missing: %v", err)
	}

	emb := fakeEmbedder{vecs: map[string][]float64{
		"vector only retained beacon message": {1, 0, 0},
	}}
	if _, err := VecIndex(con, emb, 0); err != nil {
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
	// The fix: the synthesized vector-only anchor carries missing_since, so the
	// envelope's `Missing: MissingSince > 0` fires for a vector-only retained hit.
	if got.MissingSince <= 0 {
		t.Errorf("vector-only anchor dropped missing_since (got %v) — a retained session would read as current (F1)", got.MissingSince)
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
