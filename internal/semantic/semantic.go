// Package semantic is the optional vector channel: a float32-BLOB VectorStore
// living in the SAME cache db as the keyword index (under its own schema gate),
// brute-force cosine KNN in plain Go, and reciprocal-rank fusion.
//
// It is NEVER in the keyword _rebuild() drop list and never bumps the keyword
// SchemaVersion — a keyword reindex can't nuke vectors and vice versa. Vectors
// are keyed by (session_id, content_hash) so msg-id churn on reindex is
// harmless.
//
// Vectors are packed as little-endian float32 BLOBs. The chunk_vec SQL itself
// lives in the store package (D4) — semantic keeps hashing, embedding, cosine
// KNN, and RRF fusion.
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
	"strings"
	"sync"

	"github.com/MoonCaves/rawclaw/internal/embed"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// MinChars skips trivial messages (greetings, acks).
const MinChars = 12

// RRFConstant is the reciprocal-rank-fusion k.
const RRFConstant = 60

// VecHit is one vector-KNN anchor: id, session_id, iso, parent, dist.
type VecHit struct {
	ID           int
	SessionID    string
	ISO          string
	Parent       string
	MissingSince float64 // sessions.missing_since (0 when NULL); carried so a vector-only hit on a retained-but-missing session keeps the D7 flag
	Dist         float64 // cosine similarity (higher = nearer)
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

type embedItem struct {
	key  vecKey
	mid  int
	text string
}

// VecIndex embeds any message (tool-stripped prose ≥ MinChars) not yet vectored,
// prunes stale vectors, and refreshes churned msg_ids. Resumable; returns the
// count added. `maxNew` of 0 = no cap.
// ctx bounds the embedding pass. It is load-bearing rather than decorative:
// the batch path fans out to several goroutines each blocked on a remote HTTP
// call, and without a cancellation signal a Ctrl-C leaves them running to
// completion against the endpoint while the caller has already gone. Every
// vector committed before cancellation stays committed — the pass is
// resumable, so a cancelled run costs only the batch in flight.
func VecIndex(ctx context.Context, con *sql.DB, embedder embed.Embedder, maxNew int) (added int, err error) {
	if err := store.EnsureVecSchema(con); err != nil {
		return 0, err
	}

	// Build the current (sid, hash) -> (msg_id, text) map from live messages.
	// store.AllMessages fully drains its scan before returning, so the single
	// connection is free for the chunk_vec scan below (D3).
	msgs, err := store.AllMessages(con)
	if err != nil {
		return 0, fmt.Errorf("scan messages: %w", err)
	}
	current := map[vecKey]curEntry{}
	for _, m := range msgs {
		// StripGenerated, not StripTools: every retrieval path scores against
		// the generated-stripped form, and the vector arm has to see the same
		// text or the two halves of the fusion are reading different corpora.
		// StripTools only drops tool runs; it leaves the runtime's own injected
		// envelopes — system reminders, task notifications, slash-command
		// plumbing, captured shell IO — which are machinery nobody said, repeat
		// near-identically across thousands of messages, and would crowd real
		// neighbours out of the cosine ranking.
		//
		// This changes the content hash for any message that carried an
		// envelope, so the prune pass below drops those stale vectors and they
		// re-embed from the clean text. Messages that never had one hash
		// identically and are left alone.
		text := strings.TrimSpace(parse.StripGenerated(m.Content))
		if len([]rune(text)) < MinChars {
			continue
		}
		current[vecKey{m.SessionID, contentHash(text)}] = curEntry{m.ID, text}
	}

	// Load what we already have: (sid, hash) -> stored msg_id.
	stored, err := store.VecAll(con)
	if err != nil {
		return 0, fmt.Errorf("scan chunk_vec: %w", err)
	}
	have := map[vecKey]int{}
	for _, r := range stored {
		have[vecKey{r.SessionID, r.ContentHash}] = r.MsgID
	}

	// Prune vectors whose source text no longer exists.
	for k := range have {
		if _, ok := current[k]; ok {
			continue
		}
		if err := store.VecPrune(con, k.sid, k.hash); err != nil {
			return 0, fmt.Errorf("prune stale vector: %w", err)
		}
	}

	// Collect missing messages to embed; refresh churned msg_ids (no re-embed).
	var toEmbed []embedItem
	for k, e := range current {
		if storedMID, ok := have[k]; ok {
			if storedMID != e.mid { // id churned on a reindex — refresh, don't re-embed
				if err := store.VecRefreshMsgID(con, k.sid, k.hash, e.mid); err != nil {
					return 0, fmt.Errorf("refresh msg_id: %w", err)
				}
			}
			continue
		}
		toEmbed = append(toEmbed, embedItem{key: k, mid: e.mid, text: e.text})
		if maxNew != 0 && len(toEmbed) >= maxNew {
			break
		}
	}

	if embedder == nil || len(toEmbed) == 0 {
		return 0, nil
	}

	bEmbedder, isBatch := embedder.(embed.BatchEmbedder)
	if !isBatch {
		// Serial fallback path for non-batch embedders.
		for _, item := range toEmbed {
			if ctx.Err() != nil {
				return added, nil // cancelled: keep what landed, the pass resumes
			}
			vec := embedder.Embed(item.text)
			if len(vec) == 0 {
				continue
			}
			if err := store.VecUpsert(con, item.key.sid, item.key.hash, item.mid, len(vec), packVec(vec)); err != nil {
				return 0, fmt.Errorf("insert vector: %w", err)
			}
			added++
			if maxNew != 0 && added >= maxNew {
				break
			}
		}
		return added, nil
	}

	// Batch path: group into batches bounded by 128 items OR 150000 characters.
	var batches [][]embedItem
	var currentBatch []embedItem
	currentChars := 0

	for _, item := range toEmbed {
		itemChars := len(item.text)
		if len(currentBatch) > 0 && (len(currentBatch) >= 128 || currentChars+itemChars > 150000) {
			batches = append(batches, currentBatch)
			currentBatch = nil
			currentChars = 0
		}
		currentBatch = append(currentBatch, item)
		currentChars += itemChars
	}
	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	// Concurrent HTTP embedding (up to 8 batches) funneled back to single-threaded DB writes.
	type vecResult struct {
		item embedItem
		vec  []float64
	}

	numWorkers := 8
	if len(batches) < numWorkers {
		numWorkers = len(batches)
	}

	batchChan := make(chan []embedItem, len(batches))
	for _, b := range batches {
		batchChan <- b
	}
	close(batchChan)

	// Buffer only one slot per worker. A buffer the size of the whole batch list
	// would let the workers run the entire corpus ahead of the writer and pile
	// every vector in memory, which is exactly what the streaming close above is
	// there to avoid; this size gives the writer backpressure instead.
	resultsChan := make(chan []vecResult, numWorkers)
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range batchChan {
				if ctx.Err() != nil {
					return // drain-and-exit: the range keeps the channel from leaking
				}
				texts := make([]string, len(batch))
				for j, it := range batch {
					texts[j] = it.text
				}

				vecs := bEmbedder.EmbedBatch(texts)
				if vecs == nil || len(vecs) != len(batch) {
					// Fall back to per-item Embed for just this batch if EmbedBatch fails.
					vecs = make([][]float64, len(batch))
					for j, it := range batch {
						vecs[j] = embedder.Embed(it.text)
					}
				}

				resBatch := make([]vecResult, 0, len(batch))
				for j, vec := range vecs {
					if len(vec) > 0 {
						resBatch = append(resBatch, vecResult{item: batch[j], vec: vec})
					}
				}
				select {
				case resultsChan <- resBatch:
				case <-ctx.Done():
					return // writer is gone; do not block on a send nobody will read
				}
			}
		}()
	}

	// Close the results channel once every worker has finished, from a separate
	// goroutine, so the write loop below can drain results AS THEY ARRIVE rather
	// than after the whole corpus is embedded. Waiting here instead would hold
	// every vector in memory until the last batch returned (~8KB per message —
	// gigabytes on a large corpus) and would lose the entire run on a crash,
	// which defeats the resumability this function is built for.
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Single-threaded DB writes on con to prevent SQLite locks.
	for resBatch := range resultsChan {
		if ctx.Err() != nil {
			break // stop committing; workers see the same signal and unwind
		}
		for _, res := range resBatch {
			if maxNew != 0 && added >= maxNew {
				break
			}
			if err := store.VecUpsert(con, res.item.key.sid, res.item.key.hash, res.item.mid, len(res.vec), packVec(res.vec)); err != nil {
				return 0, fmt.Errorf("insert vector: %w", err)
			}
			added++
		}
	}

	return added, nil
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
func knn(qvec []float64, rows []store.VecRow, k int) []ranked {
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
		// Score straight off the packed blob rather than unpackVec'ing it first.
		// The scan is brute-force over EVERY stored vector, so a per-row
		// []float64 is one 8KB allocation per row: at 86k stored vectors that is
		// ~700MB of garbage produced and collected on every search. Measured on
		// that corpus, the scoring loop goes 105ms -> 34ms. Reading the float32s
		// in place keeps the arithmetic identical — the stored values were
		// always float32 precision, so unpacking never added any.
		//
		// This is the whole scan strategy: brute force, no ANN index. It is
		// linear in corpus size (~4KB read per vector), which is fine at this
		// scale and is the thing to revisit if the corpus grows an order of
		// magnitude — not the allocation.
		if len(r.Vec) != len(qvec)*4 {
			continue
		}
		dot, nn := 0.0, 0.0
		for i, q := range qvec {
			v := float64(math.Float32frombits(binary.LittleEndian.Uint32(r.Vec[i*4:])))
			dot += q * v
			nn += v * v
		}
		vn := math.Sqrt(nn)
		if vn == 0 {
			vn = 1.0
		}
		out = append(out, ranked{sim: dot / (qn * vn), msgID: r.MsgID, sid: r.SessionID})
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
	// store.VecAll fully drains + closes its scan before returning — the
	// single-conn pool is free for the existence-check queries below (D3). Any
	// error (missing table, read error, late cursor error) reads as "no
	// vectors", never a partial set.
	stored, err := store.VecAll(con)
	if err != nil {
		return []VecHit{}
	}
	if len(stored) == 0 {
		return []VecHit{}
	}

	cand := knn(qvec, stored, k*3)
	out := []VecHit{}
	for _, c := range cand {
		iso, parent, isSub, missing, ok := store.MessageMeta(con, c.msgID)
		if !ok { // churned / gone row
			continue
		}
		if isSub && !includeSubagents {
			continue
		}
		out = append(out, VecHit{
			ID:           c.msgID,
			SessionID:    c.sid,
			ISO:          iso,
			Parent:       parent,
			MissingSince: missing, // 0 when NULL (present)
			Dist:         c.sim,
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
			// Carry missing_since so a retained-but-missing session matched only by
			// vector still surfaces the D7 flag (a keyword-also-hit keeps its own
			// Anchor, which already carries it — that branch is untouched).
			rowmap[v.ID] = retrieve.Anchor{
				ID:           v.ID,
				SessionID:    v.SessionID,
				ISO:          v.ISO,
				Parent:       v.Parent,
				MissingSince: v.MissingSince,
				Cov:          0,
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

// CoverageStats reports candidate embedding coverage across a store.
type CoverageStats struct {
	Candidates int
	Vectored   int
	Missing    int
}

// MeasureCoverage scans live candidate messages (tool-stripped prose >= MinChars)
// and compares them against stored vectors in chunk_vec. If projects is provided,
// only messages belonging to those projects are considered.
func MeasureCoverage(con *sql.DB, projects ...string) (CoverageStats, error) {
	var msgs []store.MessageRow
	var err error
	if len(projects) > 0 {
		msgs, err = store.MessagesForProjects(con, projects)
	} else {
		msgs, err = store.AllMessages(con)
	}
	if err != nil {
		return CoverageStats{}, fmt.Errorf("scan messages: %w", err)
	}

	current := map[vecKey]struct{}{}
	for _, m := range msgs {
		text := strings.TrimSpace(parse.StripGenerated(m.Content))
		if len([]rune(text)) < MinChars {
			continue
		}
		current[vecKey{m.SessionID, contentHash(text)}] = struct{}{}
	}
	if len(current) == 0 {
		return CoverageStats{Candidates: 0, Vectored: 0, Missing: 0}, nil
	}

	keys, err := store.VecKeys(con)
	if err != nil {
		// Missing chunk_vec table or query failure -> 0 vectored.
		return CoverageStats{
			Candidates: len(current),
			Vectored:   0,
			Missing:    len(current),
		}, nil
	}

	vectored := 0
	for _, k := range keys {
		if _, ok := current[vecKey{k.SessionID, k.ContentHash}]; ok {
			vectored++
		}
	}

	return CoverageStats{
		Candidates: len(current),
		Vectored:   vectored,
		Missing:    len(current) - vectored,
	}, nil
}
