package index

import (
	"database/sql"
	"fmt"
	"os"

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

// RebuildFromTranscripts rebuilds the index db at dbp from the durable
// transcript vault alone — the guarantee that makes the store a cache: delete
// it, run this, and every session comes back, INCLUDING the ones whose original
// source file no longer exists anywhere on disk.
//
// The db file is removed first rather than reused, so EnsureSchema starts from
// nothing and its rebuild-on-version-mismatch behavior has no rows to drop.
//
// There is deliberately NO retention pass here. Retention reconciles indexed
// sessions against a live source scan; this pass has no scan — its input is the
// vault, whose own sidecars already carry the verdict a previous scan reached.
// Running one here would see every vaulted session's source path and re-derive
// the flags from scratch, which is both redundant and wrong for a session that
// was already retained-and-flagged.
func RebuildFromTranscripts(dbp string) (RebuildStats, error) {
	var st RebuildStats

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

	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(dbp + suffix); err != nil && !os.IsNotExist(err) {
			return st, fmt.Errorf("clear store %s: %w", dbp+suffix, err)
		}
	}

	con, err := store.ConnectRW(dbp)
	if err != nil {
		return st, fmt.Errorf("open store: %w", err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		return st, fmt.Errorf("ensure schema: %w", err)
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
		if err := restoreSession(con, v, rows, started, last, fileCWD); err != nil {
			return st, err
		}
		st.Sessions++
		st.Messages += len(rows)
		if v.MissingSince != 0 {
			st.Missing++
		}
	}
	return st, nil
}

// restoreSession writes one vaulted session back into the store: its messages,
// its session row (scope and provenance from the sidecar, falling back to what
// the transcript itself records), and its file_index watermark.
//
// The watermark is keyed on the ORIGINAL source path, never the vault path.
// That matters: the next live pass compares file_index paths against the source
// walk, so a vault-keyed row would read as "source absent" and stamp
// missing_since on a session that is perfectly alive.
func restoreSession(con *sql.DB, v durable.Session, rows []reindexRow, started, last float64, fileCWD string) error {
	if _, err := con.Exec("DELETE FROM messages WHERE session_id=?", v.ID); err != nil {
		return fmt.Errorf("clear messages for %s: %w", v.ID, err)
	}
	if _, err := con.Exec("DELETE FROM sessions WHERE id=?", v.ID); err != nil {
		return fmt.Errorf("clear session %s: %w", v.ID, err)
	}
	for _, r := range rows {
		if _, err := con.Exec(
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
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id,origin_machine,source_tool,source_path,missing_since,project,cwd) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)",
		v.ID, started, last, len(rows), b2i(v.IsSubagent), parentArg, originOr(v.Origin), source, sourcePathArg, missingArg, projectArg, cwdArg,
	); err != nil {
		return fmt.Errorf("restore session %s: %w", v.ID, err)
	}

	if v.SourcePath == "" {
		return nil // nothing to watermark against
	}
	if _, err := con.Exec(
		"INSERT OR REPLACE INTO file_index(path,mtime,size,fp,session_id) VALUES(?,?,?,?,?)",
		v.SourcePath, v.SourceMTime, v.SourceSize, v.SourceFP, v.ID,
	); err != nil {
		return fmt.Errorf("restore file_index for %s: %w", v.ID, err)
	}
	return nil
}
