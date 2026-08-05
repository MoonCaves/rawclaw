package retrieve

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

// testMsg is one row to seed into the in-test FTS5 db.
type testMsg struct {
	sessionID string
	role      string
	tsISO     string
	ts        float64
	content   string
}

// testSession is one sessions-table row.
type testSession struct {
	id         string
	isSubagent int
	parentID   string // "" = NULL
	msgCount   int
	lastTS     float64
}

// newTestDB builds a real on-disk FTS5 db using the production schema via
// storetest (D7), seeds sessions + messages, and returns the open fixture
// connection plus the db path. Using the real schema (triggers populate
// messages_fts) keeps the ranking identical to production.
func newTestDB(t *testing.T, sessions []testSession, msgs []testMsg) (*sql.DB, string) {
	t.Helper()
	con, dbp := storetest.NewDB(t)

	for _, s := range sessions {
		storetest.InsertSession(t, con, storetest.Session{
			ID:           s.id,
			LastTS:       s.lastTS,
			MessageCount: s.msgCount,
			IsSubagent:   s.isSubagent != 0,
			ParentID:     s.parentID,
		})
	}
	for _, m := range msgs {
		storetest.InsertMessage(t, con, storetest.Message{
			SessionID: m.sessionID,
			Role:      m.role,
			Content:   m.content,
			TS:        m.ts,
			ISO:       m.tsISO,
		})
	}
	return con, dbp
}

func sids(hits []Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.SessionID
	}
	return out
}

