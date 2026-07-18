package store_test

import (
	"bytes"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

func TestHasVectorsDegrade(t *testing.T) {
	con, _ := storetest.NewDB(t)

	// Missing chunk_vec table reads as false, not an error.
	if store.HasVectors(con) {
		t.Error("HasVectors on missing table = true, want false")
	}

	// EnsureVecSchema is idempotent; an empty table still reads false.
	if err := store.EnsureVecSchema(con); err != nil {
		t.Fatalf("EnsureVecSchema: %v", err)
	}
	if err := store.EnsureVecSchema(con); err != nil {
		t.Fatalf("EnsureVecSchema (2nd): %v", err)
	}
	if store.HasVectors(con) {
		t.Error("HasVectors on empty table = true, want false")
	}

	if err := store.VecUpsert(con, "s", "hash1", 1, 2, []byte{1, 0, 0, 0, 2, 0, 0, 0}); err != nil {
		t.Fatalf("VecUpsert: %v", err)
	}
	if !store.HasVectors(con) {
		t.Error("HasVectors with a row = false, want true")
	}
}

func TestVecAllOnMissingTable(t *testing.T) {
	con, _ := storetest.NewDB(t)
	// No chunk_vec table yet: the scan errors (callers degrade to "no vectors").
	if _, err := store.VecAll(con); err == nil {
		t.Error("VecAll on missing table: err = nil, want error (degrade signal)")
	}
}

func TestVecRoundtrip(t *testing.T) {
	con, _ := storetest.NewDB(t)
	if err := store.EnsureVecSchema(con); err != nil {
		t.Fatalf("EnsureVecSchema: %v", err)
	}

	blobA := []byte{0x00, 0x00, 0x80, 0x3f} // float32(1.0) LE
	blobB := []byte{0x00, 0x00, 0x00, 0x40} // float32(2.0) LE
	if err := store.VecUpsert(con, "s1", "hashA", 10, 1, blobA); err != nil {
		t.Fatalf("VecUpsert A: %v", err)
	}
	if err := store.VecUpsert(con, "s1", "hashB", 11, 1, blobB); err != nil {
		t.Fatalf("VecUpsert B: %v", err)
	}

	rows, err := store.VecAll(con)
	if err != nil {
		t.Fatalf("VecAll: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("VecAll = %d rows, want 2", len(rows))
	}
	byHash := map[string]store.VecRow{}
	for _, r := range rows {
		byHash[r.ContentHash] = r
	}
	if r := byHash["hashA"]; r.SessionID != "s1" || r.MsgID != 10 || !bytes.Equal(r.Vec, blobA) {
		t.Errorf("hashA row = %+v, want s1/10/%v", r, blobA)
	}

	// Upsert replaces on the (session_id, content_hash) key.
	if err := store.VecUpsert(con, "s1", "hashA", 12, 1, blobB); err != nil {
		t.Fatalf("VecUpsert replace: %v", err)
	}
	rows, _ = store.VecAll(con)
	byHash = map[string]store.VecRow{}
	for _, r := range rows {
		byHash[r.ContentHash] = r
	}
	if r := byHash["hashA"]; r.MsgID != 12 || !bytes.Equal(r.Vec, blobB) {
		t.Errorf("replaced hashA = %+v, want msg 12 + new blob", r)
	}

	// RefreshMsgID re-points without touching the blob.
	if err := store.VecRefreshMsgID(con, "s1", "hashB", 99); err != nil {
		t.Fatalf("VecRefreshMsgID: %v", err)
	}
	rows, _ = store.VecAll(con)
	for _, r := range rows {
		if r.ContentHash == "hashB" && (r.MsgID != 99 || !bytes.Equal(r.Vec, blobB)) {
			t.Errorf("refreshed hashB = %+v, want msg 99 with original blob", r)
		}
	}

	// Prune removes exactly the keyed row.
	if err := store.VecPrune(con, "s1", "hashA"); err != nil {
		t.Fatalf("VecPrune: %v", err)
	}
	rows, _ = store.VecAll(con)
	if len(rows) != 1 || rows[0].ContentHash != "hashB" {
		t.Errorf("after prune = %+v, want only hashB", rows)
	}

	// Pruning a missing key is a no-op, not an error.
	if err := store.VecPrune(con, "s1", "nope"); err != nil {
		t.Errorf("VecPrune(missing) = %v, want nil", err)
	}
}
