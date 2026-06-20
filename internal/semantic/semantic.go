// Package semantic is the optional vector channel: a float32-BLOB VectorStore
// living in the SAME cache db as the keyword index (under its own schema gate),
// brute-force cosine KNN in plain Go, and reciprocal-rank fusion.
//
// It is NEVER in the keyword _rebuild() drop list and never bumps the keyword
// SchemaVersion — a keyword reindex can't nuke vectors and vice versa. Vectors
// are keyed by (session_id, content_hash) so msg-id churn on reindex is
// harmless.
//
// Vectors are packed as little-endian float32 BLOBs.
package semantic

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/embed"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/retrieve"
)

// VecSchemaVersion gates the chunk_vec table separately from the keyword schema.
const VecSchemaVersion = 1

// MinChars skips trivial messages (greetings, acks).
const MinChars = 12

// RRFConstant is the reciprocal-rank-fusion k.
const RRFConstant = 60

// VecHit is one vector-KNN anchor: id, session_id, iso, parent, dist.
type VecHit struct {
	ID        int
	SessionID string
	ISO       string
	Parent    string
	Dist      float64 // cosine similarity (higher = nearer)
}

// EnsureVecSchema creates the chunk_vec table (its own gate, separate from the
// keyword schema) and stamps the vec_schema_version meta key.
func EnsureVecSchema(con *sql.DB) error {
	if _, err := con.Exec(`CREATE TABLE IF NOT EXISTS chunk_vec (
        session_id TEXT, content_hash TEXT, msg_id INTEGER, dim INTEGER, vec BLOB,
        PRIMARY KEY (session_id, content_hash))`); err != nil {
		return fmt.Errorf("create chunk_vec: %w", err)
	}
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO meta(key,value) VALUES('vec_schema_version',?)",
		strconv.Itoa(VecSchemaVersion),
	); err != nil {
		return fmt.Errorf("stamp vec_schema_version: %w", err)
	}
	return nil
}

// HasVectors reports whether chunk_vec holds any rows. A missing table or read
// error reads as false.
func HasVectors(con *sql.DB) bool {
	var one int
	err := con.QueryRow("SELECT 1 FROM chunk_vec LIMIT 1").Scan(&one)
	return err == nil
}

// packVec encodes a float vector as little-endian float32 bytes. Stored values
// are float32-precision.
func packVec(vec []float64) []byte {
	buf := make([]byte, len(vec)*4)
	for i, f := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(float32(f)))
	}
	return buf
}

// unpackVec decodes little-endian float32 bytes back into a float64 slice.
// A length not divisible by 4 yields nil.
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

// contentHash is the first 16 hex chars of the SHA-1 of the text.
func contentHash(text string) string {
	sum := sha1.Sum([]byte(text))
	return hex.EncodeToString(sum[:])[:16]
}

// curEntry is one current (sid, hash) -> (msg_id, text) row.
type curEntry struct {
	mid  int
	text string
}

// vecKey is the composite primary key (session_id, content_hash).
type vecKey struct {
	sid  string
	hash string
}

