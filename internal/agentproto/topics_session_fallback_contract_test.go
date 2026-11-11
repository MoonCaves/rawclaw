package agentproto

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// Lifted from MoonCaves/rawclaw internal/agentproto/topics_session_fallback_contract_test.go L1-L214 @767be51bcca2bdbccf2f63c5e95d3edcff147032 (MIT).
func TestTopicsSessionFallsBackToProjectDB(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		populateConsolidated bool
	}{
		{name: "consolidated absent"},
		{name: "unrelated consolidated session", populateConsolidated: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_DATA_HOME", t.TempDir())

			project := t.TempDir()
			const sessionID = "aaaaaaaa-1111-2222-3333-000000000001"
			const messageUUID = "11111111-aaaa-bbbb-cccc-000000000001"
			const topic = "session fallback contract"
			const summary = "tag row from project-only cache"
			writeSession(t, project, sessionID, messageUUID, "the project-only tagged session")
			tagInto(t, project, sessionID, messageUUID, topic, summary)

			unrelated := t.TempDir()
			const unrelatedSessionID = "bbbbbbbb-1111-2222-3333-000000000002"
			writeSession(t, unrelated, unrelatedSessionID, "22222222-aaaa-bbbb-cccc-000000000002", "unrelated session")
			unrelatedDB, _, _, err := index.EnsureIndexed(unrelated, false)
			if err != nil {
				t.Fatalf("EnsureIndexed(unrelated): %v", err)
			}

			// An explicit unrelated-only scope must not resolve the target session.
			unrelatedScope := []view.Scope{{Project: paths.ProjectLabel(unrelated), TDir: unrelated}}
			_, err = Topics("", unrelatedScope, TopicsOpts{Session: sessionID[:8]})
			var notFound *ErrSessionNotFound
			if !errors.As(err, &notFound) {
				t.Fatalf("Topics(unrelated scope) error = %v, want *ErrSessionNotFound", err)
			}

			// The scope-isolation lookup can index through, so discard its store
			// state and build the final fixture from only the unrelated project.
			for _, suffix := range []string{"", "-wal", "-shm"} {
				if err := os.Remove(index.ConsolidatedPath() + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("remove consolidated store%s: %v", suffix, err)
				}
			}
			if tc.populateConsolidated {
				if _, err := index.ConsolidateFrom([]string{unrelatedDB}, true); err != nil {
					t.Fatalf("ConsolidateFrom(unrelated): %v", err)
				}
			}

			// Indexing can write through to the consolidated database, so verify
			// immediately before the positive call that it cannot supply this hit.
			if tc.populateConsolidated {
				con, sessions, err := index.OpenConsolidated()
				if err != nil {
					t.Fatalf("OpenConsolidated: %v", err)
				}
				defer con.Close()
				if sessions != 1 {
					t.Fatalf("consolidated sessions = %d, want only unrelated session", sessions)
				}
				targetRows, err := store.SessionRowsByPrefix(con, sessionID[:8], false, nil, 2)
				if err != nil {
					t.Fatalf("SessionRowsByPrefix(target): %v", err)
				}
				if len(targetRows) != 0 {
					t.Fatalf("target session is present in consolidated store: %+v", targetRows)
				}
				unrelatedRows, err := store.SessionRowsByPrefix(con, unrelatedSessionID[:8], false, nil, 2)
				if err != nil {
					t.Fatalf("SessionRowsByPrefix(unrelated): %v", err)
				}
				if len(unrelatedRows) != 1 {
					t.Fatalf("unrelated consolidated rows = %+v, want one", unrelatedRows)
				}
			} else if _, err := os.Stat(index.ConsolidatedPath()); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("consolidated store stat error = %v, want absent", err)
			}

			targetScope := []view.Scope{{Project: paths.ProjectLabel(project), TDir: project}}
			res, err := Topics("", targetScope, TopicsOpts{Session: sessionID[:8]})
			if err != nil {
				t.Fatalf("Topics(target scope): %v", err)
			}
			if len(res.Hits) != 1 {
				t.Fatalf("Topics hits = %+v, want the single target project topic", res.Hits)
			}
			got := res.Hits[0]
			if got.Topic != topic || got.ReadRef != "aaaaaaaa:11111111" || got.Summary != summary || got.Project != paths.ProjectLabel(project) {
				t.Errorf("Topics hit = %+v, want topic %q, read ref %q, summary %q, project %q",
					got, topic, "aaaaaaaa:11111111", summary, paths.ProjectLabel(project))
			}
		})
	}
}

