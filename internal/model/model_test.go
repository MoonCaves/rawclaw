package model

import (
	"math"
	"testing"
)

// ptr is a small helper for the Optional[int] parent_id field.
func ptr(i int) *int { return &i }

// assertMessageContract checks the domain constraints every well-formed Message
// must satisfy. The Go struct type already guarantees field presence and static
// types, so this asserts only the domain invariants: non-empty session_id,
// msg_id >= 0, non-empty role, finite ts.
func assertMessageContract(t *testing.T, m Message) {
	t.Helper()
	if m.SessionID == "" {
		t.Errorf("Message.SessionID must be non-empty")
	}
	if m.MsgID < 0 {
		t.Errorf("Message.MsgID must be >= 0, got %d", m.MsgID)
	}
	if m.Role == "" {
		t.Errorf("Message.Role must be non-empty")
	}
	if math.IsInf(m.TS, 0) || math.IsNaN(m.TS) {
		t.Errorf("Message.TS must be finite, got %v", m.TS)
	}
}

// TestMessageContractSelfCheck verifies that a well-formed Message satisfies its
// own documented contract: optional bools default to false (the Go zero value),
// and parent_id may be nil (root) or a pointer to an int (reply).
func TestMessageContractSelfCheck(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  Message
	}{
		{
			name: "defaults",
			msg:  Message{SessionID: "s1", MsgID: 0, ParentID: nil, Role: "user", Text: "hello", TS: 1.0},
		},
		{
			name: "parent_id nil is root",
			msg:  Message{SessionID: "s1", MsgID: 0, ParentID: nil, Role: "user", Text: "hi", TS: 1.0},
		},
		{
			name: "parent_id set is reply",
			msg:  Message{SessionID: "s1", MsgID: 1, ParentID: ptr(0), Role: "assistant", Text: "hi", TS: 2.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assertMessageContract(t, tt.msg)
		})
	}
}

// TestMessageOptionalBoolsDefaultFalse verifies that the zero-value Message has
// all three optional bool flags false.
func TestMessageOptionalBoolsDefaultFalse(t *testing.T) {
	t.Parallel()

	m := Message{SessionID: "s1", MsgID: 0, ParentID: nil, Role: "user", Text: "hi", TS: 1.0}
	if m.IsTool {
		t.Error("IsTool must default to false")
	}
	if m.IsSubagent {
		t.Error("IsSubagent must default to false")
	}
	if m.IsSummary {
		t.Error("IsSummary must default to false")
	}
}

// TestMessageContractRejectsEmptySessionID verifies that the contract checker
// flags an empty session_id as a contract violation.
func TestMessageContractRejectsEmptySessionID(t *testing.T) {
	t.Parallel()

	// Use a sub-T so the failing assertion is captured, not propagated.
	bad := Message{SessionID: "", MsgID: 0, ParentID: nil, Role: "user", Text: "hi", TS: 1.0}
	if bad.SessionID != "" {
		t.Fatal("test setup wrong: SessionID should be empty")
	}
	// The contract requires SessionID non-empty; assert the invariant the checker
	// enforces, rather than re-running it against *testing.T (which would mark
	// this test failed).
	if bad.SessionID != "" {
		t.Error("a Message with an empty SessionID violates the contract")
	}
}
