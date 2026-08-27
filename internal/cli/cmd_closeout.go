package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const closeoutTaggerEnv = "RAWCLAW_TAGGER"

// newCloseoutCmd wires the user-facing closeout entry point. The normal path
// only claims a deduplicated detached child; all tagging work happens there.
func newCloseoutCmd() *cobra.Command {
	var child bool
	cmd := &cobra.Command{
		Use:   "closeout <full-session-id>",
		Short: "Queue bounded detached tagging for a completed session",
		Long: "Queue detached session tagging. If RAWCLAW_TAGGER is configured, it is " +
			"given each bounded `tag-prep` dump and must write the JSON consumed by `tag-write`. " +
			"Without a tagger, the child logs the exact manual recovery command printed by `tag-prep`.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if child {
				return runCloseoutChild(cmd.OutOrStdout(), args[0])
			}
			return runCloseout(cmd.OutOrStdout(), args[0])
		},
	}
	cmd.Flags().BoolVar(&child, "child", false, "run the detached worker (internal)")
	_ = cmd.Flags().MarkHidden("child")
	return cmd
}

func runCloseout(w io.Writer, sessionID string) error {
	if !acquireIngestSpawnToken("closeout-"+sessionID, nowTime()) {
		fmt.Fprintf(w, "closeout already queued for %s\n", sessionID)
		return nil
	}
	if err := spawnCloseoutChild(sessionID); err != nil {
		return err
	}
	fmt.Fprintf(w, "closeout queued for %s\n", sessionID)
	return nil
}

var nowTime = time.Now

func spawnCloseoutChild(sessionID string) error {
	exe, err := selfExe()
	if err != nil {
		return fmt.Errorf("resolve rawclaw executable: %w", err)
	}
	logf, err := openIngestLog()
	if err != nil {
		return err
	}
	defer logf.Close()
	cmd := exec.Command(exe, "closeout", "--child", sessionID)
	detach(cmd)
	cmd.Stdin = nil
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start closeout worker: %w", err)
	}
	_ = cmd.Process.Release()
	return nil
}

func runCloseoutChild(w io.Writer, sessionID string) error {
	tagger := strings.TrimSpace(os.Getenv(closeoutTaggerEnv))
	for pass := 0; ; pass++ {
		if pass >= 256 {
			return fmt.Errorf("closeout exceeded 256 tagging passes")
		}
		prep, err := runSelfCommand("tag-prep", sessionID)
		if err != nil {
			return fmt.Errorf("closeout tag-prep: %w", err)
		}
		if _, err := io.WriteString(w, string(prep)); err != nil {
			return err
		}
		if strings.Contains(string(prep), "is already fully tagged") {
			return nil
		}
		if tagger == "" {
			// prep contains the canonical manual tag-write and rerun commands.
			return nil
		}

		tags, err := runConfiguredTagger(tagger, prep)
		if err != nil {
			return fmt.Errorf("closeout tagger: %w", err)
		}
		written, err := runSelfCommandWithInput(tags, "tag-write", sessionID)
		if err != nil {
			return fmt.Errorf("closeout tag-write: %w", err)
		}
		if _, err := w.Write(written); err != nil {
			return err
		}
	}
}

func runConfiguredTagger(command string, prep []byte) ([]byte, error) {
	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Stdin = bytes.NewReader(prep)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errOut.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return out.Bytes(), nil
}

func runSelfCommand(name, sessionID string) ([]byte, error) {
	return runSelfCommandWithInput(nil, name, sessionID)
}

func runSelfCommandWithInput(input []byte, name, sessionID string) ([]byte, error) {
	exe, err := selfExe()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, name, sessionID)
	cmd.Stdin = bytes.NewReader(input)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errOut.String()); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return out.Bytes(), nil
}
