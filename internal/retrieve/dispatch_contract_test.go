package retrieve

import (
	"database/sql"
	"slices"
	"testing"
)

func TestSearchDispatchWithUnavailableWordIndex(t *testing.T) {
	tests := []struct {
		name   string
		drop   string
		query  string
		params SearchParams
		msgs   []testMsg
		want   []string
	}{
		{
			name:   "missing exact index keeps stemmed result",
			drop:   "messages_fts_exact",
			query:  "connect",
			params: SearchParams{RawMatch: "connect"},
			msgs:   []testMsg{{sessionID: "stemmed", role: "user", ts: 1, content: "connected"}},
			want:   []string{"stemmed"},
		},
		{
			name:   "missing stemmed index keeps exact result",
			drop:   "messages_fts",
			query:  "connection",
			params: SearchParams{RawMatch: "connection"},
			msgs:   []testMsg{{sessionID: "exact", role: "user", ts: 1, content: "connection"}},
			want:   []string{"exact"},
		},
		{
			name:   "exact mode does not rescue missing exact index from stemmed",
			drop:   "messages_fts_exact",
			query:  "connect",
			params: SearchParams{Exact: true, RawMatch: "connect"},
			msgs:   []testMsg{{sessionID: "stemmed", role: "user", ts: 1, content: "connected"}},
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			con, dbp := newTestDB(t, []testSession{{id: "exact", msgCount: 1}, {id: "stemmed", msgCount: 1}}, tt.msgs)
			defer con.Close()
			if _, err := con.Exec("DROP TABLE " + tt.drop); err != nil {
				t.Fatalf("drop %s: %v", tt.drop, err)
			}

			assertSearchAndAnchorIDs(t, con, dbp, tt.query, tt.params, tt.want, tt.drop)
		})
	}
}

func TestExactOverridesTimeSortDispatch(t *testing.T) {
	tests := []struct {
		name string
		sort string
		want []string
	}{
		{name: "newest", sort: "newest", want: []string{"nexus", "cobalt"}},
		{name: "oldest", sort: "oldest", want: []string{"cobalt", "nexus"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			con, dbp := newTestDB(t, []testSession{{id: "cobalt", msgCount: 1, lastTS: 1}, {id: "nexus", msgCount: 1, lastTS: 2}}, []testMsg{
				{sessionID: "cobalt", role: "user", ts: 1, content: "cobalt only"},
				{sessionID: "nexus", role: "user", ts: 2, content: "nexus only"},
			})
			defer con.Close()

			assertSearchAndAnchorIDs(t, con, dbp, "cobalt nexus", SearchParams{Exact: true, Sort: tt.sort}, tt.want, "")
		})
	}
}

func assertSearchAndAnchorIDs(t *testing.T, con *sql.DB, dbp, q string, p SearchParams, want []string, missingIndex string) {
	t.Helper()
	gotHits := sids(Search(dbp, q, 10, p))
	if !slices.Equal(gotHits, want) {
		t.Errorf("Search(%q, %+v) = %v, want %v", q, p, gotHits, want)
	}
	assertIndexAbsent(t, con, missingIndex)
	anchors := MatchAnchors(con, q, 10, p)
	gotAnchors := make([]string, len(anchors))
	for i, anchor := range anchors {
		gotAnchors[i] = anchor.SessionID
	}
	if !slices.Equal(gotAnchors, want) {
		t.Errorf("MatchAnchors(%q, %+v) = %v, want %v", q, p, gotAnchors, want)
	}
	assertIndexAbsent(t, con, missingIndex)
}

func assertIndexAbsent(t *testing.T, con *sql.DB, name string) {
	t.Helper()
	if name == "" {
		return
	}
	var count int
	if err := con.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?", name).Scan(&count); err != nil {
		t.Fatalf("check missing index %s: %v", name, err)
	}
	if count != 0 {
		t.Errorf("index %s was recreated, present tables = %d", name, count)
	}
}
