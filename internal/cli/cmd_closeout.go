package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	closeoutMaxPasses    = 256
	closeoutSelfTimeout  = 2 * time.Minute
	closeoutChildTimeout = 5 * time.Minute
)

var closeoutTaggerTimeout = 60 * time.Second

var terminateCloseout = terminateCloseoutProcess

var closeoutNow = time.Now

type closeoutTaggerConfig struct {
	Argv []string `json:"argv"`
}

// newCloseoutCmd wires the user-facing closeout entry point. The ordinary path
// only claims a deduplicated detached child; all tagging work happens there.
func newCloseoutCmd() *cobra.Command {
	var child, childToken string
	cmd := &cobra.Command{
		Use:   "closeout <full-session-id>",
		Short: "Queue detached tagging for a completed session",
		Long: "Queue detached session tagging. A configured tagger receives each exact " +
			"`rawclaw tag-prep <full-session-id>` dump on stdin and must print the JSON " +
			"segment array consumed by `rawclaw tag-write <full-session-id>`. Without a " +
			"tagger, run `rawclaw tag-prep <full-session-id>` manually.",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if child {
				return runCloseoutChild(cmd.OutOrStdout(), args[0], childToken)
			}
			return runCloseout(cmd.OutOrStdout(), args[0])
		},
	}
	cmd.Flags().BoolVar(&child, "child", false, "run the detached closeout worker (internal)")
	cmd.Flags().StringVar(&childToken, "token", "", "closeout ownership token (internal)")
	_ = cmd.Flags().MarkHidden("child")
	_ = cmd.Flags().MarkHidden("token")
	return cmd
}

func runCloseout(w io.Writer, sessionID string) error {
	sessionID = strings.TrimSpace(sessionID)
	if len(sessionID) <= uuid8Len {
		return fmt.Errorf("closeout requires the full session id")
	}

	_, exists, err := loadCloseoutTaggerConfig()
	if err != nil {
		// A present but invalid config is a child failure, so it is recorded in
		// the closeout log without delaying this foreground command.
		exists = true
	}
	if !exists {
		_, err := fmt.Fprintf(w, "rawclaw tag-prep %s\n", sessionID)
		return err
	}

	token, ok := acquireCloseoutToken(sessionID)
	if !ok {
		_, err := fmt.Fprintf(w, "closeout already queued for %s\n", sessionID)
		return err
	}
	if err := spawnCloseout(sessionID, token); err != nil {
		releaseCloseoutToken(sessionID, token)
		return err
	}
	_, err = fmt.Fprintf(w, "closeout queued for %s\n", sessionID)
	return err
}

var spawnCloseout = spawnCloseoutChild

func spawnCloseoutChild(sessionID, token string) error {
	exe, err := selfExe()
	if err != nil {
		return fmt.Errorf("resolve rawclaw executable: %w", err)
	}
	logf, err := openIngestLog()
	if err != nil {
		return err
	}
	defer logf.Close()

	cmd := exec.Command(exe, "--timeout", "0", "closeout", "--child", "--token", token, sessionID)
	detach(cmd)
	cmd.Stdin = nil
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start closeout worker: %w", err)
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func closeoutTaggerConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, ".cache", "session-search", "tagger-config.json"), nil
}

func loadCloseoutTaggerConfig() (closeoutTaggerConfig, bool, error) {
	path, err := closeoutTaggerConfigPath()
	if err != nil {
		return closeoutTaggerConfig{}, false, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return closeoutTaggerConfig{}, false, nil
		}
		return closeoutTaggerConfig{}, true, fmt.Errorf("read tagger config: %w", err)
	}
	var cfg closeoutTaggerConfig
	if err := json.Unmarshal(b, &cfg); err != nil {
		return closeoutTaggerConfig{}, true, fmt.Errorf("decode tagger config: %w", err)
	}
	if len(cfg.Argv) == 0 || cfg.Argv[0] == "" || !filepath.IsAbs(cfg.Argv[0]) {
		return closeoutTaggerConfig{}, true, fmt.Errorf("tagger config argv[0] must be an absolute executable path")
	}
	return cfg, true, nil
}

