// Vectors surface (D4): the chunk_vec table's DDL and row-level SQL. The
// semantic package keeps hashing, embedding, cosine KNN, and RRF fusion — store
// owns only the SQL so table/column names live in one package. Vectors are
// keyed by (session_id, content_hash) so msg-id churn on reindex is harmless;
// the table sits under its OWN schema gate and is NEVER in the keyword
// Rebuild() drop list, so a keyword reindex can't nuke vectors.
package store

import (
	"database/sql"
	"fmt"
	"strconv"
)

// VecSchemaVersion gates the chunk_vec table separately from the keyword schema.
const VecSchemaVersion = 1

// EnsureVecSchema creates the chunk_vec table (its own gate, separate from the
// keyword schema) and stamps the vec_schema_version meta key. Idempotent.
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
// error reads as false. [semantic.HasVectors]
func HasVectors(con *sql.DB) bool {
	var one int
	err := con.QueryRow("SELECT 1 FROM chunk_vec LIMIT 1").Scan(&one)
	return err == nil
}

// VecRow is one stored vector row: the composite key, the live message rowid,
// and the packed little-endian float32 blob. (The dim column is not surfaced —
// no consumer reads it; unpacking infers the dimension from the blob length.)
type VecRow struct {
	SessionID   string
	ContentHash string
	MsgID       int
	Vec         []byte
}

// VecAll returns every chunk_vec row (unordered table scan) — the shared load
// for the indexer's have-map (which reads the key + msg_id) and the KNN scan
// (which reads msg_id + vec). The rows are fully drained before returning, so
// the single connection is free for follow-up queries (D3).
// [semantic.VecIndex, semantic.VecKNN]
func VecAll(con *sql.DB) ([]VecRow, error) {
	rows, err := con.Query("SELECT session_id, content_hash, msg_id, vec FROM chunk_vec")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VecRow
	for rows.Next() {
		var r VecRow
		if err := rows.Scan(&r.SessionID, &r.ContentHash, &r.MsgID, &r.Vec); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// VecUpsert inserts or replaces one vector row, keyed by
// (session_id, content_hash). vec is the packed little-endian float32 blob;
// dim is its element count. [semantic.VecIndex]
func VecUpsert(con *sql.DB, sid, contentHash string, msgID, dim int, vec []byte) error {
	_, err := con.Exec(
		"INSERT OR REPLACE INTO chunk_vec(session_id,content_hash,msg_id,dim,vec) VALUES(?,?,?,?,?)",
		sid, contentHash, msgID, dim, vec)
	return err
}

// VecPrune deletes the vector row keyed by (session_id, content_hash) — the
// stale-vector prune when the source text no longer exists. [semantic.VecIndex]
func VecPrune(con *sql.DB, sid, contentHash string) error {
	_, err := con.Exec(
		"DELETE FROM chunk_vec WHERE session_id=? AND content_hash=?", sid, contentHash)
	return err
}

// VecRefreshMsgID re-points a stored vector at a churned message rowid without
// re-embedding (id churn on reindex is expected; the content hash is the stable
// key). [semantic.VecIndex]
func VecRefreshMsgID(con *sql.DB, sid, contentHash string, msgID int) error {
	_, err := con.Exec(
		"UPDATE chunk_vec SET msg_id=? WHERE session_id=? AND content_hash=?",
		msgID, sid, contentHash)
	return err
}