func TestSearch(t *testing.T) {
	sessions := []testSession{
		{id: "alpha", msgCount: 5, lastTS: 100},
		{id: "beta", msgCount: 5, lastTS: 200},
		{id: "gamma", msgCount: 5, lastTS: 300},
		{id: "sub1", isSubagent: 1, parentID: "alpha", msgCount: 5, lastTS: 150},
	}

	tests := []struct {
		name     string
		msgs     []testMsg
		query    string
		limit    int
		params   SearchParams
		wantSIDs []string // exact order expected
	}{
		{
			name: "single term plain match",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "deploy the kubernetes cluster"},
				{sessionID: "beta", role: "user", tsISO: "2026-06-02", ts: 2, content: "unrelated nginx config"},
			},
			query:    "kubernetes",
			limit:    10,
			wantSIDs: []string{"alpha"},
		},
		{
			name: "multi-term OR recall finds either",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "the kubernetes deploy went fine"},
				{sessionID: "beta", role: "user", tsISO: "2026-06-02", ts: 2, content: "we discussed redis caching only"},
			},
			// OR semantics: a doc with only ONE of the terms still returns
			// (FTS5 implicit-AND would whiff here).
			query:    "kubernetes redis",
			limit:    10,
			wantSIDs: []string{"alpha", "beta"},
		},
		{
			name: "coverage re-rank floats higher-coverage doc up",
			msgs: []testMsg{
				// alpha matches one term; beta matches both -> beta first.
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "only kubernetes here"},
				{sessionID: "beta", role: "user", tsISO: "2026-06-02", ts: 2, content: "kubernetes and redis together"},
			},
			query:    "kubernetes redis",
			limit:    10,
			wantSIDs: []string{"beta", "alpha"},
		},
		{
			name: "tool-only match excluded by default",
			msgs: []testMsg{
				// Human text has no 'bashfoo'; only the stripped tool run did.
				{sessionID: "alpha", role: "assistant", tsISO: "2026-06-01", ts: 1, content: "here goes [TOOL:Bash] bashfoo unique"},
				{sessionID: "beta", role: "user", tsISO: "2026-06-02", ts: 2, content: "bashfoo unique in plain human text"},
			},
			query:    "bashfoo",
			limit:    10,
			wantSIDs: []string{"beta"},
		},
		{
			name: "tool-only match surfaces with include-tools",
			msgs: []testMsg{
				{sessionID: "alpha", role: "assistant", tsISO: "2026-06-01", ts: 1, content: "here goes [TOOL:Bash] bashfoo unique"},
			},
			query:    "bashfoo",
			limit:    10,
			params:   SearchParams{IncludeTools: true},
			wantSIDs: []string{"alpha"},
		},
		{
			name: "subagent excluded by default",
			msgs: []testMsg{
				{sessionID: "sub1", role: "user", tsISO: "2026-06-01", ts: 1, content: "secretword in a subagent"},
			},
			query:    "secretword",
			limit:    10,
			wantSIDs: []string{},
		},
		{
			name: "subagent included with flag",
			msgs: []testMsg{
				{sessionID: "sub1", role: "user", tsISO: "2026-06-01", ts: 1, content: "secretword in a subagent"},
			},
			query:    "secretword",
			limit:    10,
			params:   SearchParams{IncludeSubagents: true},
			wantSIDs: []string{"sub1"},
		},
		{
			name: "role filter",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "roletest from user"},
				{sessionID: "beta", role: "assistant", tsISO: "2026-06-02", ts: 2, content: "roletest from assistant"},
			},
			query:    "roletest",
			limit:    10,
			params:   SearchParams{Role: "assistant"},
			wantSIDs: []string{"beta"},
		},
		{
			name: "since date bound is inclusive and filters before limit",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "datedword early"},
				{sessionID: "beta", role: "user", tsISO: "2026-06-10", ts: 2, content: "datedword late"},
			},
			query:    "datedword",
			limit:    10,
			params:   SearchParams{Since: "2026-06-05"},
			wantSIDs: []string{"beta"},
		},
		{
			name: "before date bound inclusive",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "datedword early"},
				{sessionID: "beta", role: "user", tsISO: "2026-06-10", ts: 2, content: "datedword late"},
			},
			query:    "datedword",
			limit:    10,
			params:   SearchParams{Before: "2026-06-05"},
			wantSIDs: []string{"alpha"},
		},
		{
			name: "min_messages filter",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "minmsgword here"},
			},
			query:    "minmsgword",
			limit:    10,
			params:   SearchParams{MinMessages: 99},
			wantSIDs: []string{},
		},
		{
			name: "sort newest overrides relevance and skips coverage re-rank",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "kubernetes and redis both"},
				{sessionID: "gamma", role: "user", tsISO: "2026-06-03", ts: 3, content: "only kubernetes"},
			},
			query:    "kubernetes redis",
			limit:    10,
			params:   SearchParams{Sort: "newest"},
			wantSIDs: []string{"gamma", "alpha"}, // ts DESC, not coverage
		},
		{
			name: "sort oldest",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "kubernetes here"},
				{sessionID: "gamma", role: "user", tsISO: "2026-06-03", ts: 3, content: "kubernetes there"},
			},
			query:    "kubernetes",
			limit:    10,
			params:   SearchParams{Sort: "oldest"},
			wantSIDs: []string{"alpha", "gamma"},
		},
		{
			name: "limit caps results",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "limitword a"},
				{sessionID: "beta", role: "user", tsISO: "2026-06-02", ts: 2, content: "limitword b"},
				{sessionID: "gamma", role: "user", tsISO: "2026-06-03", ts: 3, content: "limitword c"},
			},
			query: "limitword",
			limit: 2,
			// relevance order is bm25; just assert count via length below.
			wantSIDs: nil,
		},
		{
			name: "empty/stopword-only query returns nothing",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "the and of"},
			},
			query:    "the and of",
			limit:    10,
			wantSIDs: []string{},
		},
		{
			name: "raw_match explicit boolean expr verbatim",
			msgs: []testMsg{
				{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "apple and banana present"},
				{sessionID: "beta", role: "user", tsISO: "2026-06-02", ts: 2, content: "apple alone no fruit pair"},
			},
			query:    "apple banana",
			limit:    10,
			params:   SearchParams{RawMatch: `"apple" AND "banana"`},
			wantSIDs: []string{"alpha"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, dbp := newTestDB(t, sessions, tt.msgs)
			got := Search(dbp, tt.query, tt.limit, tt.params)

			if tt.name == "limit caps results" {
				if len(got) != 2 {
					t.Fatalf("limit cap: got %d hits, want 2", len(got))
				}
				return
			}
			if tt.wantSIDs == nil {
				return
			}
			gotSIDs := sids(got)
			if !equalStrings(gotSIDs, tt.wantSIDs) {
				t.Fatalf("session ids = %v, want %v", gotSIDs, tt.wantSIDs)
			}
		})
	}
}

