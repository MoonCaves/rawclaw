package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/agentproto"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/sources"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

var tagPrepStderr io.Writer = os.Stderr

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
	if dump, ok := freshPrewarmDump(session8); ok {
		_, err := io.WriteString(w, dump)
		return err
	}

	dbp, fullSID, toFold, err := refreshTagSession(session8, scope, more, registrations)
	if err != nil {
		return err
	}
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return fmt.Errorf("open %q read-only: %w", dbp, err)
	}

	existingSegs, err := readAuthoritativeTagTopics(dbp, fullSID)
	if err != nil {
		_ = con.Close()
		return err
	}
	prepErr := runTagPrepWithTopics(w, con, fullSID, existingSegs)
	_ = con.Close()
	if prepErr != nil {
		return prepErr
	}

	// The dump is already delivered from the proven-fresh refresh DB. Publish the
	// retained DB asynchronously so a busy consolidated store cannot delay the
	// answer; the ingest child will retry the fold later.
	if len(toFold) > 0 && !maybeSpawnIngest(fullSID) {
		fmt.Fprintln(tagPrepStderr, "# fold deferred; refresh db retained, will fold on next ingest")
	}
	return nil
}

func freshPrewarmDump(session8 string) (string, bool) {
	dbp, fullSID, err := agentproto.LocateConsolidatedSession(session8)
	if err != nil {
		return "", false
	}
	dumpPath := index.PrewarmDumpPath(fullSID)
	if !prewarmFresh(dumpPath, prewarmSourcePath(dbp, fullSID)) {
		return "", false
	}
	dump, err := os.ReadFile(dumpPath)
	if err != nil {
		return "", false
	}
	return string(dump), true
}

func readConsolidatedTopics(sessionID string) []store.TopicSegment {
	con, err := store.ConnectRO(index.ConsolidatedPath())
	if err != nil {
		return nil
	}
	defer con.Close()
	segs, _ := store.TopicsForSession(con, sessionID)
	return segs
}

// readAuthoritativeTagTopics reads the current per-session DB. The source DB
// is authoritative because a fold may be delayed.
func readAuthoritativeTagTopics(authoritativeDB, sessionID string) ([]store.TopicSegment, error) {
	auth, err := store.ConnectRO(authoritativeDB)
	if err != nil {
		return nil, fmt.Errorf("open authoritative tag store %q: %w", authoritativeDB, err)
	}
	defer auth.Close()
	authSegs, err := store.TopicsForSession(auth, sessionID)
	if err != nil {
		return nil, fmt.Errorf("read authoritative topics for %s: %w", sessionID, err)
	}
	if authoritativeDB == index.ConsolidatedPath() {
		return authSegs, nil
	}
	return append([]store.TopicSegment(nil), authSegs...), nil
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
) (string, string, []string, error) {
	// Probe only the consolidated store first. A scope builder returning nil prevents
	// a stale/missing row from triggering the old all-project indexing sweep
	// before the targeted source refresh gets a chance to run.
	dbp, fullSID, locateErr := agentproto.LocateSession(sessionArg, nil, func() []view.Scope { return nil })
	if locateErr == nil {
		match, ok := locatedTagSource(dbp, fullSID, registrations)
		if ok {
			if match.registration.ID == "goose" && !scopes.GooseOptedIn("") {
				fmt.Fprintln(tagPrepStderr, "# goose is opted out — serving indexed copy; set RAWCLAW_GOOSE=1 to refresh")
				return dbp, fullSID, nil, nil
			}
			if _, err := os.Stat(match.container.Path); err == nil {
				targetDBP, targetSID, toFold, err := refreshTagMatches([]tagSourceMatch{match}, sessionArg)
				if err != nil {
					return "", "", nil, err
				}
				return targetDBP, targetSID, toFold, nil
			} else if !os.IsNotExist(err) {
				return "", "", nil, fmt.Errorf("inspect live transcript %s: %w", match.container.Path, err)
			}
		}
	}

	stemMatches := stemTagSources(sessionArg, registrations)
	if len(stemMatches) > 0 {
		targetDBP, targetSID, toFold, err := refreshTagMatches(stemMatches, sessionArg)
		if err != nil {
			return "", "", nil, err
		}
		return targetDBP, targetSID, toFold, nil
	}

	matches, discoverErr := discoverTagSources(sessionArg, registrations)
	if len(matches) > 0 {
		targetDBP, targetSID, toFold, err := refreshTagMatches(matches, sessionArg)
		if err != nil {
			return "", "", nil, err
		}
		return targetDBP, targetSID, toFold, nil
	}
	if discoverErr != nil {
		return "", "", nil, discoverErr
	}
	// No live source remains. Fall back to RawClaw's deliberately retained
	// history, applying the caller's project scope only at this final read.
	histDBP, histSID, histErr := agentproto.LocateSessionGuarded(sessionArg, scope, more)
	if histErr != nil {
		return "", "", nil, histErr
	}
	return histDBP, histSID, nil, nil
}

