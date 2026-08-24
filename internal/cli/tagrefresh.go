package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/sources"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// runTagPrepCmd refreshes the requested live session through the registered
// source adapters, then prints the consolidated current view.
func runTagPrepCmd(w io.Writer, session8 string, scope []view.Scope, more agentproto.ScopeFn) error {
	return runTagPrepCmdWithSources(w, session8, scope, more, sources.Registered())
}

func runTagPrepCmdWithSources(
	w io.Writer,
	session8 string,
	scope []view.Scope,
	more agentproto.ScopeFn,
	registrations []source.Registration,
) error {
	dbp, fullSID, err := refreshTagSession(session8, scope, more, registrations)
	if err != nil {
		return err
	}
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return fmt.Errorf("open %q read-only: %w", dbp, err)
	}
	defer con.Close()
	return runTagPrep(w, con, fullSID)
}

type tagSourceMatch struct {
	registration source.Registration
	adapter      source.Source
	container    source.Container
}

func refreshTagSession(
	sessionArg string,
	scope []view.Scope,
	more agentproto.ScopeFn,
	registrations []source.Registration,
) (string, string, error) {
	// Probe only the consolidated store first. Passing no scope builder prevents
	// a stale/missing row from triggering the old all-project indexing sweep
	// before the targeted source refresh gets a chance to run.
	dbp, fullSID, locateErr := agentproto.LocateSession(sessionArg, nil, nil)
	if locateErr == nil {
		match, ok := locatedTagSource(dbp, fullSID, registrations)
		if ok {
			if _, err := os.Stat(match.container.Path); err == nil {
				if err := refreshTagMatches([]tagSourceMatch{match}); err != nil {
					return "", "", err
				}
				return agentproto.LocateSession(fullSID, scope, more)
			} else if !os.IsNotExist(err) {
				return "", "", fmt.Errorf("inspect live transcript %s: %w", match.container.Path, err)
			}
		}
	}

	matches, discoverErr := discoverTagSources(sessionArg, registrations)
	if len(matches) > 0 {
		if err := refreshTagMatches(matches); err != nil {
			return "", "", err
		}
		return agentproto.LocateSession(sessionArg, scope, more)
	}
	if discoverErr != nil {
		return "", "", discoverErr
	}
	// No live source remains. Fall back to RawClaw's deliberately retained
	// history, applying the caller's project scope only at this final read.
	return agentproto.LocateSession(sessionArg, scope, more)
}

func locatedTagSource(
	dbp string,
	fullSID string,
	registrations []source.Registration,
) (tagSourceMatch, bool) {
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return tagSourceMatch{}, false
	}
	backing, ok, err := store.SessionBackingFor(con, fullSID)
	_ = con.Close()
	if err != nil || !ok || backing.SourcePath == "" {
		return tagSourceMatch{}, false
	}
	reg, ok := registrationFor(backing.SourceTool, backing.SourcePath, registrations)
	if !ok {
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
			ID:         fullSID,
			Path:       backing.SourcePath,
			CWD:        backing.CWD,
			IsSubagent: backing.IsSubagent,
			ParentID:   backing.ParentID,
		},
	}, true
}

func registrationFor(
	sourceID string,
	sourcePath string,
	registrations []source.Registration,
) (source.Registration, bool) {
	for _, reg := range registrations {
		if reg.ID == sourceID && reg.New != nil {
			return reg, true
		}
	}
	for _, reg := range registrations {
		if reg.New != nil && reg.Detect != nil && reg.Detect(sourcePath) {
			return reg, true
		}
	}
	return source.Registration{}, false
}

func discoverTagSources(
	sessionArg string,
	registrations []source.Registration,
) ([]tagSourceMatch, error) {
	prefix := agentproto.NormalizeSessionArg(sessionArg)
	matches := []tagSourceMatch{}
	var errs []error
	for _, reg := range registrations {
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
			if strings.HasPrefix(c.ID, prefix) {
				matches = append(matches, tagSourceMatch{
					registration: reg,
					adapter:      adapter,
					container:    c,
				})
			}
		}
	}
	exact := make([]tagSourceMatch, 0, len(matches))
	for _, match := range matches {
		if match.container.ID == prefix {
			exact = append(exact, match)
		}
	}
	if len(exact) > 0 {
		return exact, nil
	}
	nonSub := make([]tagSourceMatch, 0, len(matches))
	for _, match := range matches {
		if !match.container.IsSubagent {
			nonSub = append(nonSub, match)
		}
	}
	if len(nonSub) > 0 {
		return nonSub, nil
	}
	if len(matches) > 0 || len(errs) == 0 {
		return matches, nil
	}
	return nil, errors.Join(errs...)
}

func refreshTagMatches(matches []tagSourceMatch) error {
	for _, match := range matches {
		dbp := index.RefreshDBPath(
			match.registration.ID,
			match.container.ID,
			match.container.Path,
		)
		nMessages, err := index.EnsureFreshContainer(
			dbp,
			match.container,
			match.adapter.Messages,
			match.registration.ID,
		)
		if err != nil {
			return fmt.Errorf("refresh session %s from %s: %w", match.container.ID, match.registration.ID, err)
		}
		if nMessages == 0 {
			return fmt.Errorf("live session %q has no messages to tag", lastSlice8(match.container.ID))
		}
	}
	return nil
}
