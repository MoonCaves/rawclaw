package store_test

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

// tagSegment seeds one taggable message plus the topic segment anchored on it,
// in the given project. Each caller supplies a unique uuid because the segment
// is keyed by (session_id, start_uuid).
func tagSegment(t *testing.T, con *sql.DB, project, cwd, sid, uuid, topic, summary string) {
	t.Helper()
	storetest.InsertSession(t, con, storetest.Session{
		ID: sid, MessageCount: 1, Project: project, CWD: cwd, SourceTool: "claude"})
	storetest.InsertMessage(t, con, storetest.Message{
		SessionID: sid, Role: "user", Content: "anchor for " + topic, UUID: uuid})
	if err := store.UpsertTopicSegment(con, sid, uuid, "", topic, summary, 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment(%s/%s): %v", project, topic, err)
	}
}

// seedLopsidedTopics builds the shape that broke the old fan-out: one LARGE
// project holding most of the corpus, and one tiny project holding a single
// segment that mentions the query word once, buried in a long summary.
//
// The large project owns the answer — a segment whose LABEL is the query word.
// Split across two databases, the tiny project's lone mention had extreme idf in
// its own tiny corpus and outscored the label hit in the big one; that is
// exactly why merging per-database scores was reverted. One store scores both
// against the same corpus, so the label wins on its own.
func seedLopsidedTopics(t *testing.T) *sql.DB {
	t.Helper()
	con, _ := storetest.NewDB(t)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}

	// The large project: bulk segments about unrelated things, plus the one that
	// is actually named after the query word.
	filler := []string{
		"schema migration", "connection pooling", "retry backoff", "index rebuild",
		"queue draining", "clock skew", "cache eviction", "batch sizing",
		"shard rebalance", "log rotation", "token refresh", "quota accounting",
	}
	for i, topic := range filler {
		tagSegment(t, con, "billing", "/src/billing",
			"big"+string(rune('a'+i)), "uuid-big-"+string(rune('a'+i)),
			topic, "notes on "+topic+" in the billing service")
	}
	tagSegment(t, con, "billing", "/src/billing", "bigwin", "uuid-big-win",
		"watermark", "how the watermark is advanced")

	// The tiny project: one segment, one passing mention inside a long summary.
	tagSegment(t, con, "ledger", "/src/ledger", "tiny", "uuid-tiny",
		"weekly planning",
		"a long recap of the week covering staffing, the release calendar, the "+
			"open questions on pricing, a tangent about the watermark, the review "+
			"backlog, and what we agreed to defer until the next planning round")

	return con
}

