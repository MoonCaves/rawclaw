package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/sources"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/gofrs/flock"
	"github.com/spf13/cobra"
)

// IngestResult describes the outcome of ingesting one session.
type IngestResult struct {
	SessionID string `json:"session_id"`
	Messages  int    `json:"messages"`
	Source    string `json:"source,omitempty"`
}

// newIngestCmd wires `rawclaw ingest [session]`: ingest one session (by full id
// or prefix) or all discoverable active sessions into the consolidated search store.
//
// Background SessionStart hooks kick this command in the background at session
// birth so that subsequent reads and searches are fast, pure queries against
// an already-indexed store.
func newIngestCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "ingest [session]",
		Short: "Ingest a session transcript into the search index",
		Long: "Ingest one session (by full id or prefix) or all discoverable active sessions " +
			"into the consolidated search store.\n\n" +
			"This is triggered automatically in the background at session start by editor hooks " +
			"so that subsequent reads are fast index queries. It is safe to run concurrently and " +
			"repeatedly: unchanged sessions are skipped via watermarks, and concurrent runs serialize " +
			"safely through SQLite WAL mode and bounded retry.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sessionArg := ""
			if len(args) == 1 {
				sessionArg = strings.TrimSpace(args[0])
			}
			return runIngest(cmd.OutOrStdout(), sessionArg, jsonOut)
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output")
	return cmd
}

// runIngest resolves the target session(s) and refreshes their index rows into
// the consolidated store.
func runIngest(w io.Writer, sessionArg string, jsonOut bool) error {
	regs := sources.Registered()
	matches, err := resolveIngestMatches(sessionArg, regs)
	if err != nil && len(matches) == 0 {
		if jsonOut {
			return EmitJSON(w, struct {
				Ingested []IngestResult `json:"ingested"`
				Error    string         `json:"error,omitempty"`
			}{Ingested: []IngestResult{}, Error: err.Error()})
		}
		return err
	}

	var results []IngestResult
	var errs []string
	if err != nil {
		errs = append(errs, err.Error())
	}

	for _, match := range matches {
		rawPath := backingPath(match.container.Path)
		if _, err := os.Stat(rawPath); err != nil {
			if os.IsNotExist(err) {
				// Transcript file not yet on disk (newly born session before first write); skip cleanly.
				continue
			}
			logIngestError(match.container.ID, fmt.Errorf("stat live transcript %s: %w", rawPath, err))
			errs = append(errs, fmt.Sprintf("%s: %v", match.container.ID, err))
			continue
		}

		nMessages, err := ingestContainerWithRetry(match)
		if err != nil {
			logIngestError(match.container.ID, err)
			errs = append(errs, fmt.Sprintf("%s: %v", match.container.ID, err))
			continue
		}

		results = append(results, IngestResult{
			SessionID: match.container.ID,
			Messages:  nMessages,
			Source:    match.registration.ID,
		})
	}

	if jsonOut {
		errMsg := ""
		if len(errs) > 0 {
			errMsg = strings.Join(errs, "; ")
		}
		return EmitJSON(w, struct {
			Ingested []IngestResult `json:"ingested"`
			Error    string         `json:"error,omitempty"`
		}{
			Ingested: results,
			Error:    errMsg,
		})
	}

	if len(results) == 0 {
		if len(errs) > 0 {
			return fmt.Errorf("ingest failed: %s", strings.Join(errs, "; "))
		}
		if sessionArg != "" {
			fmt.Fprintf(w, "No active session matching %q found to ingest.\n", sessionArg)
		} else {
			fmt.Fprintln(w, "No sessions to ingest.")
		}
		return nil
	}

	if len(results) == 1 {
		if len(errs) > 0 {
			fmt.Fprintf(w, "Ingested session %s (%d messages) with errors: %s\n", trunc8(results[0].SessionID), results[0].Messages, strings.Join(errs, "; "))
			return fmt.Errorf("ingest partially failed: %s", strings.Join(errs, "; "))
		}
		fmt.Fprintf(w, "Ingested session %s (%d messages)\n", trunc8(results[0].SessionID), results[0].Messages)
		return nil
	}

	if len(errs) > 0 {
		fmt.Fprintf(w, "Ingested %d session(s) with errors: %s\n", len(results), strings.Join(errs, "; "))
		return fmt.Errorf("ingest partially failed: %s", strings.Join(errs, "; "))
	}
	fmt.Fprintf(w, "Ingested %d session(s)\n", len(results))
	return nil
}

