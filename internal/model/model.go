// Package model defines the ingest PORT — the data contract a pluggable Source
// adapter yields, for the planned bring-your-own-runtime feature. It is the
// ingest-side parallel to embed.VectorStore (the vector port): defined ahead of
// its adapters, ship-empty. No Source adapter is wired yet — the built-in
// Claude-Code-transcript reader uses its own internal types today — so nothing
// imports this package at present; it is the stable contract a future external
// runtime adapter conforms to, not dead code.
package model

// Message is the contract for one transcript message a Source adapter yields for
// the index to ingest. Keep it stable: do not rename or drop a field, so an
// external adapter compiled against it keeps working. Optional bools default to
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
