package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MoonCaves/rawclaw/internal/durable"
	"github.com/MoonCaves/rawclaw/internal/lifecycle"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// RebuildStats reports what one rebuild-from-transcripts pass produced.
type RebuildStats struct {
	Sessions   int // sessions written into the store
	Messages   int // messages written into the store
	Missing    int // of those sessions, ones whose original source file is gone
	Tombstoned int // vaulted sessions skipped because the user deleted them
	Unreadable int // vaulted transcripts that could not be read (reported, never silent)
}

// ErrRebuildWouldLoseHistory reports a rebuild refused because the vault holds
// fewer sessions than the store it was about to replace. Callers match on it to
// print the override rather than a bare failure.
var ErrRebuildWouldLoseHistory = errors.New("rebuild would lose history")

// rebuildForceEnv is the override for that refusal. It is an environment
// variable rather than a flag because the honest use for it is a scripted
// recovery on a machine whose store is already known to be junk.
const rebuildForceEnv = "RAWCLAW_REBUILD_FORCE"

// rebuildBeforeSwapHook is nil in normal operation. Tests use it to force a
// failure after the replacement has been built but before it can replace the
// live store, exercising the data-preservation boundary deterministically.
var rebuildBeforeSwapHook func() error

// rebuildForced reports whether the user asked to rebuild anyway.
func rebuildForced() bool {
	v := os.Getenv(rebuildForceEnv)
	return v != "" && v != "0"
}

// storedSessionCount reads how many sessions the store at dbp currently holds,
// WITHOUT touching its schema. EnsureSchema must never run here: it rebuilds a
// database whose schema_version does not match, which would drop the very rows
// this count exists to protect. A missing or unreadable store counts as zero,
// so a genuine recovery from nothing is never blocked.
func storedSessionCount(dbp string) (int, error) {
	if _, err := os.Stat(dbp); err != nil {
		return 0, err
	}
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return 0, err
	}
	defer con.Close()
	var n int
	if err := con.QueryRow("SELECT COUNT(*) FROM sessions").Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// RebuildFromTranscripts rebuilds the index db at dbp from the durable
