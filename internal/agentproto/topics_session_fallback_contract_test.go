package agentproto

import (
	"errors"
	"os"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

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