func stemTagSources(
	sessionArg string,
	registrations []source.Registration,
) []tagSourceMatch {
	prefix := agentproto.NormalizeSessionArg(sessionArg)
	hits := paths.ResolveSession(prefix)
	if len(hits) == 0 {
		return nil
	}
	var matches []tagSourceMatch
	for _, hit := range hits {
		reg, ok := registrationFor("", hit.Path, registrations)
		if !ok {
			reg, ok = registrationFor("claude", hit.Path, registrations)
		}
		if !ok || reg.New == nil {
			continue
		}
		adapter := reg.New()
		if adapter == nil {
			continue
		}
		matches = append(matches, tagSourceMatch{
			registration: reg,
			adapter:      adapter,
			container: source.Container{
				ID:         hit.SessionID,
				Path:       hit.Path,
				CWD:        hit.CWD,
				IsSubagent: false,
				ParentID:   "",
				ResumeArgv: source.ResumeArgv(reg.ID, hit.SessionID),
			},
		})
	}
	exact := make([]tagSourceMatch, 0, len(matches))
	for _, match := range matches {
		if match.container.ID == prefix {
			exact = append(exact, match)
		}
	}
	if len(exact) > 0 {
		return exact
	}
	return matches
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
	discoveryErr := errors.Join(errs...)
	if len(exact) > 0 {
		return exact, discoveryErr
	}
	nonSub := make([]tagSourceMatch, 0, len(matches))
	for _, match := range matches {
		if !match.container.IsSubagent {
			nonSub = append(nonSub, match)
		}
	}
	if len(nonSub) > 0 {
		return nonSub, discoveryErr
	}
	return matches, discoveryErr
}

func refreshTagMatches(matches []tagSourceMatch, sessionArg string) (string, string, []string, error) {
	var (
		targetDBP string
		targetSID string
		toFold    []string
	)
	prefix := agentproto.NormalizeSessionArg(sessionArg)

	targetIdx := 0
	for i, match := range matches {
		if match.container.ID == prefix || match.container.ID == sessionArg || strings.HasPrefix(match.container.ID, prefix) {
			targetIdx = i
			break
		}
	}

	for i, match := range matches {
		dbp := index.RefreshDBPath(
			match.registration.ID,
			match.container.ID,
			match.container.Path,
		)
		nMessages, err := index.PrepareFreshContainer(
			dbp,
			match.container,
			match.adapter.Messages,
			match.registration.ID,
		)
		if err != nil {
			return "", "", nil, fmt.Errorf("refresh session %s from %s: %w", match.container.ID, match.registration.ID, err)
		}
		if nMessages == 0 {
			return "", "", nil, fmt.Errorf("live session %q has no messages to tag", lastSlice8(match.container.ID))
		}
		toFold = append(toFold, dbp)
		if i == targetIdx {
			targetDBP = dbp
			targetSID = match.container.ID
		}
	}
	return targetDBP, targetSID, toFold, nil
}