func TestSearchSnippetHighlight(t *testing.T) {
	sessions := []testSession{{id: "alpha", msgCount: 5, lastTS: 100}}
	msgs := []testMsg{
		{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1,
			content: "we should configure the kubernetes ingress today"},
	}
	_, dbp := newTestDB(t, sessions, msgs)
	got := Search(dbp, "kubernetes", 10, SearchParams{})
	if len(got) != 1 {
		t.Fatalf("got %d hits, want 1", len(got))
	}
	if want := ">>>kubernetes<<<"; !containsSub(got[0].Snippet, want) {
		t.Fatalf("snippet %q missing highlight %q", got[0].Snippet, want)
	}
}

func TestSearchNonexistentDB(t *testing.T) {
	// An unopenable / missing db must degrade to an empty result, never panic.
	got := Search("/nonexistent/path/to/missing.db", "anything", 10, SearchParams{})
	if len(got) != 0 {
		t.Fatalf("missing db: got %d hits, want 0", len(got))
	}
}

func TestMatchAnchors(t *testing.T) {
	sessions := []testSession{
		{id: "alpha", msgCount: 5, lastTS: 100},
		{id: "beta", msgCount: 5, lastTS: 200},
	}
	msgs := []testMsg{
		{sessionID: "alpha", role: "user", tsISO: "2026-06-01", ts: 1, content: "only kubernetes here"},
		{sessionID: "beta", role: "user", tsISO: "2026-06-02", ts: 2, content: "kubernetes and redis together"},
	}
	con, _ := newTestDB(t, sessions, msgs)

	got := MatchAnchors(con, "kubernetes redis", 100, SearchParams{})
	if len(got) != 2 {
		t.Fatalf("got %d anchors, want 2", len(got))
	}
	// Coverage re-rank: beta (cov 2) before alpha (cov 1).
	if got[0].SessionID != "beta" || got[1].SessionID != "alpha" {
		t.Fatalf("anchor order = [%s,%s], want [beta,alpha]", got[0].SessionID, got[1].SessionID)
	}
	if got[0].Cov != 2 || got[1].Cov != 1 {
		t.Fatalf("coverage = [%d,%d], want [2,1]", got[0].Cov, got[1].Cov)
	}
	if got[0].ID == 0 {
		t.Fatalf("anchor must carry a message id, got 0")
	}
}

func TestLineageRoot(t *testing.T) {
	// root <- mid <- leaf ; plus a self-cycle guard and a missing-session id.
	sessions := []testSession{
		{id: "root", msgCount: 1, lastTS: 1},
		{id: "mid", parentID: "root", msgCount: 1, lastTS: 2},
		{id: "leaf", parentID: "mid", msgCount: 1, lastTS: 3},
		{id: "selfcycle", parentID: "selfcycle", msgCount: 1, lastTS: 4},
	}
	con, _ := newTestDB(t, sessions, nil)

	tests := []struct {
		name string
		sid  string
		want string
	}{
		{"leaf walks to root", "leaf", "root"},
		{"mid walks to root", "mid", "root"},
		{"root is its own root", "root", "root"},
		{"self-cycle terminates", "selfcycle", "selfcycle"},
		{"unknown id returns itself", "ghost", "ghost"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LineageRoot(con, tt.sid); got != tt.want {
				t.Fatalf("LineageRoot(%q) = %q, want %q", tt.sid, got, tt.want)
			}
		})
	}
}

