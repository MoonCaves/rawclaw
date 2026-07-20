package store_test

import (
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/store/storetest"
)

func TestMessagesBeforeAfter(t *testing.T) {
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "s"})
	var ids []int
	for _, c := range []string{"m1", "m2", "m3", "m4"} {
		ids = append(ids, storetest.InsertMessage(t, con, storetest.Message{SessionID: "s", Role: "user", Content: c, UUID: c}))
	}
	anchor := ids[2] // m3

	// Before: id<=anchor, DESC, limit — the anchor row itself is INCLUDED.
	before, err := store.MessagesBefore(con, "s", anchor, 2)
	if err != nil {
		t.Fatalf("MessagesBefore: %v", err)
	}
	if len(before) != 2 || before[0].Content != "m3" || before[1].Content != "m2" {
		t.Errorf("MessagesBefore = %+v, want [m3 m2] (DESC, anchor included)", before)
	}

	// After: id>anchor, ASC — the anchor row is EXCLUDED.
	after, err := store.MessagesAfter(con, "s", anchor, 5)
	if err != nil {
		t.Fatalf("MessagesAfter: %v", err)
	}
	if len(after) != 1 || after[0].Content != "m4" {
		t.Errorf("MessagesAfter = %+v, want [m4]", after)
	}

	// Other sessions never bleed in; an unknown session reads empty.
	if got, err := store.MessagesBefore(con, "nope", anchor, 5); err != nil || len(got) != 0 {
		t.Errorf("MessagesBefore(nope) = %v (%v), want empty", got, err)
	}
}

func TestBookendMessages(t *testing.T) {
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "s"})
	var ids []int
	msgs := []storetest.Message{
		{SessionID: "s", Role: "user", Content: "u1", UUID: "a"},
		{SessionID: "s", Role: "tool", Content: "tool noise", UUID: "b"}, // filtered: role
		{SessionID: "s", Role: "assistant", Content: "", UUID: "c"},      // filtered: empty
		{SessionID: "s", Role: "assistant", Content: "a1", UUID: "d"},
		{SessionID: "s", Role: "user", Content: "u2", UUID: "e"},
		{SessionID: "s", Role: "assistant", Content: "a2", UUID: "f"},
	}
	for _, m := range msgs {
		ids = append(ids, storetest.InsertMessage(t, con, m))
	}

	// Unbounded ascending: session-start bookend (user/assistant, non-empty).
	got, err := store.BookendMessages(con, "s", 0, false, true, 2)
	if err != nil {
		t.Fatalf("BookendMessages: %v", err)
	}
	if len(got) != 2 || got[0].Content != "u1" || got[1].Content != "a1" {
		t.Errorf("BookendMessages start = %+v, want [u1 a1]", got)
	}

	// Unbounded descending: session-end bookend, DESC order.
	got, err = store.BookendMessages(con, "s", 0, false, false, 2)
	if err != nil || len(got) != 2 || got[0].Content != "a2" || got[1].Content != "u2" {
		t.Errorf("BookendMessages end = %+v (%v), want [a2 u2]", got, err)
	}

	// Bounded ascending: id < bound (the run-up to a window).
	got, err = store.BookendMessages(con, "s", ids[4], true, true, 5)
	if err != nil || len(got) != 2 || got[0].Content != "u1" || got[1].Content != "a1" {
		t.Errorf("BookendMessages bounded-asc = %+v (%v), want [u1 a1]", got, err)
	}

	// Bounded descending: id > bound (the tail after a window).
	got, err = store.BookendMessages(con, "s", ids[3], true, false, 5)
	if err != nil || len(got) != 2 || got[0].Content != "a2" || got[1].Content != "u2" {
		t.Errorf("BookendMessages bounded-desc = %+v (%v), want [a2 u2]", got, err)
	}
}

func TestFirstUserMessages(t *testing.T) {
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "s"})
	for _, m := range []storetest.Message{
		{SessionID: "s", Role: "assistant", Content: "not user", UUID: "a"},
		{SessionID: "s", Role: "user", Content: "", UUID: "b"}, // empty: filtered
		{SessionID: "s", Role: "user", Content: "hi", UUID: "c"},
		{SessionID: "s", Role: "user", Content: "real question", UUID: "d"},
		{SessionID: "s", Role: "user", Content: "beyond limit", UUID: "e"},
	} {
		storetest.InsertMessage(t, con, m)
	}
	got, err := store.FirstUserMessages(con, "s", 2)
	if err != nil {
		t.Fatalf("FirstUserMessages: %v", err)
	}
	if len(got) != 2 || got[0] != "hi" || got[1] != "real question" {
		t.Errorf("FirstUserMessages = %v, want [hi, real question]", got)
	}
	if got, err := store.FirstUserMessages(con, "empty", 5); err != nil || len(got) != 0 {
		t.Errorf("FirstUserMessages(empty) = %v (%v), want none", got, err)
	}
}