// transcript vault alone — the guarantee that makes the store a cache: delete
// it, run this, and every session comes back, INCLUDING the ones whose original
// source file no longer exists anywhere on disk.
//
// There is deliberately NO retention pass here. Retention reconciles indexed
// sessions against a live source scan; this pass has no scan — its input is the
// vault, whose own sidecars already carry the verdict a previous scan reached.
// Running one here would see every vaulted session's source path and re-derive
// the flags from scratch, which is both redundant and wrong for a session that
// was already retained-and-flagged.
func RebuildFromTranscripts(dbp string) (RebuildStats, error) {
	var st RebuildStats
	preserved, err := readTagState(dbp)
	if err != nil {
		return st, fmt.Errorf("preserve tags: %w", err)
	}

	vaulted, err := durable.List()
	if err != nil {
		return st, err
	}

	// A user delete outlives the store it was applied to: the tombstone sidecar
	// lives beside the db, not inside it, so a rebuild must honor it or deleting
	// a session would be undone by the next rebuild.
	tombstoned, terr := lifecycle.LoadTombstones("")
	if terr != nil {
		tombstoned = map[string]struct{}{}
	}

	// Refuse to trade a populated store for a thinner vault. The vault only
	// fills as sessions are indexed, so on a machine that already had a large
	// corpus before the vault existed it starts out empty or sparse — and this
	// command deletes the store outright. Recovering less history than you had
	// is not recovery, so a rebuild that would lose sessions stops and says by
	// how much instead. ErrRebuildWouldLoseHistory names the escape hatch.
	have, herr := storedSessionCount(dbp)
	if herr == nil && have > len(vaulted) && !rebuildForced() {
		return st, fmt.Errorf("%w: the store holds %d sessions but the transcript vault holds %d; "+
			"rebuilding now would lose %d. Index normally to fill the vault first, or set %s=1 to override",
			ErrRebuildWouldLoseHistory, have, len(vaulted), have-len(vaulted), rebuildForceEnv)
	}

	tempDir, err := os.MkdirTemp(filepath.Dir(dbp), ".rawclaw-rebuild-*")
	if err != nil {
		return st, fmt.Errorf("create replacement directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()
	tempDB := filepath.Join(tempDir, filepath.Base(dbp))

	con, err := store.ConnectRW(tempDB)
	if err != nil {
		return st, fmt.Errorf("open replacement store: %w", err)
	}
	closeStore := func() error {
		if _, err := con.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			_ = con.Close()
			return fmt.Errorf("checkpoint replacement store: %w", err)
		}
		if err := con.Close(); err != nil {
			return fmt.Errorf("close replacement store: %w", err)
		}
		con = nil
		return nil
	}
	defer func() {
		if con != nil {
			_ = con.Close()
		}
	}()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		return st, fmt.Errorf("ensure schema: %w", err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		return st, fmt.Errorf("ensure topic schema: %w", err)
	}
	if err := restoreTagState(con, preserved); err != nil {
		return st, fmt.Errorf("restore tags: %w", err)
	}

	for _, v := range vaulted {
		if isMember(tombstoned, v.ID) {
			st.Tombstoned++
			continue
		}
		rows, started, last, fileCWD, ok := parseTranscript(v.Transcript, v.ID)
		if !ok {
			st.Unreadable++
			continue
		}
		if err := restoreSession(con, restoreSessionParams{
			session: v,
			rows:    rows,
			started: started,
			last:    last,
			fileCWD: fileCWD,
		}); err != nil {
			return st, err
		}
		st.Sessions++
		st.Messages += len(rows)
		if v.MissingSince != 0 {
			st.Missing++
		}
	}
	if err := closeStore(); err != nil {
		return st, err
	}
	if rebuildBeforeSwapHook != nil {
		if err := rebuildBeforeSwapHook(); err != nil {
			return st, fmt.Errorf("prepare store swap: %w", err)
		}
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Remove(tempDB + suffix); err != nil && !os.IsNotExist(err) {
			return st, fmt.Errorf("remove replacement sidecar %s: %w", tempDB+suffix, err)
		}
		if err := os.Remove(dbp + suffix); err != nil && !os.IsNotExist(err) {
			return st, fmt.Errorf("remove old sidecar %s: %w", dbp+suffix, err)
		}
	}
	if err := os.Rename(tempDB, dbp); err != nil {
		return st, fmt.Errorf("swap rebuilt store: %w", err)
	}
	return st, nil
}

type restoreSessionParams struct {
	session durable.Session
	rows    []reindexRow
	started float64
	last    float64
	fileCWD string
}

// restoreSession writes one vaulted session back into the store: its messages,
// its session row (scope and provenance from the sidecar, falling back to what
// the transcript itself records), and its file_index watermark.
//
// The watermark is keyed on the ORIGINAL source path, never the vault path.
// That matters: the next live pass compares file_index paths against the source
// walk, so a vault-keyed row would read as "source absent" and stamp
// missing_since on a session that is perfectly alive.
func restoreSession(con *sql.DB, params restoreSessionParams) error {
	v := params.session
	rows := params.rows
	started := params.started
	last := params.last
	fileCWD := params.fileCWD

	tx, err := con.Begin()
	if err != nil {
		return fmt.Errorf("begin restore session %s: %w", v.ID, err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.Exec("DELETE FROM messages WHERE session_id=?", v.ID); err != nil {
		return fmt.Errorf("clear messages for %s: %w", v.ID, err)
	}
	if _, err := tx.Exec("DELETE FROM sessions WHERE id=?", v.ID); err != nil {
		return fmt.Errorf("clear session %s: %w", v.ID, err)
	}
	for _, r := range rows {
		if _, err := tx.Exec(
			"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
			v.ID, r.role, r.content, r.ts, r.tsISO, r.uuid,
		); err != nil {
			return fmt.Errorf("restore messages for %s: %w", v.ID, err)
		}
	}

	var parentArg any
	if v.ParentID != "" {
		parentArg = v.ParentID
	}
	var missingArg any
	if v.MissingSince != 0 {
		missingArg = v.MissingSince
	}
	// The sidecar's scope wins when it has one; a stranded transcript with no
	// sidecar still yields a scope, because the cwd is recorded in the file.
	projectArg, cwdArg := scopeOf(v.CWD, projectScope{})
	if projectArg == nil && cwdArg == nil {
		projectArg, cwdArg = scopeOf(fileCWD, projectScope{})
	}
	source := v.Source
	if source == "" {
		source = sourceClaude
	}
	var sourcePathArg any
	if v.SourcePath != "" {
		sourcePathArg = v.SourcePath
	}
	if _, err := tx.Exec(
		"INSERT OR REPLACE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id,origin_machine,source_tool,source_path,missing_since,project,cwd) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)",
		v.ID, started, last, len(rows), b2i(v.IsSubagent), parentArg, originOr(v.Origin), source, sourcePathArg, missingArg, projectArg, cwdArg,
	); err != nil {
		return fmt.Errorf("restore session %s: %w", v.ID, err)
	}

	if v.SourcePath != "" {
		if _, err := tx.Exec(
			"INSERT OR REPLACE INTO file_index(path,mtime,size,fp,session_id) VALUES(?,?,?,?,?)",
			v.SourcePath, v.SourceMTime, v.SourceSize, v.SourceFP, v.ID,
		); err != nil {
			return fmt.Errorf("restore file_index for %s: %w", v.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit restore session %s: %w", v.ID, err)
	}
	return nil
}
