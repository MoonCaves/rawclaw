package index

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/provenance"
	"github.com/MoonCaves/rawclaw/internal/retention"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// MessagesFunc yields one container's normalized messages — a source adapter's
// Messages method, injected so this package never imports the concrete adapters.
// The index stays source-agnostic; the caller (cli) wires source → index.
type MessagesFunc func(source.Container) ([]model.Message, error)

// EnsureIndexedContainers builds/updates the db at dbp from cs (one scope's
// containers), pulling each container's messages via msgs. It mirrors
// EnsureIndexed's reindex + busy-lock semantics, but is source-agnostic: the
// containers carry their own id, lineage, and backing path, replacing the
// Claude-only directory walk of UpdateIndex. sourceID (the source's
// Registration.ID, e.g. "codex") is stamped as each row's source_tool (D3),
// injected alongside msgs so the index never imports the concrete adapters.
// origin is the origin_machine to stamp ("" = this machine) — a replicated
// tree's containers carry their owner's identity.
//
// CONTRACT — cs MUST be the COMPLETE container set for dbp on every call. The
// retention pass (updateContainers) reconciles indexed sessions against cs as
// the full live scan: in a REPLICA scope (origin set) an omitted session is
// pruned outright; in a local scope it is stamped missing_since — either way
// a partial cs corrupts the outcome for the omitted sessions. Corollary:
// never point two sources (or two scopes) at the same dbp — give each its
// own, distinctly-namespaced cache file, so one source's set is never
// "incomplete" relative to another's rows.
func EnsureIndexedContainers(dbp string, reindex bool, cs []source.Container, msgs MessagesFunc, sourceID, origin string) (nSessions int, status IndexStatus, err error) {
	if reindex {
		if _, statErr := os.Stat(dbp); statErr == nil {
			_ = os.Remove(dbp) // best-effort; ignore a remove error
		}
	}
	con, openErr := store.ConnectRW(dbp)
	if openErr != nil {
		return store.CountSessions(dbp), IndexStale, nil
	}
	defer con.Close()

	if err := EnsureSchema(con, sourceID); err != nil {
		if isBusy(err) {
			return store.CountSessions(dbp), IndexStale, nil
		}
		return 0, IndexFresh, fmt.Errorf("ensure schema: %w", err)
	}
	if err := updateContainers(con, cs, msgs, sourceID, origin); err != nil {
		if isBusy(err) {
			return store.CountSessions(dbp), IndexStale, nil
		}
		return 0, IndexFresh, fmt.Errorf("update containers: %w", err)
	}
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&nSessions); err != nil {
		if isBusy(err) {
			return store.CountSessions(dbp), IndexStale, nil
		}
		return 0, IndexFresh, fmt.Errorf("count sessions: %w", err)
	}
	return nSessions, IndexFresh, nil
}

// updateContainers watermarks each container by its backing file, reindexes
// the changed ones, and runs the retention pass over the rest (replica-scope
// absence prunes; local-scope absence retains-and-flags) — the container-
// driven parallel of UpdateIndex. A container whose messages fail to load is
// left untouched (existing rows + watermark preserved), never partially
// written.
func updateContainers(con *sql.DB, cs []source.Container, msgs MessagesFunc, sourceID, origin string) error {
	onDisk := make(map[string]struct{}, len(cs))
	for _, c := range cs {
		onDisk[realpath(c.Path)] = struct{}{}
	}

	tombstoned, terr := lifecycle.LoadTombstones("")
	if terr != nil {
		tombstoned = map[string]struct{}{} // best-effort: never block indexing
	}

	cur, err := loadFileIndex(con)
	if err != nil {
		return fmt.Errorf("load file_index: %w", err)
	}

	for _, c := range cs {
		rp := realpath(c.Path)
		st, err := os.Stat(c.Path)
		if err != nil {
			continue
		}
		if isMember(tombstoned, c.ID) {
			continue // user-deleted session: honor across reindex
		}
		mtime := mtimeOf(st)
		size := st.Size()
		if prev, found := cur[rp]; found {
			if absDiff(prev.mtime, mtime) < 0.001 && prev.size == size {
				if prev.fp == provenance.FileFingerprint(c.Path, size) {
					continue // genuinely unchanged
				}
			}
		}
		ms, mErr := msgs(c)
		if mErr != nil {
			continue // bad container: leave existing rows + watermark untouched
		}
		if reindexContainer(con, c, ms, sourceID, origin) {
			if _, err := con.Exec(
				"INSERT OR REPLACE INTO file_index(path,mtime,size,fp,session_id) VALUES(?,?,?,?,?)",
				rp, mtime, size, provenance.FileFingerprint(c.Path, size), c.ID,
			); err != nil {
				return fmt.Errorf("update file_index: %w", err)
			}
		}
	}

	// Retention pass (parallel of UpdateIndex): an absent own-source container is
	// flagged missing_since and retained; only an explicit tombstone deletes; a
	// foreign-origin row is never a candidate (D1/D2/D5). An ARCHIVE-replica
	// scan (origin set) instead treats absence as authoritative — the owner's
	// delete propagated through the archive (E5); see DecideRetention.
	if err := retention.ReconcileRetention(con, onDisk, tombstoned, nowEpoch(), retention.RetentionMirror(), origin != ""); err != nil {
		return err
	}
	return nil
}

// reindexContainer atomically replaces one container's rows: the messages are
// already parsed into ms, so a write failure can't commit away existing data
// (delete + insert run under database/sql autocommit). Returns false on any
// write error. Mirrors ReindexFile for the container path. sourceID is stamped as
// the row's source_tool (D3); origin as its origin_machine ("" = this machine).
func reindexContainer(con *sql.DB, c source.Container, ms []model.Message, sourceID, origin string) bool {
	if _, err := con.Exec("DELETE FROM messages WHERE session_id=?", c.ID); err != nil {
		return false
	}
	if _, err := con.Exec("DELETE FROM sessions WHERE id=?", c.ID); err != nil {
		return false
	}
	var started, last float64
	var startedSet, lastSet bool
	for _, m := range ms {
		if _, err := con.Exec(
			"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
			c.ID, m.Role, m.Text, m.TS, m.TSISO, m.UUID,
		); err != nil {
			return false
		}
		if m.TS != 0 {
			if !startedSet || m.TS < started {
				started, startedSet = m.TS, true
			}
			if !lastSet || m.TS > last {
				last, lastSet = m.TS, true
			}
		}
	}
	var parentArg any
	if c.ParentID != "" {
		parentArg = c.ParentID
	} // else nil → SQL NULL
	// Stamp provenance (D3); missing_since NULL — a (re)indexed container is present.
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id,origin_machine,source_tool,source_path,missing_since) VALUES(?,?,?,?,?,?,?,?,?,NULL)",
		c.ID, started, last, len(ms), b2i(c.IsSubagent), parentArg, originOr(origin), sourceID, realpath(c.Path),
	); err != nil {
		return false
	}
	return true
}

// b2i maps a bool to the 0/1 the is_subagent column stores.
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
