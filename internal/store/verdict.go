// Session-verdict surface: the per-session `routine` verdict + its
// floor|agent source, kept in the topic sidecar (session_verdict, gated by
// TopicSchemaVersion). A verdict is a surfacing signal only — never a truth or
// importance claim on the transcript (raw stays raw). Downstream consumers use
// it for sort-tiering and for the routine-fed delete plan.
package store

import (
	"database/sql"
	"fmt"
)

// Verdict source values. `floor` = the deterministic math floor; `agent` =
// an LLM tagger's explicit call. `source` is load-bearing for the
// provenance-gated delete plan.
const (
	VerdictSourceFloor = "floor"
	VerdictSourceAgent = "agent"
)

// VerdictRoutine is the only verdict kind today: the session is routine (trivial /
// low-signal). Kept as a named value rather than a bool so the column can carry
// future verdicts without a schema change (one schema).
const VerdictRoutine = "routine"

// Verdict is one session's verdict row.
type Verdict struct {
	SessionID     string
	Verdict       string
	Source        string
	OriginMachine string
	TaggedAt      float64
}

// UpsertVerdict writes a session's verdict — the AUTHORING path (local floor
// write / local tag-write --routine). The local author's intent wins
// unconditionally; origin_machine stamps who wrote it. The cross-machine INGEST
// path is MergeVerdict (tagged_at-LWW), the author-vs-replicate split mirrored
// from the segment path.
func UpsertVerdict(con *sql.DB, v Verdict) error {
	if v.Verdict == "" || v.Source == "" {
		return fmt.Errorf("verdict and source are required (session %s)", v.SessionID)
	}
	_, err := con.Exec(`
INSERT INTO session_verdict(session_id, verdict, source, origin_machine, tagged_at)
VALUES(?,?,?,?,?)
ON CONFLICT(session_id) DO UPDATE SET
  verdict=excluded.verdict, source=excluded.source,
  origin_machine=excluded.origin_machine, tagged_at=excluded.tagged_at`,
		v.SessionID, v.Verdict, v.Source, v.OriginMachine, v.TaggedAt)
	if err != nil {
		return fmt.Errorf("upsert verdict: %w", err)
	}
	return nil
}

// MergeVerdict is the cross-machine INGEST path for a verdict. Unlike segments
// (resolved by provenance authority), the verdict tie-break is FIXED by design —
// "verdict-vs-verdict tie = latest tagged_at wins" —
// so it is wall-clock LWW by design, not by default. This is low-stakes precisely
// because the only verdict kind is `routine`: when two machines both mark a
// session routine, the outcome is identical and the tie-break only selects which
// source/origin attribution to keep. The incoming row wins if strictly newer, or
// ties with a lexicographically-higher origin_machine (deterministic, skew is
// immaterial when both verdicts agree). Idempotent: an equal-or-older row is a
// no-op.
//
// The cross-KIND rule — a real topic tag beats routine — is NOT applied here
// (destructively at ingest); it is resolved at READ time by IsEffectivelyRoutine,
// so it is order-independent and reversible by re-tag.
func MergeVerdict(con *sql.DB, v Verdict) error {
	if v.Verdict == "" || v.Source == "" {
		return fmt.Errorf("verdict and source are required (session %s)", v.SessionID)
	}
	_, err := con.Exec(`
INSERT INTO session_verdict(session_id, verdict, source, origin_machine, tagged_at)
VALUES(?,?,?,?,?)
ON CONFLICT(session_id) DO UPDATE SET
  verdict=excluded.verdict, source=excluded.source,
  origin_machine=excluded.origin_machine, tagged_at=excluded.tagged_at
WHERE excluded.tagged_at > session_verdict.tagged_at
   OR (excluded.tagged_at = session_verdict.tagged_at
       AND excluded.origin_machine > session_verdict.origin_machine)`,
		v.SessionID, v.Verdict, v.Source, v.OriginMachine, v.TaggedAt)
	if err != nil {
		return fmt.Errorf("merge verdict: %w", err)
	}
	return nil
}

// VerdictFor returns a session's verdict row, ok=false when it has none. A missing
// table reads as "no verdict" (non-fatal).
func VerdictFor(con *sql.DB, sessionID string) (Verdict, bool, error) {
	var (
		v      Verdict
		src    sql.NullString
		origin sql.NullString
		at     sql.NullFloat64
	)
	err := con.QueryRow(
		"SELECT session_id, verdict, source, origin_machine, tagged_at FROM session_verdict WHERE session_id=?",
		sessionID,
	).Scan(&v.SessionID, &v.Verdict, &src, &origin, &at)
	if err == sql.ErrNoRows {
		return Verdict{}, false, nil
	}
	if err != nil {
		return Verdict{}, false, nil // missing table / read error reads as no verdict
	}
	v.Source = src.String
	v.OriginMachine = origin.String
	v.TaggedAt = at.Float64
	return v, true, nil
}

// IsEffectivelyRoutine resolves the cross-kind rule at read time: a session is
// effectively routine iff it carries a `routine` verdict AND has no real topic
// segment. "A real tag beats routine" — someone bothered to tag it, so it is not
// noise. Non-destructive and reversible: adding a real segment silently demotes
// the routine verdict without touching it; re-tagging reverses. The sort-tier
// surfacing reads exactly this.
func IsEffectivelyRoutine(con *sql.DB, sessionID string) (bool, error) {
	v, ok, err := VerdictFor(con, sessionID)
	if err != nil || !ok || v.Verdict != VerdictRoutine {
		return false, err
	}
	real, err := SessionHasRealSegments(con, sessionID)
	if err != nil {
		return false, err
	}
	return !real, nil
}
