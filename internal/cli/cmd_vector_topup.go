package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/MoonCaves/rawclaw/internal/adapters"
	"github.com/MoonCaves/rawclaw/internal/semantic"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
	"github.com/spf13/cobra"
)

// newVectorTopupCmd wires the hidden `rawclaw vector-topup` verb: the body of
// the detached background top-up child that ordinary invokes spawn.
func newVectorTopupCmd() *cobra.Command {
	var dbp string
	var maxNew int
	cmd := &cobra.Command{
		Use:           "vector-topup",
		Short:         "Top-up vectors for a scope (detached background child)",
		Hidden:        true,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVectorTopupChild(cmd.Context(), cmd.OutOrStdout(), dbp, maxNew)
		},
	}
	cmd.Flags().StringVar(&dbp, "dbp", "", "database path to top up")
	cmd.Flags().IntVar(&maxNew, "max-new", semantic.DefaultTopupMaxNew, "max new vectors to embed")
	return cmd
}

// runVectorTopupChild executes the background top-up pass for a single store.
func runVectorTopupChild(ctx context.Context, w io.Writer, dbp string, maxNew int) error {
	if dbp == "" {
		return nil
	}
	emb := adapters.GetEmbedder()
	if emb == nil {
		vectorTopupLogLine(w, "vector-topup: no embedder configured; exiting")
		return nil
	}

	release, ok := semantic.TryAcquireTopupLock(dbp)
	if !ok {
		vectorTopupLogLine(w, "vector-topup: skipped (busy lock for %s)", dbp)
		return nil
	}
	defer release()

	con, err := store.ConnectRW(dbp)
	if err != nil {
		vectorTopupLogLine(w, "vector-topup: connect %s: %v", dbp, err)
		return err
	}
	defer con.Close()

	added, err := semantic.VecIndex(ctx, con, emb, maxNew)
	if err != nil {
		vectorTopupLogLine(w, "vector-topup: %s: %v", dbp, err)
		return err
	}
	vectorTopupLogLine(w, "vector-topup: %s +%d vector(s)", dbp, added)
	return nil
}

// vectorTopupLogLine writes one timestamped receipt line to the top-up log.
func vectorTopupLogLine(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, timefmt.UTC(time.Now())+" "+format+"\n", args...)
}
