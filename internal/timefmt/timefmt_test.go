package timefmt

import (
	"testing"
	"time"
)

// ref is a fixed instant with a non-UTC zone, so a formatter that forgets the
// UTC conversion is caught (13:30+02:00 == 11:30Z).
var ref = time.Date(2026, 1, 2, 13, 30, 45, 123456789, time.FixedZone("CEST", 2*60*60))

// TestUTC pins the agent-surface policy: RFC3339, seconds precision, explicit Z.
func TestUTC(t *testing.T) {
	t.Parallel()
	if got, want := UTC(ref), "2026-01-02T11:30:45Z"; got != want {
		t.Errorf("UTC() = %q, want %q", got, want)
	}
}

// TestUTCClock pins the bare-clock policy: HH:MM:SS with explicit Z.
func TestUTCClock(t *testing.T) {
	t.Parallel()
	if got, want := UTCClock(ref), "11:30:45Z"; got != want {
		t.Errorf("UTCClock() = %q, want %q", got, want)
	}
}

// TestLocal pins the human-surface policy: local minute stamp WITH a zone
// abbreviation — never an unmarked local time. The exact zone string depends
// on the host, so the assertion formats the expectation from the same instant.
func TestLocal(t *testing.T) {
	t.Parallel()
	want := ref.Local().Format("2006-01-02 15:04 MST")
	if got := Local(ref); got != want {
		t.Errorf("Local() = %q, want %q", got, want)
	}
	// The zone abbreviation must actually be present (a bare "2026-01-02 15:04"
	// would satisfy the layout comparison above only if MST rendered empty).
	if zone := ref.Local().Format("MST"); zone == "" {
		t.Fatal("host zone abbreviation rendered empty — cannot verify marker")
	}
}

// TestUTCFromISO covers the normalizer: native transcript stamps (fractional,
// Z or offset), zoneless legacy stamps (taken as UTC), and the verbatim
// passthrough for empty/garbage input.
func TestUTCFromISO(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"native transcript millis Z", "2026-04-17T10:00:00.123Z", "2026-04-17T10:00:00Z"},
		{"plain rfc3339 Z", "2026-04-17T10:00:00Z", "2026-04-17T10:00:00Z"},
		{"numeric offset converts", "2026-04-17T12:00:00+02:00", "2026-04-17T10:00:00Z"},
		{"zoneless legacy taken as utc", "2026-04-17T10:00:00", "2026-04-17T10:00:00Z"},
		{"space-separated legacy", "2026-04-17 10:00:00", "2026-04-17T10:00:00Z"},
		{"empty passes through", "", ""},
		{"garbage passes through", "not-a-time", "not-a-time"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := UTCFromISO(tc.in); got != tc.want {
				t.Errorf("UTCFromISO(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
