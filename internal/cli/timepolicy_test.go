package cli

import (
	"regexp"
	"testing"
	"time"
)

// localStampRe matches the human-surface stamp policy: local minute precision
// WITH a zone abbreviation (letters or a numeric offset like +0530 on hosts
// without a named zone) — never a bare "YYYY-MM-DD HH:MM".
var localStampRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2} [A-Za-z0-9+-]+$`)

// TestStampLabel pins the `archive status` time policy through the timefmt
// seam: zero time reads "never"; a real stamp is local time with the zone
// marker present.
func TestStampLabel(t *testing.T) {
	t.Parallel()
	if got := stampLabel(time.Time{}); got != "never" {
		t.Errorf("stampLabel(zero) = %q, want %q", got, "never")
	}

	ref := time.Date(2026, 1, 2, 13, 30, 45, 0, time.UTC)
	got := stampLabel(ref)
	if !localStampRe.MatchString(got) {
		t.Errorf("stampLabel = %q, want local minute stamp with zone marker", got)
	}
	// The rendered instant must be ref in local time — not an unmarked copy of
	// the UTC clock on hosts west/east of UTC.
	if want := ref.Local().Format("2006-01-02 15:04 MST"); got != want {
		t.Errorf("stampLabel = %q, want %q", got, want)
	}
}