// VecIndex embeds any message (tool-stripped prose ≥ MinChars) not yet vectored,
// prunes stale vectors, and refreshes churned msg_ids. Resumable; returns the
// count added. `maxNew` of 0 = no cap.
func VecIndex(con *sql.DB, embedder embed.Embedder, maxNew int) (added int, err error) {
	if err := EnsureVecSchema(con); err != nil {
		return 0, err
	}
	ctx := context.Background()

	// Build the current (sid, hash) -> (msg_id, text) map from live messages.
	current := map[vecKey]curEntry{}
	rows, err := con.QueryContext(ctx, "SELECT id, session_id, content FROM messages")
	if err != nil {
		return 0, fmt.Errorf("scan messages: %w", err)
	}
	for rows.Next() {
		var (
			mid     int
			sid     string
			content sql.NullString
		)
		if err := rows.Scan(&mid, &sid, &content); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan message row: %w", err)
		}
		text := strings.TrimSpace(parse.StripTools(content.String))
		if len([]rune(text)) < MinChars {
			continue
		}
		current[vecKey{sid, contentHash(text)}] = curEntry{mid, text}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate messages: %w", err)
	}
	rows.Close()

	// Load what we already have: (sid, hash) -> stored msg_id.
	have := map[vecKey]int{}
	hrows, err := con.QueryContext(ctx, "SELECT session_id, content_hash, msg_id FROM chunk_vec")
	if err != nil {
		return 0, fmt.Errorf("scan chunk_vec: %w", err)
	}
	for hrows.Next() {
		var (
			sid string
			h   string
			msg int
		)
		if err := hrows.Scan(&sid, &h, &msg); err != nil {
			hrows.Close()
			return 0, fmt.Errorf("scan chunk_vec row: %w", err)
		}
		have[vecKey{sid, h}] = msg
	}
	if err := hrows.Err(); err != nil {
		hrows.Close()
		return 0, fmt.Errorf("iterate chunk_vec: %w", err)
	}
	hrows.Close()

	// Prune vectors whose source text no longer exists.
	for k := range have {
		if _, ok := current[k]; ok {
			continue
		}
		if _, err := con.ExecContext(ctx,
			"DELETE FROM chunk_vec WHERE session_id=? AND content_hash=?", k.sid, k.hash); err != nil {
			return 0, fmt.Errorf("prune stale vector: %w", err)
		}
	}

	// Embed the missing; refresh churned msg_ids (no re-embed).
	for k, e := range current {
		if storedMID, ok := have[k]; ok {
			if storedMID != e.mid { // id churned on a reindex — refresh, don't re-embed
				if _, err := con.ExecContext(ctx,
					"UPDATE chunk_vec SET msg_id=? WHERE session_id=? AND content_hash=?",
					e.mid, k.sid, k.hash); err != nil {
					return 0, fmt.Errorf("refresh msg_id: %w", err)
				}
			}
			continue
		}
		vec := embedder.Embed(e.text) // document side
		if len(vec) == 0 {
			continue
		}
		if _, err := con.ExecContext(ctx,
			"INSERT OR REPLACE INTO chunk_vec(session_id,content_hash,msg_id,dim,vec) VALUES(?,?,?,?,?)",
			k.sid, k.hash, e.mid, len(vec), packVec(vec)); err != nil {
			return 0, fmt.Errorf("insert vector: %w", err)
		}
		added++
		if maxNew != 0 && added >= maxNew {
			break
		}
	}
	return added, nil
}

// knnRow is one stored vector pulled for the brute-force scan.
type knnRow struct {
	msgID int
	sid   string
	blob  []byte
}

// ranked is one cosine-scored candidate (sim, msg_id, sid). Sorting is a
// descending order over the (sim, msg_id, sid) tuple.
type ranked struct {
	sim   float64
	msgID int
	sid   string
}

// knn runs the brute-force cosine scan and returns the top-`k` candidates,
// nearest first. Vectors whose dim != len(qvec) are skipped.
func knn(qvec []float64, rows []knnRow, k int) []ranked {
	qn := 0.0
	for _, x := range qvec {
		qn += x * x
	}
	qn = math.Sqrt(qn)
	if qn == 0 {
		qn = 1.0
	}

	out := make([]ranked, 0, len(rows))
	for _, r := range rows {
		v := unpackVec(r.blob)
		if len(v) != len(qvec) {
			continue
		}
		dot, nn := 0.0, 0.0
		for i := range qvec {
			dot += qvec[i] * v[i]
			nn += v[i] * v[i]
		}
		vn := math.Sqrt(nn)
		if vn == 0 {
			vn = 1.0
		}
		out = append(out, ranked{sim: dot / (qn * vn), msgID: r.msgID, sid: r.sid})
	}

	// Descending over (sim, msg_id, sid): sim first, then msg_id, then sid.
	sort.Slice(out, func(i, j int) bool {
		if out[i].sim != out[j].sim {
			return out[i].sim > out[j].sim
		}
		if out[i].msgID != out[j].msgID {
			return out[i].msgID > out[j].msgID
		}
		return out[i].sid > out[j].sid
	})
	if k < 0 {
		k = 0
	}
	if len(out) > k {
		out = out[:k]
	}
	return out
}

