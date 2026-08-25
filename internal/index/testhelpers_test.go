package index

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// isolateCache points HOME, XDG_CACHE_HOME, and XDG_DATA_HOME at a temp dir so
// ConsolidatedPath, per-project dbs, and the tombstone sidecar land under the
// test's own tree.
func isolateCache(t testing.TB) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	return home
}

// appendFile appends content string to the specified file.
func appendFile(t testing.TB, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("appendFile open %s: %v", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("appendFile write %s: %v", path, err)
	}
}

// writeFile writes content to a file, creating parent directories if needed.
func writeFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("writeFile mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile write %s: %v", path, err)
	}
}

// scalar runs a single-value query and returns it as a string ("<NULL>" if NULL).
func scalar(t testing.TB, con *sql.DB, q string, args ...any) string {
	t.Helper()
	var v sql.NullString
	if err := con.QueryRow(q, args...).Scan(&v); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	if !v.Valid {
		return "<NULL>"
	}
	return v.String
}

// scalarInt runs a single-value integer query.
func scalarInt(t testing.TB, con *sql.DB, q string, args ...any) int {
	t.Helper()
	var v int
	if err := con.QueryRow(q, args...).Scan(&v); err != nil {
		t.Fatalf("query int %q: %v", q, err)
	}
	return v
}

// openConsolidated opens the consolidated store read-only for assertions.
func openConsolidated(t testing.TB) *sql.DB {
	t.Helper()
	con, err := store.ConnectRO(ConsolidatedPath())
	if err != nil {
		t.Fatalf("open consolidated: %v", err)
	}
	t.Cleanup(func() { con.Close() })
	return con
}

// indexProject writes a one-file project dir, indexes it to its own cache db
// the way a real run does, and returns that db's path.
func indexProject(t *testing.T, name string, lines ...string) string {
	t.Helper()
	proj := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSONL(t, filepath.Join(proj, strings.TrimPrefix(name, "-")+".jsonl"), lines...)
	dbp, _, _, err := EnsureIndexed(proj, false)
	if err != nil {
		t.Fatalf("index %s: %v", name, err)
	}
	return dbp
}

// msgRow is one message in a seeded source db.
type msgRow struct {
	uuid, role, content string
	ts                  float64
}

// sessionRow is one session in a seeded source db. missing=0 means present.
type sessionRow struct {
	id, project, cwd string
	missing          float64
	msgs             []msgRow
}

// seedSessionDB writes a per-project cache db directly, bypassing the transcript reader.
func seedSessionDB(t testing.TB, name string, rows ...sessionRow) string {
	t.Helper()
	dbp := filepath.Join(t.TempDir(), name)
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open %s: %v", name, err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		t.Fatalf("schema %s: %v", name, err)
	}
	for _, r := range rows {
		var missing any
		if r.missing != 0 {
			missing = r.missing
		}
		var started, last float64
		for i, m := range r.msgs {
			if i == 0 || m.ts < started {
				started = m.ts
			}
			if m.ts > last {
				last = m.ts
			}
			if _, err := con.Exec(
				"INSERT INTO messages(session_id,role,content,ts,ts_iso,uuid) VALUES(?,?,?,?,?,?)",
				r.id, m.role, m.content, m.ts, "", m.uuid,
			); err != nil {
				t.Fatalf("seed message in %s: %v", name, err)
			}
		}
		if _, err := con.Exec(
			`INSERT INTO sessions(id,started_at,last_ts,message_count,is_subagent,parent_id,
			 origin_machine,source_tool,source_path,only_copy_since,project,cwd)
			 VALUES(?,?,?,?,0,NULL,'m',?,?,?,?,?)`,
			r.id, started, last, len(r.msgs), sourceClaude, "/t/"+r.id+".jsonl", missing, r.project, r.cwd,
		); err != nil {
			t.Fatalf("seed session in %s: %v", name, err)
		}
	}
	return dbp
}

// tagSession writes one topic segment into a per-project db.
func tagSession(t testing.TB, dbp, sessionID, startUUID, topic, summary string, taggedAt float64) {
	t.Helper()
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open %s for tagging: %v", dbp, err)
	}
	defer con.Close()
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("ensure topic schema: %v", err)
	}
	if err := store.UpsertTopicSegment(con, sessionID, startUUID, "", topic, summary, taggedAt); err != nil {
		t.Fatalf("tag %s: %v", sessionID, err)
	}
}

// tagSessionWithOrigin writes one topic segment with an explicit origin_machine into a per-project db.
func tagSessionWithOrigin(t testing.TB, dbp, sessionID, startUUID, topic, summary, origin string, taggedAt float64) {
	t.Helper()
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open %s for tagging: %v", dbp, err)
	}
	defer con.Close()
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("ensure topic schema: %v", err)
	}
	if err := store.ReplaceSessionSegments(con, sessionID, []store.TopicSegment{
		{
			SessionID:     sessionID,
			StartUUID:     startUUID,
			EndUUID:       startUUID,
			Topic:         topic,
			Summary:       summary,
			TaggedAt:      taggedAt,
			OriginMachine: origin,
		},
	}); err != nil {
		t.Fatalf("tag %s: %v", sessionID, err)
	}
}

