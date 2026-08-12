package semantic

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"
	_ "modernc.org/sqlite"
)

// countTopupSpawns swaps the spawn seam for a counter so gate tests observe spawn
// decisions without forking processes.
func countTopupSpawns(t *testing.T) *int {
	t.Helper()
	calls := 0
	old := spawnVectorTopup
	SetSpawnVectorTopup(func(dbp string) { calls++ })
	t.Cleanup(func() {
		spawnMu.Lock()
		spawnVectorTopup = old
		spawnMu.Unlock()
	})
	return &calls
}

// mockEmbedder implements embed.Embedder for testing.
type mockEmbedder struct {
	vec []float64
}

func (m *mockEmbedder) Embed(text string) []float64 {
	return m.vec
}

// TestMaybeVectorTopup_NoEmbedderMeansZeroSpawns: RAWCLAW_EMBED_ENDPOINT unset →
// zero top-up spawns.
func TestMaybeVectorTopup_NoEmbedderMeansZeroSpawns(t *testing.T) {
	t.Setenv("RAWCLAW_EMBED_ENDPOINT", "")
	SetNoVector(false)
	calls := countTopupSpawns(t)

	dbp := filepath.Join(t.TempDir(), "test.db")
	MaybeVectorTopup(dbp)

	if *calls != 0 {
		t.Errorf("spawns with no embedder = %d, want 0", *calls)
	}
}

// TestMaybeVectorTopup_NoVectorFlagMeansZeroSpawns: --no-vector set →
// zero top-up spawns even with an embedder configured.
func TestMaybeVectorTopup_NoVectorFlagMeansZeroSpawns(t *testing.T) {
	t.Setenv("RAWCLAW_EMBED_ENDPOINT", "http://localhost:11434/api/embeddings")
	SetNoVector(true)
	t.Cleanup(func() { SetNoVector(false) })
	calls := countTopupSpawns(t)

	dbp := filepath.Join(t.TempDir(), "test.db")
	MaybeVectorTopup(dbp)

	if *calls != 0 {
		t.Errorf("spawns under --no-vector = %d, want 0", *calls)
	}
}

// TestVecIndex_RespectsMaxNewBound: VecIndex bounds the count of new vectors
// created per pass when maxNew > 0.
func TestVecIndex_RespectsMaxNewBound(t *testing.T) {
	dbp := filepath.Join(t.TempDir(), "test_bound.db")
	con, err := sql.Open("sqlite", dbp)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer con.Close()

	if err := index.EnsureSchema(con, "claude"); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	if err := store.EnsureVecSchema(con); err != nil {
		t.Fatalf("ensure vec schema: %v", err)
	}

	// Insert 10 distinct messages >= MinChars.
	for i := 1; i <= 10; i++ {
		sid := "sess-1"
		role := "user"
		content := "This is a sufficiently long message for vector indexing tests #" + string(rune('A'+i))
		ts := float64(1000 + i)
		iso := "2026-08-12T10:00:00Z"
		uuid := "uuid-" + string(rune('A'+i))
		if _, err := con.Exec("INSERT INTO messages(id,session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?,?)",
			i, sid, role, content, ts, iso, uuid); err != nil {
			t.Fatalf("insert message %d: %v", i, err)
		}
	}

	emb := &mockEmbedder{vec: []float64{0.1, 0.2, 0.3}}

	// Pass maxNew = 3: should embed exactly 3 messages.
	added1, err := VecIndex(context.Background(), con, emb, 3)
	if err != nil {
		t.Fatalf("VecIndex pass 1: %v", err)
	}
	if added1 != 3 {
		t.Errorf("pass 1 added = %d, want 3", added1)
	}

	var count1 int
	if err := con.QueryRow("SELECT COUNT(*) FROM chunk_vec").Scan(&count1); err != nil {
		t.Fatalf("count chunk_vec pass 1: %v", err)
	}
	if count1 != 3 {
		t.Errorf("chunk_vec count after pass 1 = %d, want 3", count1)
	}

	// Pass maxNew = 4: should embed another 4 messages (total 7).
	added2, err := VecIndex(context.Background(), con, emb, 4)
	if err != nil {
		t.Fatalf("VecIndex pass 2: %v", err)
	}
	if added2 != 4 {
		t.Errorf("pass 2 added = %d, want 4", added2)
	}

	var count2 int
	if err := con.QueryRow("SELECT COUNT(*) FROM chunk_vec").Scan(&count2); err != nil {
		t.Fatalf("count chunk_vec pass 2: %v", err)
	}
	if count2 != 7 {
		t.Errorf("chunk_vec count after pass 2 = %d, want 7", count2)
	}
}

// TestTryAcquireTopupLock_ConcurrentLockSkipped: two concurrent lock attempts on
// the same store -> second one returns ok=false (skipped).
func TestTryAcquireTopupLock_ConcurrentLockSkipped(t *testing.T) {
	dbp := filepath.Join(t.TempDir(), "lock_test.db")

	rel1, ok1 := TryAcquireTopupLock(dbp)
	if !ok1 || rel1 == nil {
		t.Fatalf("first acquire failed, want ok=true")
	}

	rel2, ok2 := TryAcquireTopupLock(dbp)
	if ok2 || rel2 != nil {
		t.Fatalf("second acquire succeeded during active lock, want ok=false")
	}

	rel1()

	rel3, ok3 := TryAcquireTopupLock(dbp)
	if !ok3 || rel3 == nil {
		t.Fatalf("acquire after release failed, want ok=true")
	}
	rel3()
}

// TestAcquireTopupToken_ThrottlesSpawnsPerWindow: two token acquisitions in the
// same window -> second returns false.
func TestAcquireTopupToken_ThrottlesSpawnsPerWindow(t *testing.T) {
	dbp := filepath.Join(t.TempDir(), "token_test_unique.db")
	tokenPath := topupTokenPath(dbp)
	_ = os.Remove(tokenPath)
	t.Cleanup(func() { _ = os.Remove(tokenPath) })

	now := time.Now()

	if !AcquireTopupToken(dbp, now) {
		t.Fatalf("first AcquireTopupToken = false, want true")
	}
	if AcquireTopupToken(dbp, now) {
		t.Errorf("second AcquireTopupToken in same window = true, want false")
	}
	if !AcquireTopupToken(dbp, now.Add(2*topupWindow)) {
		t.Errorf("AcquireTopupToken after window = false, want true")
	}
}