func TestSessionMessagesAndAllMessages(t *testing.T) {
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "s1"})
	storetest.InsertSession(t, con, storetest.Session{ID: "s2"})
	id1 := storetest.InsertMessage(t, con, storetest.Message{SessionID: "s1", Role: "user", Content: "one", UUID: "uuid-1"})
	id2 := storetest.InsertMessage(t, con, storetest.Message{SessionID: "s1", Role: "tool", Content: "two", UUID: "uuid-2"})
	storetest.InsertMessage(t, con, storetest.Message{SessionID: "s2", Role: "user", Content: "other", UUID: "uuid-3"})

	// SessionMessages: full spine incl. uuid and tool rows, id order.
	msgs, err := store.SessionMessages(con, "s1")
	if err != nil {
		t.Fatalf("SessionMessages: %v", err)
	}
	if len(msgs) != 2 || msgs[0].ID != id1 || msgs[0].UUID != "uuid-1" || msgs[1].ID != id2 || msgs[1].Role != "tool" {
		t.Errorf("SessionMessages = %+v, want 2 rows with uuids in id order", msgs)
	}

	// AllMessages: the corpus-wide scan spans sessions.
	all, err := store.AllMessages(con)
	if err != nil {
		t.Fatalf("AllMessages: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("AllMessages = %d rows, want 3", len(all))
	}
	seen := map[string]bool{}
	for _, m := range all {
		seen[m.SessionID+"/"+m.Content] = true
	}
	if !seen["s1/one"] || !seen["s2/other"] {
		t.Errorf("AllMessages rows = %+v, missing expected content", all)
	}
}

func TestResolveMessageUUID(t *testing.T) {
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "s"})
	id1 := storetest.InsertMessage(t, con, storetest.Message{SessionID: "s", Role: "user", Content: "a", UUID: "aabbccdd-0001"})
	storetest.InsertMessage(t, con, storetest.Message{SessionID: "s", Role: "user", Content: "b", UUID: "aabbccdd-0002"})
	storetest.InsertMessage(t, con, storetest.Message{SessionID: "s", Role: "user", Content: "c", UUID: "eeff0011-0001"})
	storetest.InsertSession(t, con, storetest.Session{ID: "other"})
	storetest.InsertMessage(t, con, storetest.Message{SessionID: "other", Role: "user", Content: "d", UUID: "aabbccdd-9999"})

	// Unique within the session.
	ids, err := store.ResolveMessageUUID(con, "s", "eeff0011", 2)
	if err != nil || len(ids) != 1 {
		t.Fatalf("ResolveMessageUUID unique = %v (%v), want 1 id", ids, err)
	}

	// Ambiguous prefix, limit 2: exactly 2 ids in id order — the caller's
	// git-style guard (0=miss, 1=hit, 2=ambiguous) keys off this cap.
	ids, err = store.ResolveMessageUUID(con, "s", "aabbccdd", 2)
	if err != nil || len(ids) != 2 || ids[0] != id1 {
		t.Errorf("ResolveMessageUUID ambiguous = %v (%v), want 2 ids starting at %d", ids, err, id1)
	}

	// Scoped to the session: the other session's aabbccdd row never leaks in.
	ids, _ = store.ResolveMessageUUID(con, "s", "aabbccdd-9999", 2)
	if len(ids) != 0 {
		t.Errorf("ResolveMessageUUID cross-session = %v, want none", ids)
	}

	// Miss.
	if ids, _ := store.ResolveMessageUUID(con, "s", "zzzz", 2); len(ids) != 0 {
		t.Errorf("ResolveMessageUUID miss = %v, want none", ids)
	}
}

func TestMessageUUIDAndMeta(t *testing.T) {
	con, _ := storetest.NewDB(t)
	storetest.InsertSession(t, con, storetest.Session{ID: "s", ParentID: "root", IsSubagent: true})
	storetest.InsertSession(t, con, storetest.Session{ID: "root"})
	mid := storetest.InsertMessage(t, con, storetest.Message{SessionID: "s", Role: "user", Content: "x", ISO: "2026-06-01T10:00:00Z", UUID: "uuid-x"})

	if got := store.MessageUUID(con, mid); got != "uuid-x" {
		t.Errorf("MessageUUID = %q, want uuid-x", got)
	}
	if got := store.MessageUUID(con, 999999); got != "" {
		t.Errorf("MessageUUID(missing) = %q, want empty", got)
	}

	iso, parent, isSub, missing, ok := store.MessageMeta(con, mid)
	if !ok || iso != "2026-06-01T10:00:00Z" || parent != "root" || !isSub || missing != 0 {
		t.Errorf("MessageMeta = (%q,%q,%v,%v,%v), want (iso,root,true,0,true)", iso, parent, isSub, missing, ok)
	}

	// missing_since (durable retention watermark) surfaces through the join.
	storetest.SetSessionField(t, con, "s", "missing_since", 42.5)
	if _, _, _, missing, ok := store.MessageMeta(con, mid); !ok || missing != 42.5 {
		t.Errorf("MessageMeta missing_since = (%v,%v), want (42.5,true)", missing, ok)
	}

	// A churned/gone rowid reads as ok=false.
	if _, _, _, _, ok := store.MessageMeta(con, 999999); ok {
		t.Error("MessageMeta(missing) ok = true, want false")
	}
}
