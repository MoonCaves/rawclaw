package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
	"github.com/spf13/cobra"
)

// tagPublishChildTimeoutArg bounds the detached derived-store publication.
const tagPublishChildTimeoutArg = "25s"

var tagPublishLogPath = func() string {
	return filepath.Join(filepath.Dir(index.ConsolidatedPath()), "tag-publish.log")
}

var spawnTagPublish = spawnTagPublishChild

func newTagPublishCmd() *cobra.Command {
	var dbp string
	cmd := &cobra.Command{
		Use:           "tag-publish",
		Short:         "Publish one per-project tag database to the consolidated store (detached)",
		Hidden:        true,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTagPublishChild(cmd.Context(), cmd.OutOrStdout(), dbp)
		},
	}
	cmd.Flags().StringVar(&dbp, "dbp", "", "per-project database path to publish")
	return cmd
}

func spawnTagPublishChild(dbp string) error {
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
	cmd := exec.Command(exe, "tag-publish", "--dbp", dbp, "--timeout", tagPublishChildTimeoutArg)
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

func runTagPublishChild(ctx context.Context, w io.Writer, dbp string) error {
	if isConsolidatedSource(dbp) {
		tagPublishLogLine(w, "tag-publish: skipped invalid/self source %q", dbp)
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := index.SyncConsolidatedFrom(dbp); err != nil {
		tagPublishLogLine(w, "tag-publish: %s: %v", dbp, err)
		return err
	}
	tagPublishLogLine(w, "tag-publish: published %s", dbp)
	return nil
}

// isConsolidatedSource rejects alternate spellings and symlinks to the derived
// store; publishing it into itself could otherwise attach the live destination.
func isConsolidatedSource(dbp string) bool {
	if dbp == "" {
		return true
	}
	dst := index.ConsolidatedPath()
	resolvedDB, dbErr := filepath.EvalSymlinks(dbp)
	resolvedDst, dstErr := filepath.EvalSymlinks(dst)
	if dbErr == nil && dstErr == nil {
		return filepath.Clean(resolvedDB) == filepath.Clean(resolvedDst)
	}
	return filepath.Clean(dbp) == filepath.Clean(dst)
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