// resolveIngestMatches finds the tagSourceMatch candidates for the given sessionArg.
func resolveIngestMatches(sessionArg string, regs []source.Registration) ([]tagSourceMatch, error) {
	if sessionArg == "" {
		return discoverAllIngestSources(regs)
	}

	prefix := agentproto.NormalizeSessionArg(sessionArg)

	// 1. Check the durable session catalog first.
	if catalogMatch, ok := catalogIngestSource(prefix, regs); ok {
		return []tagSourceMatch{catalogMatch}, nil
	}

	// 2. Check stem lookup (e.g. paths.ResolveSession for Claude).
	if stems := stemTagSources(prefix, regs); len(stems) > 0 {
		return stems, nil
	}

	// 3. Check registered source adapters discovery.
	if discovered, err := discoverTagSources(prefix, regs); len(discovered) > 0 {
		return discovered, err
	} else if err != nil {
		return nil, err
	}

	// 4. Check already located backing in the consolidated store.
	if located, ok := locatedTagSource(index.ConsolidatedPath(), prefix, regs); ok {
		return []tagSourceMatch{located}, nil
	}

	return nil, nil
}

// catalogIngestSource looks up a session in the session catalog directory.
func catalogIngestSource(sessionID string, regs []source.Registration) (tagSourceMatch, bool) {
	catalogDir := paths.CatalogDir()
	entryPath := filepath.Join(catalogDir, sessionID)
	entry, err := paths.ReadCatalogEntry(entryPath)
	if err != nil || entry.SessionID == "" || entry.TranscriptPath == "" {
		return tagSourceMatch{}, false
	}
	reg, ok := registrationFor(entry.Source, entry.TranscriptPath, regs)
	if !ok || reg.New == nil {
		return tagSourceMatch{}, false
	}
	adapter := reg.New()
	if adapter == nil {
		return tagSourceMatch{}, false
	}
	return tagSourceMatch{
		registration: reg,
		adapter:      adapter,
		container: source.Container{
			ID:   entry.SessionID,
			Path: entry.TranscriptPath,
			CWD:  entry.CWD,
		},
	}, true
}

// discoverAllIngestSources discovers all containers across all registered adapters.
func discoverAllIngestSources(regs []source.Registration) ([]tagSourceMatch, error) {
	var matches []tagSourceMatch
	var errs []error
	for _, reg := range regs {
		if reg.New == nil {
			continue
		}
		adapter := reg.New()
		if adapter == nil {
			continue
		}
		containers, err := adapter.Discover()
		if err != nil {
			errs = append(errs, fmt.Errorf("discover %s sessions: %w", reg.ID, err))
			continue
		}
		for _, c := range containers {
			matches = append(matches, tagSourceMatch{
				registration: reg,
				adapter:      adapter,
				container:    c,
			})
		}
	}
	return matches, errors.Join(errs...)
}

// Contention safety choice:
// We use a single-flight exclusive file lock (flock) keyed to the consolidated
// store (consolidated.lock) with a 10s timeout, combined with SQLite WAL mode
// and bounded retry with backoff and jitter.
// This guarantees that N concurrent hooks collapse safely into serialized runs,
// preventing store creation races, corruptions, and SQLite write contention.
func ingestContainerWithRetry(match tagSourceMatch) (int, error) {
	lockPath := filepath.Join(store.CacheDir(), "consolidated.lock")
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return 0, fmt.Errorf("create store lock dir: %w", err)
	}

	fl := flock.New(lockPath)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	locked, err := fl.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		return 0, fmt.Errorf("acquire consolidated store lock for %s: %w", match.container.ID, err)
	}
	if !locked {
		return 0, fmt.Errorf("timed out waiting for consolidated store lock for %s", match.container.ID)
	}
	defer func() { _ = fl.Unlock() }()

	dbp := index.RefreshDBPath(
		match.registration.ID,
		match.container.ID,
		match.container.Path,
	)

	const maxAttempts = 5
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		nMessages, err := index.EnsureFreshContainer(
			dbp,
			match.container,
			match.adapter.Messages,
			match.registration.ID,
		)
		if err == nil {
			return nMessages, nil
		}
		lastErr = err

		// Exponential backoff: 20ms, 40ms, 80ms, 160ms, 320ms + jitter up to 20ms
		backoff := time.Duration(20*(1<<attempt))*time.Millisecond + time.Duration(rand.Intn(20))*time.Millisecond
		time.Sleep(backoff)
	}

	return 0, fmt.Errorf("refresh session %s from %s after %d attempts: %w",
		match.container.ID, match.registration.ID, maxAttempts, lastErr)
}

// logIngestError appends an ingest failure trace to the cache ingest.log so failures
// during detached background executions remain diagnosable without polluting agent stdio.
func logIngestError(sessionID string, err error) {
	logDir := store.CacheDir()
	_ = os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, "ingest.log")
	f, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if openErr != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(f, "%s [error] session=%s: %v\n", ts, sessionID, err)
}

func backingPath(p string) string {
	if idx := strings.IndexByte(p, '#'); idx >= 0 {
		return p[:idx]
	}
	return p
}
