package index

import (
	"fmt"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
)

func BenchmarkPruneTombstonedIDs(b *testing.B) {
	con, err := store.ConnectRW(b.TempDir() + "/prune.db")
	if err != nil {
		b.Fatal(err)
	}
	defer con.Close()
	if err := EnsureSchema(con, sourceClaude); err != nil {
		b.Fatal(err)
	}
	if err := store.EnsureTopicSchema(con); err != nil {
		b.Fatal(err)
	}
	ids := make([]string, 600)
	for i := range ids {
		ids[i] = fmt.Sprintf("missing_%d", i)
	}
	for i := 0; i < 200; i++ {
		ids = append(ids, fmt.Sprintf("live_%d", i))
	}
	seed := func() {
		tx, err := con.Begin()
		if err != nil {
			b.Fatal(err)
		}
		for i := 0; i < 200; i++ {
			id := fmt.Sprintf("live_%d", i)
			for _, q := range []string{
				"INSERT INTO sessions(id) VALUES (?)",
				"INSERT INTO messages(session_id, uuid) VALUES (?, ?)",
				"INSERT INTO session_sources(session_id, source_db) VALUES (?, ?)",
				"INSERT INTO file_index(path, session_id) VALUES (?, ?)",
				"INSERT INTO topic_segment(session_id, start_uuid) VALUES (?, ?)",
				"INSERT INTO session_verdict(session_id, verdict, source) VALUES (?, 'routine', 'bench')",
			} {
				args := []any{id}
				if q == "INSERT INTO messages(session_id, uuid) VALUES (?, ?)" || q == "INSERT INTO topic_segment(session_id, start_uuid) VALUES (?, ?)" {
					args = append(args, id+"-u")
				}
				if q == "INSERT INTO session_sources(session_id, source_db) VALUES (?, ?)" {
					args = append(args, "bench.db")
				}
				if q == "INSERT INTO file_index(path, session_id) VALUES (?, ?)" {
					args = []any{"/tmp/" + id, id}
				}
				if _, err := tx.Exec(q, args...); err != nil {
					_ = tx.Rollback()
					b.Fatal(err)
				}
			}
		}
		if err := tx.Commit(); err != nil {
			b.Fatal(err)
		}
	}
	seed()
	b.ResetTimer()
	for b.Loop() {
		b.StopTimer()
		if _, err := con.Exec("DELETE FROM sessions; DELETE FROM messages; DELETE FROM session_sources; DELETE FROM file_index; DELETE FROM topic_segment; DELETE FROM session_verdict"); err != nil {
			b.Fatal(err)
		}
		seed()
		b.StartTimer()
		if err := pruneTombstonedIDs(con, ids); err != nil {
			b.Fatal(err)
		}
	}
}
