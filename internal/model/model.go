// Package model defines the normalized transcript message a Source adapter
// yields for the index to ingest — the ingest-side data contract, parallel to
// internal/embed's vector ports (defined ahead of its adapters, ship-empty).
//
// The fields mirror one row the index persists into the messages table
// (role, content, ts, ts_iso, uuid) exactly, so an adapter's output maps to a
// row with no lossy translation. Session-level attributes (id, cwd, subagent,
// parent, resume) travel separately on source.Container, not here.
package model

// Message is one normalized transcript message in transcript order. Keep it
// aligned with the messages-table columns the index writes; a new field here
// implies a schema change there.
type Message struct {
	Role  string  // "user" | "assistant" | "system" | "summary"
	Text  string  // flattened, searchable content (tool/thinking markers baked in)
	TS    float64 // epoch seconds, 0 if the record carried no parseable timestamp
	TSISO string  // the original ISO-8601 timestamp string, "" if none
	UUID  string  // stable per-message id the source assigns (drives <session8>:<uuid8> refs)
}
