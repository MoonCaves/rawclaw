package index

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/gofrs/flock"
)

const (
	consolidatedLockRetry = 25 * time.Millisecond
	consolidatedLockWait  = 30 * time.Second
)

var consolidatedLockWaitThreshold = 2 * time.Second

var runConsolidatedLsof = func(paths []string) ([]byte, error) {
	args := []string{"-nP", "-F", "c", "--"}
	args = append(args, paths...)
	return exec.Command("lsof", args...).CombinedOutput()
}

type ConsolidatedFence struct {
	lock *flock.Flock
}

func AcquireConsolidatedFence(ctx context.Context) (*ConsolidatedFence, error) {
	lockPath := filepath.Join(store.CacheDir(), "consolidated.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create consolidated lock directory: %w", err)
	}

	lock := flock.New(lockPath)
	waitCtx, cancel := context.WithTimeout(ctx, consolidatedLockWait)
	defer cancel()
	started := time.Now()
	reportedHolder := false
	for {
		locked, err := lock.TryLock()
		if err != nil {
			return nil, fmt.Errorf("acquire consolidated lock: %w", err)
		}
		if locked {
			return &ConsolidatedFence{lock: lock}, nil
		}
		if !reportedHolder && time.Since(started) >= consolidatedLockWaitThreshold {
			logConsolidatedLockHolder()
			reportedHolder = true
		}
		timer := time.NewTimer(consolidatedLockRetry)
		select {
		case <-waitCtx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, fmt.Errorf("wait for consolidated lock: %w", waitCtx.Err())
		case <-timer.C:
		}
	}
}

func (f *ConsolidatedFence) Close() error {
	if f == nil || f.lock == nil {
		return nil
	}
	return f.lock.Unlock()
}

func logConsolidatedLockHolder() {
	paths, err := filepath.Glob(ConsolidatedPath() + "*")
	if err != nil {
		slog.Warn("consolidated lock wait", "holder_processes", "glob failed", "err", err)
		return
	}
	if len(paths) == 0 {
		slog.Warn("consolidated lock wait", "holder_processes", "none")
		return
	}
	out, err := runConsolidatedLsof(paths)
	if err != nil {
		slog.Warn("consolidated lock wait", "holder_processes", lsofCommands(out), "lsof_err", err)
		return
	}
	slog.Warn("consolidated lock wait", "holder_processes", lsofCommands(out))
}

func lsofCommands(out []byte) string {
	var commands []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if strings.HasPrefix(line, "c") && len(line) > 1 {
			commands = append(commands, strings.TrimSpace(line[1:]))
		}
	}
	if len(commands) == 0 {
		return "unknown"
	}
	return strings.Join(commands, ",")
}