func runCloseoutChild(w io.Writer, sessionID, token string) error {
	if !validateCloseoutToken(sessionID, token) {
		return closeoutFailure(w, fmt.Errorf("closeout worker ownership token is invalid"))
	}
	defer releaseCloseoutToken(sessionID, token)
	ctx, cancel := context.WithTimeout(context.Background(), closeoutChildTimeout)
	defer cancel()
	cfg, exists, err := loadCloseoutTaggerConfig()
	if err != nil {
		return closeoutFailure(w, err)
	}
	if !exists {
		_, writeErr := fmt.Fprintf(w, "rawclaw tag-prep %s\n", sessionID)
		return writeErr
	}

	for range closeoutMaxPasses {
		prep, err := runCloseoutSelfCommand(ctx, "tag-prep", sessionID, nil, w)
		if err != nil {
			return closeoutFailure(w, fmt.Errorf("tag-prep: %w", err))
		}
		if _, err := w.Write(prep); err != nil {
			return err
		}
		if strings.Contains(string(prep), " is already fully tagged\n") {
			return nil
		}

		tags, err := runCloseoutTaggerContext(ctx, cfg.Argv, prep, w)
		if err != nil {
			return closeoutFailure(w, err)
		}
		written, err := runCloseoutSelfCommand(ctx, "tag-write", sessionID, tags, w)
		if err != nil {
			return closeoutFailure(w, fmt.Errorf("tag-write: %w", err))
		}
		if _, err := w.Write(written); err != nil {
			return err
		}
	}
	return closeoutFailure(w, fmt.Errorf("exceeded %d tagging passes", closeoutMaxPasses))
}

func runCloseoutTagger(argv []string, prep []byte, stderr io.Writer) ([]byte, error) {
	return runCloseoutTaggerContext(context.Background(), argv, prep, stderr)
}

func runCloseoutTaggerContext(parent context.Context, argv []string, prep []byte, stderr io.Writer) ([]byte, error) {
	cmd := exec.Command(argv[0], argv[1:]...)
	configureCloseoutProcess(cmd)
	cmd.Stdin = bytes.NewReader(prep)
	stdoutFile, err := os.CreateTemp("", "rawclaw-closeout-stdout-*")
	if err != nil {
		return nil, fmt.Errorf("create tagger stdout capture: %w", err)
	}
	stdoutPath := stdoutFile.Name()
	defer func() { _ = os.Remove(stdoutPath) }()
	defer stdoutFile.Close()
	stderrFile, err := os.CreateTemp("", "rawclaw-closeout-stderr-*")
	if err != nil {
		return nil, fmt.Errorf("create tagger stderr capture: %w", err)
	}
	stderrPath := stderrFile.Name()
	defer func() { _ = os.Remove(stderrPath) }()
	defer stderrFile.Close()
	cmd.Stdout = stdoutFile
	cmd.Stderr = stderrFile
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start tagger: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	ctx, cancel := context.WithTimeout(parent, closeoutTaggerTimeout)
	defer cancel()
	timer := time.NewTimer(closeoutTaggerTimeout)
	defer timer.Stop()
	select {
	case err := <-done:
		if err != nil {
			copyCloseoutStderr(stderrFile, stderr)
			return nil, fmt.Errorf("tagger exited unsuccessfully: %w", err)
		}
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded && parent.Err() != nil {
			return nil, fmt.Errorf("closeout deadline exceeded: %w", parent.Err())
		}
		killErr := terminateCloseout(cmd)
		select {
		case <-done:
		case <-time.After(time.Second):
			copyCloseoutStderr(stderrFile, stderr)
			if killErr == nil {
				killErr = fmt.Errorf("process did not exit after termination")
			}
			return nil, fmt.Errorf("tagger timed out after %s; process cleanup: %w", closeoutTaggerTimeout, killErr)
		}
		copyCloseoutStderr(stderrFile, stderr)
		if killErr != nil {
			return nil, fmt.Errorf("tagger timed out after %s; process cleanup: %w", closeoutTaggerTimeout, killErr)
		}
		return nil, fmt.Errorf("tagger timed out after %s", closeoutTaggerTimeout)
	}
	if err := stdoutFile.Close(); err != nil {
		return nil, fmt.Errorf("close tagger stdout capture: %w", err)
	}
	copyCloseoutStderr(stderrFile, stderr)
	stdout, err := os.ReadFile(stdoutPath)
	if err != nil {
		return nil, fmt.Errorf("read tagger output: %w", err)
	}
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("tagger returned empty output")
	}
	var segments []rawSegment
	if err := json.Unmarshal(trimmed, &segments); err != nil {
		return nil, fmt.Errorf("tagger returned malformed JSON: %w", err)
	}
	if len(segments) == 0 {
		return nil, fmt.Errorf("tagger returned an empty segment array")
	}
	return stdout, nil
}

func copyCloseoutStderr(file *os.File, dst io.Writer) {
	if dst == nil {
		return
	}
	_, _ = file.Seek(0, io.SeekStart)
	_, _ = io.Copy(dst, file)
}

func runCloseoutSelfCommand(ctx context.Context, name, sessionID string, input []byte, stderr io.Writer) ([]byte, error) {
	exe, err := selfExe()
	if err != nil {
		return nil, fmt.Errorf("resolve rawclaw executable: %w", err)
	}
	args := []string{"--timeout", closeoutSelfTimeout.String(), name, sessionID}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Stdin = bytes.NewReader(input)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

func closeoutFailure(w io.Writer, err error) error {
	_, _ = fmt.Fprintf(w, "closeout: failed: %v\n", err)
	return err
}
