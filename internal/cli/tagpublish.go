package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
	"github.com/spf13/cobra"
)

// tagPublishChildTimeoutArg bounds the detached derived-store publication.
const tagPublishChildTimeoutArg = "25s"

var tagPublishLogPath = func() string {
	return filepath.Join(filepath.Dir(index.ConsolidatedPath()), "tag-publish.log")
}

var tagPublishSessionID string
var tagPublishMu sync.Mutex
var spawnTagPublish = func(dbp string) error { return spawnTagPublishChild(dbp, tagPublishSessionID) }

func newTagPublishCmd() *cobra.Command {
	var dbp, sessionID string
	cmd := &cobra.Command{
		Use:           "tag-publish",
		Short:         "Publish one per-project tag database to the consolidated store (detached)",
		Hidden:        true,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagPublishChild(cmd.Context(), cmd.OutOrStdout(), dbp, sessionID)
		},
	}
	cmd.Flags().StringVar(&dbp, "dbp", "", "per-project database path to publish")
	cmd.Flags().StringVar(&sessionID, "session", "", "full session id to publish")
	return cmd
}

func spawnTagPublishChild(dbp string, sessionID ...string) error {
	if isConsolidatedSource(dbp) {
		return nil
	}
	exe, err := selfExe()
	if err != nil {
		return fmt.Errorf("resolve rawclaw executable: %w", err)
	}
	logf, err := openTagPublishLog()
	if err != nil {
		return err
	}
	defer logf.Close()
	args := []string{"tag-publish", "--dbp", dbp, "--session", ""}
	if len(sessionID) > 0 {
		args[4] = sessionID[0]
	}
	args = append(args, "--timeout", tagPublishChildTimeoutArg)
	cmd := exec.Command(exe, args...)
	detach(cmd)
	cmd.Stdin = nil
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start tag publisher: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

func runTagPublishChild(ctx context.Context, w io.Writer, dbp string, sessionIDs ...string) error {
	if isConsolidatedSource(dbp) {
		tagPublishLogLine(w, "tag-publish: skipped invalid/self source %q", dbp)
		return nil
	}
	if len(sessionIDs) == 0 || sessionIDs[0] == "" {
		return fmt.Errorf("tag-publish: missing session id")
	}
	sid := sessionIDs[0]
	if err := ctx.Err(); err != nil {
		return err
	}
	src, err := store.ConnectRO(dbp)
	if err != nil {
		return err
	}
	defer src.Close()
	segments, err := store.TopicsForSession(src, sid)
	if err != nil {
		return err
	}
	verdict, hasVerdict, err := store.VerdictFor(src, sid)
	if err != nil {
		return err
	}
	fence, err := index.AcquireConsolidatedFence(ctx)
	if err != nil {
		return fmt.Errorf("acquire consolidated store lock: %w", err)
	}
	defer fence.Close()
	dst, err := store.ConnectRW(index.ConsolidatedPath())
	if err != nil {
		return err
	}
	defer dst.Close()
	if err := store.EnsureTopicSchema(dst); err != nil {
		return err
	}
	if err := publishSession(ctx, dst, sid, segments, verdict, hasVerdict); err != nil {
		tagPublishLogLine(w, "tag-publish: %s: %v", dbp, err)
		return err
	}
	tagPublishLogLine(w, "tag-publish: published %s", dbp)
	return nil
}

func isConsolidatedSource(dbp string) bool {
	if dbp == "" {
		return true
	}
	db, de := filepath.EvalSymlinks(dbp)
	dst, te := filepath.EvalSymlinks(index.ConsolidatedPath())
	if de == nil && te == nil {
		if filepath.Clean(db) == filepath.Clean(dst) {
			return true
		}
		if a, err := os.Stat(db); err == nil {
			if b, err := os.Stat(dst); err == nil && os.SameFile(a, b) {
				return true
			}
		}
	}
	return filepath.Clean(dbp) == filepath.Clean(index.ConsolidatedPath())
}

func publishSession(ctx context.Context, con *sql.DB, sid string, segments []store.TopicSegment, verdict store.Verdict, hasVerdict bool) error {
	sourceOrigin := ""
	if len(segments) > 0 {
		sourceOrigin = segments[0].OriginMachine
	} else if hasVerdict {
		sourceOrigin = verdict.OriginMachine
	}
	var sourceRevision float64
	for _, s := range segments {
		if s.TaggedAt > sourceRevision {
			sourceRevision = s.TaggedAt
		}
	}
	if hasVerdict && verdict.TaggedAt > sourceRevision {
		sourceRevision = verdict.TaggedAt
	}
	tx, err := con.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var existingOrigin string
	if err := tx.QueryRowContext(ctx, "SELECT COALESCE(MAX(origin_machine),'') FROM topic_segment WHERE session_id=?", sid).Scan(&existingOrigin); err != nil {
		return err
	}
	if existingOrigin > sourceOrigin {
		return tx.Commit()
	}
	var currentRevision sql.NullFloat64
	revisionQuery := "SELECT MAX(tagged_at) FROM topic_segment WHERE session_id=? AND (origin_machine IS NULL OR origin_machine='')"
	args := []any{sid}
	if sourceOrigin != "" {
		revisionQuery = "SELECT MAX(tagged_at) FROM topic_segment WHERE session_id=? AND origin_machine=?"
		args = append(args, sourceOrigin)
	}
	if err := tx.QueryRowContext(ctx, revisionQuery, args...).Scan(&currentRevision); err != nil {
		return err
	}
	if !currentRevision.Valid || currentRevision.Float64 < sourceRevision {
		where := "session_id=? AND (origin_machine IS NULL OR origin_machine='')"
		if sourceOrigin != "" {
			where = "session_id=?"
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM topic_segment WHERE "+where, sid); err != nil {
			return err
		}
		for _, s := range segments {
			if _, err := tx.ExecContext(ctx, `INSERT INTO topic_segment(session_id,start_uuid,end_uuid,topic,summary,tagged_at,origin_machine) VALUES(?,?,?,?,?,?,NULLIF(?,''))`, sid, s.StartUUID, s.EndUUID, s.Topic, s.Summary, s.TaggedAt, s.OriginMachine); err != nil {
				return err
			}
		}
	}
	if hasVerdict {
		var existing store.Verdict
		var ok bool
		var src string
		var at sql.NullFloat64
		err = tx.QueryRowContext(ctx, "SELECT source, tagged_at, origin_machine FROM session_verdict WHERE session_id=?", sid).Scan(&src, &at, &existing.OriginMachine)
		if err == sql.ErrNoRows {
			err = nil
		} else if err == nil {
			existing.Source, existing.TaggedAt, ok = src, at.Float64, true
		}
		if err != nil {
			return err
		}
		if !ok || (existing.OriginMachine <= verdict.OriginMachine && verdict.TaggedAt > existing.TaggedAt) {
			_, err = tx.ExecContext(ctx, `INSERT INTO session_verdict(session_id,verdict,source,origin_machine,tagged_at) VALUES(?,?,?,?,?) ON CONFLICT(session_id) DO UPDATE SET verdict=excluded.verdict,source=excluded.source,origin_machine=excluded.origin_machine,tagged_at=excluded.tagged_at`, sid, verdict.Verdict, verdict.Source, verdict.OriginMachine, verdict.TaggedAt)
		}
	} else {
		_, err = tx.ExecContext(ctx, "DELETE FROM session_verdict WHERE session_id=? AND (origin_machine IS NULL OR origin_machine='') AND tagged_at < ?", sid, sourceRevision)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

func openTagPublishLog() (*os.File, error) {
	p := tagPublishLogPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", filepath.Dir(p), err)
	}
	return os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
}

func tagPublishLogLine(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, timefmt.UTC(time.Now())+" "+format+"\n", args...)
}
