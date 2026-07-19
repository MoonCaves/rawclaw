// Package timefmt is the single seam for user-facing time rendering. Every
// surface that prints a wall-clock instant formats it here, so the policy
// lives in one place instead of being re-decided per call site:
//
//   - AGENT-PARSED surfaces (search results, the outline header, live list +
//     stream, any --json payload): UTC with an explicit marker — RFC3339 "Z"
//     for full instants, "HH:MM:SSZ" for bare clocks. An agent must never have
//     to guess the zone of a timestamp it is about to reason over.
//   - HUMAN browse tables and `archive status`: local time with the zone
//     abbreviation spelled out (e.g. "2026-01-02 15:04 PST") — friendly to
//     read, still unambiguous.
//
// JSON payloads that carry a source-recorded ISO string verbatim (message
// ts_iso from the transcript record) already satisfy the marked-UTC policy at
// the source and are passed through unchanged; UTCFromISO is the display-side
// normalizer for those strings.
package timefmt

import "time"

const (
	// utcLayout renders a full instant: RFC3339 at seconds precision, always
	// with the explicit "Z" marker (the time is converted to UTC first).
	utcLayout = "2006-01-02T15:04:05Z"
	// utcClockLayout renders a bare wall-clock with the same explicit marker.
	utcClockLayout = "15:04:05Z"
	// localLayout renders a human table stamp: minute precision plus the zone
	// abbreviation, so a local time never reads as an unmarked/ambiguous one.
	localLayout = "2006-01-02 15:04 MST"
)

// UTC renders t as a marked-UTC RFC3339 instant ("2026-01-02T15:04:05Z") —
// the format for agent-parsed surfaces.
func UTC(t time.Time) string { return t.UTC().Format(utcLayout) }

// UTCClock renders t's wall-clock as marked UTC ("15:04:05Z") — for compact
// per-message clocks on agent-parsed surfaces (live stream).
func UTCClock(t time.Time) string { return t.UTC().Format(utcClockLayout) }

// Local renders t in local time with the zone abbreviation
// ("2026-01-02 15:04 PST") — the format for human browse tables and
// `archive status`.
func Local(t time.Time) string { return t.Local().Format(localLayout) }

// isoLayouts are the source timestamp shapes UTCFromISO accepts, tried in
// order. RFC3339Nano covers the transcript record's native form (fractional
// seconds, "Z" or numeric offset); the zoneless forms cover legacy index rows
// written before this seam existed and are taken as UTC (best-effort: an
// unzoned stamp has no recoverable zone, and UTC is the corpus default).
var isoLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
}

// UTCFromISO normalizes a stored ISO timestamp string to the marked-UTC
// display form ("2026-01-02T15:04:05Z"). An empty or unparseable input is
// returned verbatim — the seam never invents a time it cannot read.
func UTCFromISO(iso string) string {
	if iso == "" {
		return iso
	}
	for _, layout := range isoLayouts {
		if t, err := time.Parse(layout, iso); err == nil {
			return UTC(t)
		}
	}
	return iso
}