func TestStripBoolOps(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"apple && banana", "apple   banana"},
		{"apple || banana", "apple   banana"},
		{"!apple banana", " apple banana"},
		{"foo!bar", "foo!bar"}, // '!' preceded by word byte is NOT an operator
		{"a !b", "a  b"},
	}
	for _, tt := range tests {
		if got := stripBoolOps(tt.in); got != tt.want {
			t.Fatalf("stripBoolOps(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsSub(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// TestSearchRoutesSubstringProbe pins the whole point of the substring index at
// the level a caller sees it: a probe landing inside a word returns nothing from
// the word index, and now returns the message anyway. The same fixture also
// pins that a word query is untouched by the routing — it never reaches the
// fallback, because the fallback only fires on zero word hits.
func TestSearchRoutesSubstringProbe(t *testing.T) {
	sessions := []testSession{{id: "ledger", msgCount: 5, lastTS: 100}}
	msgs := []testMsg{
		{sessionID: "ledger", role: "user", tsISO: "2026-06-01", ts: 1,
			content: "we should configure the reconciliation job today"},
	}
	_, dbp := newTestDB(t, sessions, msgs)

	// "iliat" sits inside "reconciliation" and is on no token boundary.
	got := Search(dbp, "iliat", 10, SearchParams{})
	if len(got) != 1 {
		t.Fatalf("substring probe: got %d hits, want 1", len(got))
	}
	if got[0].SessionID != "ledger" {
		t.Errorf("substring probe: session %q, want ledger", got[0].SessionID)
	}
	if !containsSub(got[0].Snippet, "iliat") {
		t.Errorf("substring probe: snippet %q must highlight the probe", got[0].Snippet)
	}

	// The word path still answers a word query, unchanged.
	if w := Search(dbp, "reconciliation", 10, SearchParams{}); len(w) != 1 {
		t.Errorf("word query: got %d hits, want 1", len(w))
	}
	// A probe matching nothing at all stays empty — the fallback is a second
	// chance, not a way to invent hits.
	if none := Search(dbp, "zzqxw", 10, SearchParams{}); len(none) != 0 {
		t.Errorf("unmatchable probe: got %d hits, want 0", len(none))
	}
}

// TestMatchAnchorsRoutesSubstringProbe is the anchor-recall half of
// TestSearchRoutesSubstringProbe: read-refs must be reachable by substring too,
// or a substring hit could be found but not opened.
func TestMatchAnchorsRoutesSubstringProbe(t *testing.T) {
	sessions := []testSession{{id: "ledger", msgCount: 5, lastTS: 100}}
	msgs := []testMsg{
		{sessionID: "ledger", role: "user", tsISO: "2026-06-01", ts: 1,
			content: "we should configure the reconciliation job today"},
	}
	con, _ := newTestDB(t, sessions, msgs)

	got := MatchAnchors(con, "iliat", 100, SearchParams{})
	if len(got) != 1 {
		t.Fatalf("substring probe: got %d anchors, want 1", len(got))
	}
	if got[0].ID == 0 {
		t.Errorf("substring anchor must carry a message id, got 0")
	}
	if got[0].SessionID != "ledger" {
		t.Errorf("substring anchor: session %q, want ledger", got[0].SessionID)
	}
}

// TestSubstringFallbackRoutingRule pins the routing rule itself, including the
// three cases that must NOT route — each of which is a way the fallback could
// quietly change behavior it has no business touching.
func TestSubstringFallbackRoutingRule(t *testing.T) {
	tests := []struct {
		name      string
		q         string
		p         SearchParams
		wordHits  int
		wantRoute bool
		wantMatch string
	}{
		{name: "no word hits routes", q: "iliat", wantRoute: true, wantMatch: `"iliat"`},
		{name: "phrase with a space routes as one phrase", q: "ion job", wantRoute: true, wantMatch: `"ion job"`},
		{name: "embedded quote is doubled", q: `a"b`, wantRoute: true, wantMatch: `"a""b"`},
		{name: "word index answered so no route", q: "iliat", wordHits: 1},
		{name: "explicit boolean query is left alone", q: "iliat", p: SearchParams{RawMatch: "a OR b"}},
		// Two characters cannot form a trigram, so the query could only ever
		// return nothing — asking is pure cost.
		{name: "shorter than a trigram does not route", q: "il"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, terms, route := substringFallback(tt.q, tt.p, tt.wordHits)
			if route != tt.wantRoute {
				t.Fatalf("route = %v, want %v", route, tt.wantRoute)
			}
			if !route {
				return
			}
			if match != tt.wantMatch {
				t.Errorf("match = %q, want %q", match, tt.wantMatch)
			}
			if len(terms) != 1 || terms[0] != strings.ToLower(strings.TrimSpace(tt.q)) {
				t.Errorf("terms = %q, want the lowercased probe as the single highlight term", terms)
			}
		})
	}
}
