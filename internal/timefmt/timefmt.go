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

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	// DateLayout renders and parses a date-only string (YYYY-MM-DD).
	DateLayout = "2006-01-02"
	// utcLayout renders a full instant: RFC3339 at seconds precision, always
	// with the explicit "Z" marker (the time is converted to UTC first).
	utcLayout = "2006-01-02T15:04:05Z"
	// utcClockLayout renders a bare wall-clock with the same explicit marker.
	utcClockLayout = "15:04:05Z"
	// utcShortLayout renders a date and minute-precision time, still with the
	// explicit marker. For scan-dense listings (search hit headers) where the
	// full RFC3339 instant costs more line width than the seconds are worth.
	// Dropping the marker instead of the seconds was rejected: an unmarked
	// stamp is the ambiguity this package exists to prevent.
	utcShortLayout = "2006-01-02 15:04Z"
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

// UTCShort renders t as a marked-UTC date and minute ("2026-01-02 15:04Z") —
// the compact form for dense listings that still must not be ambiguous.
func UTCShort(t time.Time) string { return t.UTC().Format(utcShortLayout) }

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

// UTCShortFromISO normalizes a stored ISO timestamp to the compact marked-UTC
// display form ("2026-01-02 15:04Z"). Like UTCFromISO, an empty or unparseable
// input is returned verbatim rather than guessed at.
func UTCShortFromISO(iso string) string {
	if iso == "" {
		return iso
	}
	for _, layout := range isoLayouts {
		if t, err := time.Parse(layout, iso); err == nil {
			return UTCShort(t)
		}
	}
	return iso
}

// ParseDateFilter normalizes a human or agent date string into a standard YYYY-MM-DD date.
// Accepts:
//   - Standard ISO dates: "2006-01-02", RFC3339 ("2006-01-02T15:04:05Z") via time.Parse
//   - Relative day keywords: "today", "yesterday", "now"
//   - Standard durations: "-24h", "-168h", "72h" via time.ParseDuration
//   - Day/week shorthand: "-7d", "7d", "-1w", "1w" (translated to hours for time.ParseDuration)
func ParseDateFilter(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Try standard ISO formats first via time.Parse
	if t, err := time.Parse(DateLayout, s); err == nil {
		return t.UTC().Format(DateLayout)
	}
	for _, layout := range isoLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format(DateLayout)
		}
	}

	lower := strings.ToLower(s)
	now := time.Now().UTC()
	switch lower {
	case "today", "now":
		return now.Format(DateLayout)
	case "yesterday":
		return now.AddDate(0, 0, -1).Format(DateLayout)
	}

	// Translate day/week shorthand to Go standard duration format for time.ParseDuration
	durStr := lower
	if strings.HasSuffix(durStr, "d") {
		numStr := strings.TrimSuffix(strings.TrimPrefix(durStr, "-"), "d")
		if n, err := strconv.Atoi(numStr); err == nil {
			durStr = fmt.Sprintf("-%dh", n*24)
		}
	} else if strings.HasSuffix(durStr, "w") {
		numStr := strings.TrimSuffix(strings.TrimPrefix(durStr, "-"), "w")
		if n, err := strconv.Atoi(numStr); err == nil {
			durStr = fmt.Sprintf("-%dh", n*168)
		}
	} else if !strings.HasPrefix(durStr, "-") && !strings.HasPrefix(durStr, "+") {
		durStr = "-" + durStr
	}

	if d, err := time.ParseDuration(durStr); err == nil {
		return now.Add(d).Format(DateLayout)
	}

	return s
}