func TestTopicsSessionPartialCatalogStillDetectsRetainedPrefixCollision(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(root, "cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	config := filepath.Join(root, "claude")
	t.Setenv("CLAUDE_CONFIG_DIR", config)
	t.Setenv("RAWCLAW_CATALOG_DIR", filepath.Join(root, "catalog"))
	live := filepath.Join(config, "projects", "live")
	retained := filepath.Join(config, "projects", "retained")
	for _, dir := range []string{
		filepath.Join(root, "home"), filepath.Join(root, "cache"), filepath.Join(root, "data"),
		live, retained,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}
	const liveSessionID = "aaaaaaaa-1111-2222-3333-000000000001"
	const liveMessageUUID = "11111111-aaaa-bbbb-cccc-000000000001"
	writeSession(t, live, liveSessionID, liveMessageUUID, "the live catalog session")
	tagInto(t, live, liveSessionID, liveMessageUUID, "live topic", "live session summary")
	liveDB := index.DBPath(live)
	if err := paths.WriteCatalogEntry(paths.CatalogDir(), paths.CatalogEntry{
		SessionID:      liveSessionID,
		TranscriptPath: filepath.Join(live, liveSessionID+".jsonl"),
		CWD:            live,
		Source:         "claude",
	}); err != nil {
		t.Fatalf("WriteCatalogEntry(live): %v", err)
	}
	if got := paths.ProjectDirOf(filepath.Join(live, liveSessionID+".jsonl")); got != paths.Realpath(live) {
		t.Fatalf("ProjectDirOf(live transcript) = %q, want %q", got, paths.Realpath(live))
	}

	const retainedSessionID = "aaaaaaaa-9999-8888-7777-000000000002"
	const retainedMessageUUID = "22222222-aaaa-bbbb-cccc-000000000002"
	const retainedTopic = "retained topic"
	const retainedSummary = "retained session summary"
	writeSession(t, retained, retainedSessionID, retainedMessageUUID, "the retained indexed session")
	tagInto(t, retained, retainedSessionID, retainedMessageUUID, retainedTopic, retainedSummary)
	retainedDB := index.DBPath(retained)
	if err := os.Remove(filepath.Join(retained, retainedSessionID+".jsonl")); err != nil {
		t.Fatalf("remove retained transcript: %v", err)
	}
	if err := os.RemoveAll(retained); err != nil {
		t.Fatalf("remove retained project: %v", err)
	}
	if _, err := index.EnsureOrphanReconciled(retainedDB); err != nil {
		t.Fatalf("EnsureOrphanReconciled(retained): %v", err)
	}

	scope := []view.Scope{
		{Project: paths.ProjectLabel(live), DBP: liveDB},
		{Project: paths.ProjectLabel(retained), DBP: retainedDB},
	}
	resetConsolidated := func() {
		t.Helper()
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if err := os.Remove(index.ConsolidatedPath() + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("remove consolidated store%s: %v", suffix, err)
			}
		}
	}
	resetConsolidated()
	res, err := Topics("", scope[1:], TopicsOpts{Session: retainedSessionID})
	if err != nil {
		t.Fatalf("Topics(exact retained session): %v", err)
	}
	if len(res.Hits) != 1 {
		t.Fatalf("Topics exact-session hits = %+v, want one retained topic", res.Hits)
	}
	got := res.Hits[0]
	if got.Topic != retainedTopic || got.ReadRef != "aaaaaaaa:22222222" || got.Summary != retainedSummary || got.Project != paths.ProjectLabel(retained) {
		t.Errorf("Topics exact-session hit = %+v, want topic %q, read ref %q, summary %q, project %q",
			got, retainedTopic, "aaaaaaaa:22222222", retainedSummary, paths.ProjectLabel(retained))
	}

	resetConsolidated()
	if _, err := os.Stat(index.ConsolidatedPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("consolidated store stat error = %v, want absent before prefix lookup", err)
	}
	catalogHits := paths.ResolveSession("aaaaaaaa")
	if len(catalogHits) != 1 || catalogHits[0].SessionID != liveSessionID {
		t.Fatalf("catalog hits = %+v, want only live session %q", catalogHits, liveSessionID)
	}
	_, err = Topics("", scope, TopicsOpts{Session: "aaaaaaaa"})
	var ambiguous *ErrAmbiguousSession
	if !errors.As(err, &ambiguous) {
		t.Fatalf("Topics(prefix) error = %v, want *ErrAmbiguousSession", err)
	}
	if len(ambiguous.Candidates) != 2 {
		t.Fatalf("ambiguous candidates = %+v, want the live and retained sessions", ambiguous.Candidates)
	}
	wantIDs := map[string]bool{liveSessionID: true, retainedSessionID: true}
	for _, candidate := range ambiguous.Candidates {
		if !wantIDs[candidate.SessionID] {
			t.Errorf("ambiguous candidate session = %q, want %v", candidate.SessionID, wantIDs)
		}
		delete(wantIDs, candidate.SessionID)
	}
	if len(wantIDs) != 0 {
		t.Errorf("ambiguous candidates missing sessions: %v", wantIDs)
	}
}
