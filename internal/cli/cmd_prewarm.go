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
	cmd := &cobra.Command{
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
	return cmd
}

func runPrewarmCmd(
	w io.Writer,
	sessionArg string,
	scope []view.Scope,
	more agentproto.ScopeFn,
	registrations []source.Registration,
) error {
	// An explicitly empty scope makes LocateSession use only its consolidated
	// store probe; it cannot fall through to the per-project sweep.
	_, _, locateErr := locatePrewarmStore(sessionArg)
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
	segs := readConsolidatedTopics(fullSID)
	var dump struct {
		Fingerprint prewarmFingerprint
		Content     string
	}
	var content bytes.Buffer
	if err := runTagPrepWithTopics(&content, con, fullSID, segs); err != nil {
		_ = con.Close()
		return err
	}
	_ = con.Close()
	dump.Content = content.String()
	if sourcePath != "" {
		st, err := os.Stat(sourcePath)
		if err != nil {
			return fmt.Errorf("stat transcript %s: %w", sourcePath, err)
		}
		dump.Fingerprint = prewarmFingerprint{MTime: st.ModTime().UnixNano(), Size: st.Size()}
	}
	if err := durable.WriteAtomic(dumpPath, []byte(dump.Content)); err != nil {
		return fmt.Errorf("write prewarm dump: %w", err)
	}
	state, err := json.Marshal(dump.Fingerprint)
	if err != nil {
		return err
	}
	if err := durable.WriteAtomic(dumpPath+".state", state); err != nil {
		return fmt.Errorf("write prewarm fingerprint: %w", err)
	}
	_, _ = io.WriteString(w, dumpPath+"\n")
	return nil
}

func locatePrewarmStore(sessionArg string) (string, string, error) {
	return agentproto.LocateConsolidatedSession(sessionArg)
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
