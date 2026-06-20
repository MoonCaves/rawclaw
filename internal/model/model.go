// Package model holds the shared contract types passed between the rawclaw
// ingest, index, retrieve, and view layers. These types are the FROZEN
// contract every other package depends on.
package model

// Message is the shared contract for one transcript message — the unit a Source
// adapter yields and the index ingests.
//
// FROZEN CONTRACT: do not rename or drop a field. Optional bools default to
// their Go zero value (false).
type Message struct {
	SessionID  string
	MsgID      int
	ParentID   *int // nil = root message
	Role       string
	Text       string
	TS         float64
	IsTool     bool
	IsSubagent bool
	IsSummary  bool
}