// VecKNN returns up to k vector-anchor VecHits nearest to qvec, existence-checked
// against the live messages table.
func VecKNN(con *sql.DB, qvec []float64, k int, includeSubagents bool) []VecHit {
	ctx := context.Background()
	rows, err := con.QueryContext(ctx, "SELECT msg_id, session_id, vec FROM chunk_vec")
	if err != nil {
		return []VecHit{} // missing table / read error reads as "no vectors"
	}
	stored := []knnRow{}
	for rows.Next() {
		var r knnRow
		if err := rows.Scan(&r.msgID, &r.sid, &r.blob); err != nil {
			rows.Close()
			return []VecHit{}
		}
		stored = append(stored, r)
	}
	errRows := rows.Err()
	rows.Close() // explicit (not defer): single-conn pool — free it before the existence-check query below
	if errRows != nil {
		return []VecHit{} // a late cursor error reads as "no vectors", not a partial set
	}
	if len(stored) == 0 {
		return []VecHit{}
	}

	cand := knn(qvec, stored, k*3)
	out := []VecHit{}
	for _, c := range cand {
		var (
			iso    sql.NullString
			parent sql.NullString
			isSub  int
		)
		err := con.QueryRowContext(ctx,
			"SELECT m.ts_iso, s.parent_id, s.is_subagent FROM messages m "+
				"JOIN sessions s ON s.id=m.session_id WHERE m.id=?", c.msgID,
		).Scan(&iso, &parent, &isSub)
		if err != nil { // churned / gone row
			continue
		}
		if isSub != 0 && !includeSubagents {
			continue
		}
		out = append(out, VecHit{
			ID:        c.msgID,
			SessionID: c.sid,
			ISO:       iso.String,
			Parent:    parent.String,
			Dist:      c.sim,
		})
		if len(out) >= k {
			break
		}
	}
	return out
}

// Fuse merges keyword anchors + vector KNN via RRF(k=RRFConstant) by message id,
// returning a merged anchor list ordered by fused score (each row's Fused set).
// Keyword-only rows keep their fields; vector-only rows are synthesized (Cov=0).
func Fuse(con *sql.DB, kwRows []retrieve.Anchor, qvec []float64, knnK int, includeSubagents bool) []retrieve.Anchor {
	vhits := VecKNN(con, qvec, knnK, includeSubagents)

	score := map[int]float64{}
	rowmap := map[int]retrieve.Anchor{}

	for rank, r := range kwRows {
		score[r.ID] += 1.0 / float64(RRFConstant+rank+1)
		rowmap[r.ID] = r
	}
	for rank, v := range vhits {
		score[v.ID] += 1.0 / float64(RRFConstant+rank+1)
		if _, ok := rowmap[v.ID]; !ok {
			// Vector-only synthesized anchor (Role empty, Snip empty, Cov 0).
			rowmap[v.ID] = retrieve.Anchor{
				ID:        v.ID,
				SessionID: v.SessionID,
				ISO:       v.ISO,
				Parent:    v.Parent,
				Cov:       0,
			}
		}
	}

	// Order by fused score descending. Map iteration is unordered, so we tiebreak
	// deterministically on id to keep output reproducible.
	ids := make([]int, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if score[ids[i]] != score[ids[j]] {
			return score[ids[i]] > score[ids[j]]
		}
		return ids[i] < ids[j]
	})

	merged := make([]retrieve.Anchor, 0, len(ids))
	for _, id := range ids {
		r := rowmap[id]
		r.Fused = score[id]
		merged = append(merged, r)
	}
	return merged
}
