package index

import (
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/durable"
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

// RefreshDBPath returns the private per-container cache used by targeted live
// refreshes. It lives below the cache root so normal scope/orphan discovery
// never mistakes it for another searchable project database.
func RefreshDBPath(sourceID, sessionID, sourcePath string) string {
	sum := sha1.Sum([]byte(sourceID + "\x00" + sessionID + "\x00" + realpath(sourcePath)))
	dir := filepath.Join(store.CacheDir(), "refresh")
	_ = os.MkdirAll(dir, 0o755)
	return filepath.Join(dir, hex.EncodeToString(sum[:])+".db")
}

// EnsureFreshContainer incrementally refreshes one live container, proves its
// watermark matches the current file, and strictly folds it into the
// consolidated store. Unlike ordinary advisory indexing, any uncertainty is an
// error: tag-prep must not print a known-stale partial transcript.
func EnsureFreshContainer(
	dbp string,
	c source.Container,
	msgs MessagesFunc,
	sourceID string,
) (int, error) {
	var loadErr error
	strictMessages := func(got source.Container) ([]model.Message, error) {
		ms, err := msgs(got)
		if err != nil {
			loadErr = err
		}
		return ms, err
	}

	_, status, err := ensureIndexedContainers(
		dbp,
		false,
		[]source.Container{c},
		strictMessages,
		sourceID,
		"",
	)
	if loadErr != nil {
		return 0, fmt.Errorf("read live transcript %s: %w", c.Path, loadErr)
	}
	if err != nil {
		return 0, err
	}
	if status != IndexFresh {
		return 0, fmt.Errorf("refresh cache %s is locked or stale", dbp)
	}

	nMessages, err := verifyFreshContainer(dbp, c)
	if err != nil {
		return 0, err
	}
	if err := SyncConsolidatedFrom(dbp); err != nil {
		return 0, fmt.Errorf("publish refreshed session %s: %w", c.ID, err)
	}
	if err := verifyConsolidatedCount(c.ID, nMessages); err != nil {
		return 0, err
	}
	return nMessages, nil
}

func backingFilePath(p string) string {
	if idx := strings.IndexByte(p, '#'); idx >= 0 {
		return p[:idx]
	}
	return p
}

func backingFileState(rawPath string) (mtime float64, size int64, fp string, err error) {
	st, statErr := os.Stat(rawPath)
	if statErr != nil {
		return 0, 0, "", statErr
	}
	mtime = mtimeOf(st)
	size = st.Size()
	fp = provenance.FileFingerprint(rawPath, size)

	// If a SQLite WAL file exists alongside the backing database, incorporate
	// its mtime, size, and fingerprint so uncheckpointed WAL writes trigger reindexing.
	if walSt, walErr := os.Stat(rawPath + "-wal"); walErr == nil && walSt.Size() > 0 {
		walMTime := mtimeOf(walSt)
		if walMTime > mtime {
			mtime = walMTime
		}
		size += walSt.Size()
		walFP := provenance.FileFingerprint(rawPath+"-wal", walSt.Size())
		fp = fp + "+" + walFP
	}
	return mtime, size, fp, nil
}

func verifyFreshContainer(dbp string, c source.Container) (int, error) {
	rawPath := backingFilePath(c.Path)
	wantMTime, wantSize, wantFP, err := backingFileState(rawPath)
	if err != nil {
		return 0, fmt.Errorf("stat live transcript %s: %w", rawPath, err)
	}
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return 0, fmt.Errorf("open refresh cache %s: %w", dbp, err)
	}
	defer con.Close()

	var (
		mtime     float64
		size      int64
		fp        string
		sessionID string
	)
	err = con.QueryRow(
		"SELECT mtime,size,fp,session_id FROM file_index WHERE path=?",
		realpath(c.Path),
	).Scan(&mtime, &size, &fp, &sessionID)
	if err != nil {
		return 0, fmt.Errorf("verify refreshed transcript %s: %w", c.Path, err)
	}
	if sessionID != c.ID || size != wantSize || absDiff(mtime, wantMTime) >= 0.001 || fp != wantFP {
		return 0, fmt.Errorf("live transcript %s changed or was not fully refreshed", c.Path)
	}

	var nMessages int
	if err := con.QueryRow("SELECT message_count FROM sessions WHERE id=?", c.ID).Scan(&nMessages); err != nil {
		return 0, fmt.Errorf("verify refreshed session %s: %w", c.ID, err)
	}
	return nMessages, nil
}

