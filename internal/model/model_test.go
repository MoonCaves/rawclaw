package model

import (
	"math"
	"testing"
)

// assertMessageContract checks the domain invariants every well-formed Message
// must satisfy. The struct type already guarantees field presence and static
// types, so this asserts only the domain rules: a non-empty role and a finite
// timestamp. Text/TSISO/UUID may legitimately be empty for some records, so they
// are not constrained here.
func assertMessageContract(t *testing.T, m Message) {
	t.Helper()
	if m.Role == "" {
		t.Errorf("Message.Role must be non-empty")
	}
	if math.IsInf(m.TS, 0) || math.IsNaN(m.TS) {
		t.Errorf("Message.TS must be finite, got %v", m.TS)
	}
}

// TestMessageContractSelfCheck verifies a well-formed Message — as a Source
// adapter yields it — satisfies its documented contract and maps cleanly onto the
// messages-table columns (role, content, ts, ts_iso, uuid).
func TestMessageContractSelfCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  Message
	}{
		{
			name: "user with iso + uuid",
			msg:  Message{Role: "user", Text: "hello", TS: 1.0, TSISO: "2026-07-15T00:00:00Z", UUID: "u1"},
		},
		{
			name: "assistant",
			msg:  Message{Role: "assistant", Text: "hi", TS: 2.0, TSISO: "2026-07-15T00:00:01Z", UUID: "u2"},
		},
		{
			name: "timestampless record (ts zero, no iso) is still valid",
			msg:  Message{Role: "summary", Text: "recap", TS: 0, TSISO: "", UUID: ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertMessageContract(t, tt.msg)
		})
	}
}

// TestMessageZeroValueEmptyStrings verifies the zero-value Message has empty
// string fields and a zero timestamp — the "nothing set yet" state an adapter
// starts from before populating a record.
func TestMessageZeroValueEmptyStrings(t *testing.T) {
	t.Parallel()

	var m Message
	if m.Role != "" || m.Text != "" || m.TSISO != "" || m.UUID != "" {
		t.Errorf("zero-value Message must have empty string fields, got %+v", m)
	}
	if m.TS != 0 {
		t.Errorf("zero-value Message.TS must be 0, got %v", m.TS)
	}
}