// TestMatchTopicsRanksLabelOverPassingMention is the ranking regression: a weak
// hit from a small project must not outrank a strong hit from a large one. The
// weak hit is a single word inside a long summary; the strong hit is a topic
// label. Nothing in MatchTopics weights the label column — the assertion holds
// on corpus statistics alone, which is the point of having one store.
func TestMatchTopicsRanksLabelOverPassingMention(t *testing.T) {
	con := seedLopsidedTopics(t)

	hits, err := store.MatchTopics(con, "watermark", 10, nil)
	if err != nil {
		t.Fatalf("MatchTopics: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("MatchTopics(watermark) = %d hits, want both the label and the mention: %+v", len(hits), hits)
	}
	if hits[0].Topic != "watermark" {
		t.Errorf("top hit = %q (project %q), want the label hit %q from the large project",
			hits[0].Topic, hits[0].Project, "watermark")
	}
	if hits[0].Project != "billing" {
		t.Errorf("top hit project = %q, want billing", hits[0].Project)
	}
	if hits[1].Topic != "weekly planning" {
		t.Errorf("second hit = %q, want the passing mention %q", hits[1].Topic, "weekly planning")
	}
}

// TestMatchTopicsNarrowsByProject confirms scope is a WHERE clause over the one
// store: naming a project drops every other project's hits, and naming a project
// the store has never heard of returns nothing rather than everything.
func TestMatchTopicsNarrowsByProject(t *testing.T) {
	con := seedLopsidedTopics(t)

	only, err := store.MatchTopics(con, "watermark", 10, []string{"ledger"})
	if err != nil {
		t.Fatalf("MatchTopics(ledger): %v", err)
	}
	if len(only) != 1 || only[0].Project != "ledger" {
		t.Fatalf("MatchTopics narrowed to ledger = %+v, want exactly the ledger hit", only)
	}

	none, err := store.MatchTopics(con, "watermark", 10, []string{"payroll"})
	if err != nil {
		t.Fatalf("MatchTopics(payroll): %v", err)
	}
	if len(none) != 0 {
		t.Errorf("MatchTopics narrowed to an unknown project = %+v, want no hits", none)
	}
}

// TestMatchTopicsCapsTheCombinedList confirms limit caps the ONE ranked list
// rather than each project's slice of it: with a limit of 1 exactly one hit
// comes back, even though two projects match.
func TestMatchTopicsCapsTheCombinedList(t *testing.T) {
	con := seedLopsidedTopics(t)

	hits, err := store.MatchTopics(con, "watermark", 1, nil)
	if err != nil {
		t.Fatalf("MatchTopics: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("MatchTopics(limit=1) = %d hits, want 1 (a global cap, not one per project)", len(hits))
	}
	if hits[0].Project != "billing" {
		t.Errorf("the surviving hit is from %q, want the top-ranked project billing", hits[0].Project)
	}
}

// TestMatchTopicsSurvivesAMissingSessionRow guards the LEFT join: the project
// label is metadata for the caller, so a segment whose session row never made it
// into this database still surfaces, just without a label.
func TestMatchTopicsSurvivesAMissingSessionRow(t *testing.T) {
	con, _ := storetest.NewDB(t)
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("EnsureTopicSchema: %v", err)
	}
	// A message and a segment, but deliberately NO sessions row for "orphan".
	storetest.InsertMessage(t, con, storetest.Message{
		SessionID: "orphan", Role: "user", Content: "anchor", UUID: "uuid-orphan"})
	if err := store.UpsertTopicSegment(con, "orphan", "uuid-orphan", "",
		"watermark", "how the watermark is advanced", 1.0); err != nil {
		t.Fatalf("UpsertTopicSegment: %v", err)
	}

	hits, err := store.MatchTopics(con, "watermark", 10, nil)
	if err != nil {
		t.Fatalf("MatchTopics: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("MatchTopics = %d hits, want the sessionless segment to survive", len(hits))
	}
	if hits[0].Project != "" {
		t.Errorf("hit Project = %q, want an empty label for a segment with no session row", hits[0].Project)
	}
}

// TestMatchTopicsCarriesTheProjectLabel confirms every hit reports which project
// it came from, which is what lets one merged list still be attributable.
func TestMatchTopicsCarriesTheProjectLabel(t *testing.T) {
	con := seedLopsidedTopics(t)

	hits, err := store.MatchTopics(con, "watermark planning", 10, nil)
	if err != nil {
		t.Fatalf("MatchTopics: %v", err)
	}
	seen := map[string]bool{}
	for _, h := range hits {
		seen[h.Project] = true
	}
	for _, want := range []string{"billing", "ledger"} {
		if !seen[want] {
			t.Errorf("no hit labelled %q; got labels %v", want, keysOf(seen))
		}
	}
}

// keysOf renders a label set for a failure message.
func keysOf(m map[string]bool) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return strings.Join(out, ",")
}

func TestSessionHasRealSegments(t *testing.T) {
	t.Run("missing table returns false and nil error", func(t *testing.T) {
		con, _ := storetest.NewDB(t)
		// Do not call EnsureTopicSchema, so topic_segment table does not exist
		has, err := store.SessionHasRealSegments(con, "sess-1")
		if err != nil {
			t.Fatalf("expected nil error on missing table, got %v", err)
		}
		if has {
			t.Fatalf("expected false on missing table, got true")
		}
	})

	t.Run("returns true when session has real segment", func(t *testing.T) {
		con, _ := storetest.NewDB(t)
		if err := store.EnsureTopicSchema(con); err != nil {
			t.Fatalf("EnsureTopicSchema: %v", err)
		}
		storetest.InsertMessage(t, con, storetest.Message{
			SessionID: "sess-1", Role: "user", Content: "hello", UUID: "uuid-1",
		})
		if err := store.UpsertTopicSegment(con, "sess-1", "uuid-1", "", "billing", "summary", 1.0); err != nil {
			t.Fatalf("UpsertTopicSegment: %v", err)
		}

		has, err := store.SessionHasRealSegments(con, "sess-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !has {
			t.Fatalf("expected true, got false")
		}
	})

	t.Run("returns false when session has no real segment", func(t *testing.T) {
		con, _ := storetest.NewDB(t)
		if err := store.EnsureTopicSchema(con); err != nil {
			t.Fatalf("EnsureTopicSchema: %v", err)
		}
		storetest.InsertMessage(t, con, storetest.Message{
			SessionID: "sess-1", Role: "user", Content: "hello", UUID: "uuid-1",
		})
		if err := store.UpsertTopicSegment(con, "sess-1", "uuid-1", "", "", "summary only", 1.0); err != nil {
			t.Fatalf("UpsertTopicSegment: %v", err)
		}

		has, err := store.SessionHasRealSegments(con, "sess-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if has {
			t.Fatalf("expected false for empty topic, got true")
		}
	})

	t.Run("returns error on database failure", func(t *testing.T) {
		con, _ := storetest.NewDB(t)
		if err := store.EnsureTopicSchema(con); err != nil {
			t.Fatalf("EnsureTopicSchema: %v", err)
		}
		// Close the database connection to force an error
		con.Close()

		_, err := store.SessionHasRealSegments(con, "sess-1")
		if err == nil {
			t.Fatalf("expected error on closed database connection, got nil")
		}
	})
}