func verifyConsolidatedCount(sessionID string, minimum int) error {
	con, err := store.ConnectRO(ConsolidatedPath())
	if err != nil {
		return fmt.Errorf("open refreshed consolidated store: %w", err)
	}
	defer con.Close()
	var count int
	if err := con.QueryRow("SELECT message_count FROM sessions WHERE id=?", sessionID).Scan(&count); err != nil {
		return fmt.Errorf("verify published session %s: %w", sessionID, err)
	}
	if count < minimum {
		return fmt.Errorf("published session %s has %d messages, want at least %d", sessionID, count, minimum)
	}
	return nil
}

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
	nSessions, status, err = ensureIndexedContainers(dbp, reindex, cs, msgs, sourceID, origin)
	writeThroughConsolidated(dbp, err)
	return nSessions, status, err
}

func ensureIndexedContainers(dbp string, reindex bool, cs []source.Container, msgs MessagesFunc, sourceID, origin string) (nSessions int, status IndexStatus, err error) {
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
		return 0, IndexStatusUnknown, fmt.Errorf("ensure schema: %w", err)
	}
	if err := updateContainers(con, cs, msgs, sourceID, origin); err != nil {
		if isBusy(err) {
			return store.CountSessions(dbp), IndexStale, nil
		}
		return 0, IndexStatusUnknown, fmt.Errorf("update containers: %w", err)
	}
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&nSessions); err != nil {
		if isBusy(err) {
			return store.CountSessions(dbp), IndexStale, nil
		}
		return 0, IndexStatusUnknown, fmt.Errorf("count sessions: %w", err)
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
		rawPath := backingFilePath(c.Path)
		mtime, size, fp, err := backingFileState(rawPath)
		if err != nil {
			continue
		}
		if isMember(tombstoned, c.ID) {
			continue // user-deleted session: honor across reindex
		}
		var prev fileMeta
		var found bool
		if prev, found = cur[rp]; found {
			if absDiff(prev.mtime, mtime) < 0.001 && prev.size == size {
				if prev.fp == fp {
					continue // genuinely unchanged
				}
			}
		}

		// Fast path: incremental tail ingest if file grew append-only.
		if found && prev.size > 0 && size > prev.size {
			headFP := provenance.FileFingerprint(rawPath, prev.size)
			if headFP != "" && headFP == prev.fp {
				tailMs, newOffset, ok := parseTailMessages(con, c, sourceID, rawPath, prev.size, size)
				if ok {
					newFP := provenance.FileFingerprint(rawPath, newOffset)
					if err := appendContainer(con, c, tailMs, sourceID, origin, rp, mtime, newOffset, newFP); err == nil {
						IncrementalIngestCount.Add(1)
						continue
					}
				}
			}
		}

		ms, mErr := msgs(c)
		if mErr != nil {
			continue // bad container: leave existing rows + watermark untouched
		}
		FullReindexCount.Add(1)
		if err := reindexContainer(con, reindexContainerParams{
			container:   c,
			messages:    ms,
			sourceID:    sourceID,
			origin:      origin,
			path:        rp,
			mtime:       mtime,
			size:        size,
			fingerprint: fp,
		}); err != nil {
			return err
		}
	}

	// Retention pass (parallel of UpdateIndex): an absent own-source container is
	// flagged missing_since and retained; only an explicit tombstone deletes; a
	// foreign-origin row is never a candidate (D1/D2/D5). An ARCHIVE-replica
	// scan (origin set) instead treats absence as authoritative — the owner's
	// delete propagated through the archive (E5); see DecideRetention.
	now := nowEpoch()
	res, err := retention.ReconcileRetention(con, onDisk, tombstoned, now, retention.RetentionMirror(), origin != "")
	if err != nil {
		return err
	}
	applyRetentionToVault(res, now, origin)
	return nil
}

