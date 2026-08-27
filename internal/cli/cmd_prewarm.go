package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/durable"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/sources"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
	"github.com/spf13/cobra"
)

type prewarmFingerprint struct {
	MTime int64 `json:"mtime"`
	Size  int64 `json:"size"`
}

// newPrewarmCmd wires the internal closeout prewarm command.
func newPrewarmCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "prewarm <session8>",
		Short:         "Prepare a session closeout dump",
		Hidden:        true,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPrewarmCmd(cmd.OutOrStdout(), args[0], nil, nil, sources.Registered())
		},
	}
}

func runPrewarmCmd(
	w io.Writer,
	sessionArg string,
	scope []view.Scope,
	more agentproto.ScopeFn,
	registrations []source.Registration,
) error {
	// Only a one-store miss requires folding the targeted refresh result.
	_, _, locateErr := agentproto.LocateConsolidatedSession(sessionArg)
	dbPath, fullSID, toFold, err := refreshTagSession(sessionArg, scope, more, registrations)
	if err != nil {
		return err
	}
	if locateErr != nil {
		for _, refreshDB := range toFold {
			if err := index.SyncConsolidatedFrom(refreshDB); err != nil {
				return fmt.Errorf("fold refreshed session %s: %w", fullSID, err)
			}
		}
	}

	sourcePath := prewarmSourcePath(dbPath, fullSID)
	dumpPath := index.PrewarmDumpPath(fullSID)
	if prewarmFresh(dumpPath, sourcePath) {
		return nil
	}

	con, err := store.ConnectRO(dbPath)
	if err != nil {
		return fmt.Errorf("open %q read-only: %w", dbPath, err)
	}
	defer con.Close()

	var content bytes.Buffer
	if err := runTagPrepWithTopics(&content, con, fullSID, readConsolidatedTopics(fullSID)); err != nil {
		return err
	}

	var fp prewarmFingerprint
	if sourcePath != "" {
		st, err := os.Stat(sourcePath)
		if err != nil {
			return fmt.Errorf("stat transcript %s: %w", sourcePath, err)
		}
		fp = prewarmFingerprint{MTime: st.ModTime().UnixNano(), Size: st.Size()}
	}
	if err := durable.WriteAtomic(dumpPath, content.Bytes()); err != nil {
		return fmt.Errorf("write prewarm dump: %w", err)
	}
	state, err := json.Marshal(fp)
	if err != nil {
		return err
	}
	if err := durable.WriteAtomic(dumpPath+".state", state); err != nil {
		return fmt.Errorf("write prewarm fingerprint: %w", err)
	}
	_, _ = io.WriteString(w, dumpPath+"\n")
	return nil
}

func prewarmSourcePath(dbPath, sessionID string) string {
	con, err := store.ConnectRO(dbPath)
	if err != nil {
		return ""
	}
	defer con.Close()
	var path string
	if err := con.QueryRow("SELECT source_path FROM session_sources WHERE session_id=? LIMIT 1", sessionID).Scan(&path); err == nil {
		return path
	}
	if err := con.QueryRow("SELECT path FROM file_index WHERE session_id=? LIMIT 1", sessionID).Scan(&path); err == nil {
		return path
	}
	return ""
}

func prewarmFresh(dumpPath, sourcePath string) bool {
	if _, err := os.Stat(dumpPath); err != nil {
		return false
	}
	if sourcePath == "" {
		return true
	}
	st, err := os.Stat(sourcePath)
	if err != nil {
		return false
	}
	stateBytes, err := os.ReadFile(dumpPath + ".state")
	if err != nil {
		return false
	}
	var saved prewarmFingerprint
	if json.Unmarshal(stateBytes, &saved) != nil {
		return false
	}
	return saved.Size == st.Size() && saved.MTime == st.ModTime().UnixNano()
}
