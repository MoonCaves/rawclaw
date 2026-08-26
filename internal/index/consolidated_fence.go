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
	lock       *flock.Flock
	acquiredAt time.Time
}

func AcquireConsolidatedFence(ctx context.Context) (*ConsolidatedFence, error) {
	started := time.Now()
	slog.Info("consolidated fence phase", "phase", "acquire", "event", "start")
	lockPath := filepath.Join(store.CacheDir(), "consolidated.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create consolidated lock directory: %w", err)
	}

	lock := flock.New(lockPath)
	waitCtx, cancel := context.WithTimeout(ctx, consolidatedLockWait)
	defer cancel()
	reportedHolder := false
	// One timer, reused with Reset across retries — the lock is polled every
	// consolidatedLockRetry for up to consolidatedLockWait (up to ~1200 ticks
	// on a busy store), and this is a hot path: nearly every ingest hook
	// acquires this fence. Reset is only ever called after the channel has
	// already delivered (the timer.C case below), which is the one case
	// that's safe to reuse without a drain dance on any Go version.
	timer := time.NewTimer(consolidatedLockRetry)
	defer timer.Stop()
	for {
		locked, err := lock.TryLock()
		if err != nil {
			return nil, fmt.Errorf("acquire consolidated lock: %w", err)
		}
		if locked {
			slog.Info("consolidated fence phase", "phase", "acquire", "duration", time.Since(started))
			return &ConsolidatedFence{lock: lock, acquiredAt: time.Now()}, nil
		}
		if !reportedHolder && time.Since(started) >= consolidatedLockWaitThreshold {
			logConsolidatedLockHolder()
			reportedHolder = true
		}
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("wait for consolidated lock: %w", waitCtx.Err())
		case <-timer.C:
			timer.Reset(consolidatedLockRetry)
		}
	}
}

func (f *ConsolidatedFence) Close() error {
	if f == nil || f.lock == nil {
		return nil
	}
	started := time.Now()
	slog.Info("consolidated fence phase", "phase", "release", "event", "start")
	err := f.lock.Unlock()
	slog.Info("consolidated fence phase", "phase", "release", "duration", time.Since(started), "held", time.Since(f.acquiredAt))
	return err
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