type reindexContainerParams struct {
	container   source.Container
	messages    []model.Message
	sourceID    string
	origin      string
	path        string
	mtime       float64
	size        int64
	fingerprint string
}

// reindexContainer atomically replaces one container's rows under a single
// transaction: messages, sessions row, and file_index watermark are written
// together. If any statement or the durable vault write fails, the transaction
// is rolled back so readers never observe a partial session or a committed
// session without its watermark. Returns an error on any failure.
//
// The begin is DEFERRED (database/sql's default), not BEGIN IMMEDIATE:
// store.ConnectRW calls SetMaxOpenConns(1) to limit that one pool, not the
// database file — another process or call (such as the indexer and tag-write
// path) can open the same file concurrently. Deferred is safe because the
// transaction's first statement is already a write (DELETE), acquiring the
// write lock immediately without prior reads to risk a lock-upgrade race.
func reindexContainer(con *sql.DB, params reindexContainerParams) error {
	c := params.container
	ms := params.messages
	sourceID := params.sourceID
	origin := params.origin
	rp := params.path
	mtime := params.mtime
	size := params.size
	fp := params.fingerprint

	tx, err := con.Begin()
	if err != nil {
		return fmt.Errorf("session %s begin tx: %w", c.ID, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec("DELETE FROM messages WHERE session_id=?", c.ID); err != nil {
		return fmt.Errorf("session %s delete messages: %w", c.ID, err)
	}
	if _, err := tx.Exec("DELETE FROM sessions WHERE id=?", c.ID); err != nil {
		return fmt.Errorf("session %s delete session: %w", c.ID, err)
	}
	var started, last float64
	var startedSet, lastSet bool
	for _, m := range ms {
		if _, err := tx.Exec(
			"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
			c.ID, m.Role, m.Text, m.TS, m.TSISO, m.UUID,
		); err != nil {
			return fmt.Errorf("session %s insert message: %w", c.ID, err)
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
	projectArg, cwdArg := scopeOf(c.CWD, projectScope{})
	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id,origin_machine,source_tool,source_path,missing_since,project,cwd) VALUES(?,?,?,?,?,?,?,?,?,NULL,?,?)",
		c.ID, started, last, len(ms), b2i(c.IsSubagent), parentArg, originOr(origin), sourceID, realpath(c.Path), projectArg, cwdArg,
	); err != nil {
		return fmt.Errorf("session %s insert session: %w", c.ID, err)
	}

	// Drop any watermark this session held under a DIFFERENT path before writing
	// the current one. file_index is keyed by path, not session_id, so a source
	// that renames its backing file — Antigravity prefers transcript_full.jsonl
	// once it appears, having previously served transcript.jsonl for the same
	// session — otherwise leaves the old row behind. That orphan then names a file
	// that no longer exists, retention reads it as a purged source and prunes the
	// session, and the still-current watermark makes the next scan skip
	// re-indexing it: the session disappears and never comes back.
	if _, err := tx.Exec("DELETE FROM file_index WHERE session_id=? AND path<>?", c.ID, rp); err != nil {
		// Returned, not logged: the caller owns the single handling of this error.
		// Logging here as well would emit two entries for one failure.
		return fmt.Errorf("delete stale file_index for %s: %w", c.ID, err)
	}
	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO file_index(path,mtime,size,fp,session_id) VALUES(?,?,?,?,?)",
		rp, mtime, size, fp, c.ID,
	); err != nil {
		return fmt.Errorf("session %s insert file_index: %w", c.ID, err)
	}

	// Vault rawclaw's own copy inside the atomic success gate: own sessions
	// only, and a failure rolls back the transaction and withholds the watermark.
	if origin == "" {
		if err := vaultContainer(c, ms, sourceID, projectArg, cwdArg); err != nil {
			return fmt.Errorf("session %s vault container: %w", c.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("session %s commit tx: %w", c.ID, err)
	}
	return nil
}

// appendContainer atomically appends new messages to an existing session under a single
// transaction: messages, sessions message_count/last_ts, and file_index watermark are
// updated together. If any statement or vault write fails, the transaction is rolled back.
func appendContainer(con *sql.DB, c source.Container, ms []model.Message, sourceID, origin, rp string, mtime float64, size int64, fp string) error {
	tx, err := con.Begin()
	if err != nil {
		return fmt.Errorf("session %s begin tx for append: %w", c.ID, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var maxTS float64
	for _, m := range ms {
		if _, err := tx.Exec(
			"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
			c.ID, m.Role, m.Text, m.TS, m.TSISO, m.UUID,
		); err != nil {
			return fmt.Errorf("session %s append message: %w", c.ID, err)
		}
		if m.TS > maxTS {
			maxTS = m.TS
		}
	}

	if _, err := tx.Exec(`
		UPDATE sessions
		SET message_count = (SELECT COUNT(*) FROM messages WHERE messages.session_id = sessions.id),
		    last_ts = CASE WHEN ? > COALESCE(last_ts, 0) THEN ? ELSE last_ts END,
		    missing_since = NULL
		WHERE id = ?`,
		maxTS, maxTS, c.ID,
	); err != nil {
		return fmt.Errorf("session %s update session on append: %w", c.ID, err)
	}

	if _, err := tx.Exec("DELETE FROM file_index WHERE session_id=? AND path<>?", c.ID, rp); err != nil {
		return fmt.Errorf("delete stale file_index for %s: %w", c.ID, err)
	}
	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO file_index(path,mtime,size,fp,session_id) VALUES(?,?,?,?,?)",
		rp, mtime, size, fp, c.ID,
	); err != nil {
		return fmt.Errorf("session %s insert file_index on append: %w", c.ID, err)
	}

	if origin == "" {
		projectArg, cwdArg := scopeOf(c.CWD, projectScope{})
		if err := vaultContainerAll(tx, c, sourceID, projectArg, cwdArg); err != nil {
			return fmt.Errorf("session %s vault container on append: %w", c.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("session %s commit tx for append: %w", c.ID, err)
	}
	return nil
}

// vaultContainer stores rawclaw's own copy of a container-sourced session.
func vaultContainer(c source.Container, ms []model.Message, sourceID string, projectArg, cwdArg any) error {
	m := durable.Meta{
		ID:         c.ID,
		Source:     sourceID,
		Project:    strOf(projectArg),
		CWD:        strOf(cwdArg),
		IsSubagent: c.IsSubagent,
		ParentID:   c.ParentID,
		SourcePath: realpath(c.Path),
	}
	rawPath := backingFilePath(c.Path)
	if mtime, size, fp, err := backingFileState(rawPath); err == nil {
		m.SourceMTime = mtime
		m.SourceSize = size
		m.SourceFP = fp
	}
	return durable.StoreMessages(m, ms)
}

// vaultContainerAll vaults a complete session after append.
func vaultContainerAll(tx *sql.Tx, c source.Container, sourceID string, projectArg, cwdArg any) error {
	m := durable.Meta{
		ID:         c.ID,
		Source:     sourceID,
		Project:    strOf(projectArg),
		CWD:        strOf(cwdArg),
		IsSubagent: c.IsSubagent,
		ParentID:   c.ParentID,
		SourcePath: realpath(c.Path),
	}
	rawPath := backingFilePath(c.Path)
	if mtime, size, fp, err := backingFileState(rawPath); err == nil {
		m.SourceMTime = mtime
		m.SourceSize = size
		m.SourceFP = fp
	}
	if sourceID == sourceClaude || sourceID == "" {
		return durable.StoreFile(m, rawPath)
	}
	rows, err := tx.Query("SELECT role, content, ts, ts_iso, uuid FROM messages WHERE session_id=? ORDER BY id ASC", c.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var msgs []model.Message
	for rows.Next() {
		var msg model.Message
		if err := rows.Scan(&msg.Role, &msg.Text, &msg.TS, &msg.TSISO, &msg.UUID); err != nil {
			return err
		}
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return durable.StoreMessages(m, msgs)
}

// b2i maps a bool to the 0/1 the is_subagent column stores.
func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