// setVerdict writes one session verdict into a per-project db.
func setVerdict(t testing.TB, dbp string, v store.Verdict) {
	t.Helper()
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open %s for verdict: %v", dbp, err)
	}
	defer con.Close()
	if err := store.EnsureTopicSchema(con); err != nil {
		t.Fatalf("ensure topic schema: %v", err)
	}
	if err := store.UpsertVerdict(con, v); err != nil {
		t.Fatalf("upsert verdict on %s: %v", dbp, err)
	}
}

// firstSessionID returns the single session id in a project db.
func firstSessionID(t testing.TB, dbp string) string {
	t.Helper()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		t.Fatalf("open %s: %v", dbp, err)
	}
	defer con.Close()
	return scalar(t, con, "SELECT id FROM sessions LIMIT 1")
}

// rewindScopeColumns removes project/cwd columns to simulate pre-migration schema.
func rewindScopeColumns(t testing.TB, dbp string) string {
	t.Helper()
	con, err := store.ConnectRW(dbp)
	if err != nil {
		t.Fatalf("open db for rewind: %v", err)
	}
	defer con.Close()
	for _, c := range scopeColumns {
		if _, err := con.Exec("ALTER TABLE sessions DROP COLUMN " + c.name); err != nil {
			t.Fatalf("rewind %s: %v", c.name, err)
		}
	}
	if _, err := con.Exec("DELETE FROM meta WHERE key=?", scopeBackfillKey); err != nil {
		t.Fatalf("clear backfill stamp: %v", err)
	}
	return dbp
}

// assertFTSMatchCount verifies that FTS5 match query yields exactly the expected row count.
func assertFTSMatchCount(t testing.TB, con *sql.DB, query string, want int) {
	t.Helper()
	var count int
	err := con.QueryRow(
		`SELECT COUNT(*) FROM messages_fts f
		 JOIN messages m ON m.id = f.rowid
		 WHERE messages_fts MATCH ?`, query).Scan(&count)
	if err != nil {
		t.Fatalf("FTS query %q failed: %v", query, err)
	}
	if count != want {
		t.Fatalf("FTS match count for %q = %d, want %d", query, count, want)
	}
}

// assertMessagesEqual verifies that two databases have identical messages for sessionID.
func assertMessagesEqual(t testing.TB, con1, con2 *sql.DB, sessionID string) {
	t.Helper()
	rows1, err := con1.Query("SELECT role, content, ts, ts_iso, uuid FROM messages WHERE session_id=? ORDER BY id ASC", sessionID)
	if err != nil {
		t.Fatalf("assertMessagesEqual con1 query: %v", err)
	}
	defer rows1.Close()

	rows2, err := con2.Query("SELECT role, content, ts, ts_iso, uuid FROM messages WHERE session_id=? ORDER BY id ASC", sessionID)
	if err != nil {
		t.Fatalf("assertMessagesEqual con2 query: %v", err)
	}
	defer rows2.Close()

	for {
		has1 := rows1.Next()
		has2 := rows2.Next()
		if has1 != has2 {
			t.Fatalf("mismatched row count between databases: con1=%v, con2=%v", has1, has2)
		}
		if !has1 {
			break
		}
		var r1, r2 model.Message
		if err := rows1.Scan(&r1.Role, &r1.Text, &r1.TS, &r1.TSISO, &r1.UUID); err != nil {
			t.Fatalf("assertMessagesEqual con1 scan: %v", err)
		}
		if err := rows2.Scan(&r2.Role, &r2.Text, &r2.TS, &r2.TSISO, &r2.UUID); err != nil {
			t.Fatalf("assertMessagesEqual con2 scan: %v", err)
		}
		if r1 != r2 {
			t.Fatalf("mismatched message: con1=%+v, con2=%+v", r1, r2)
		}
	}
}

// claudeTailMsgsFn returns a Container-to-Messages function parsing Claude tail chunks.
func claudeTailMsgsFn() func(source.Container) ([]model.Message, error) {
	return func(got source.Container) ([]model.Message, error) {
		data, err := os.ReadFile(got.Path)
		if err != nil {
			return nil, err
		}
		return parseClaudeTail(data)
	}
}

// claudeFullMsgsFn returns a full file line-by-line Claude message parser.
func claudeFullMsgsFn() func(source.Container) ([]model.Message, error) {
	return func(got source.Container) ([]model.Message, error) {
		data, err := os.ReadFile(got.Path)
		if err != nil {
			return nil, err
		}
		var out []model.Message
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			var o map[string]any
			if err := json.Unmarshal([]byte(line), &o); err != nil || !indexable(o) {
				continue
			}
			text := parse.ExtractText(o)
			if text == "" {
				continue
			}
			iso, _ := o["timestamp"].(string)
			out = append(out, model.Message{
				Role:  parse.MsgRole(o),
				Text:  text,
				TS:    parse.ISOToEpoch(iso),
				TSISO: iso,
				UUID:  parse.MsgUUID(o),
			})
		}
		return out, nil
	}
}
