package cli

import (
	"testing"
	"time"
)

func TestParseFlexibleDate(t *testing.T) {
	now := time.Now().UTC()
	todayStr := now.Format("2006-01-02")
	yesterdayStr := now.AddDate(0, 0, -1).Format("2006-01-02")
	sevenDaysAgoStr := now.AddDate(0, 0, -7).Format("2006-01-02")

	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"today", todayStr},
		{"TODAY", todayStr},
		{"now", todayStr},
		{"yesterday", yesterdayStr},
		{"-7d", sevenDaysAgoStr},
		{"7d", sevenDaysAgoStr},
		{"-1w", sevenDaysAgoStr},
		{"1w", sevenDaysAgoStr},
		{"2026-05-12", "2026-05-12"},
		{"2026-05-12T15:04:05Z", "2026-05-12"},
	}

	for _, tc := range cases {
		got := parseFlexibleDate(tc.in)
		if got != tc.want {
			t.Errorf("parseFlexibleDate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOptions_NormalizeDates(t *testing.T) {
	now := time.Now().UTC()
	todayStr := now.Format("2006-01-02")
	yesterdayStr := now.AddDate(0, 0, -1).Format("2006-01-02")
	sevenDaysAgoStr := now.AddDate(0, 0, -7).Format("2006-01-02")

	t.Run("until alias for before", func(t *testing.T) {
		o := Options{Until: "2026-08-01"}
		o.normalizeDates()
		if o.Before != "2026-08-01" {
			t.Errorf("want Before=2026-08-01, got %q", o.Before)
		}
	})

	t.Run("days flag", func(t *testing.T) {
		o := Options{Days: 7}
		o.normalizeDates()
		if o.Since != sevenDaysAgoStr {
			t.Errorf("want Since=%s, got %q", sevenDaysAgoStr, o.Since)
		}
	})

	t.Run("today flag", func(t *testing.T) {
		o := Options{Today: true}
		o.normalizeDates()
		if o.Since != todayStr {
			t.Errorf("want Since=%s, got %q", todayStr, o.Since)
		}
	})

	t.Run("yesterday flag", func(t *testing.T) {
		o := Options{Yesterday: true}
		o.normalizeDates()
		if o.Since != yesterdayStr || o.Before != yesterdayStr {
			t.Errorf("want Since=Before=%s, got Since=%q Before=%q", yesterdayStr, o.Since, o.Before)
		}
	})

	t.Run("week flag", func(t *testing.T) {
		o := Options{Week: true}
		o.normalizeDates()
		if o.Since != sevenDaysAgoStr {
			t.Errorf("want Since=%s, got %q", sevenDaysAgoStr, o.Since)
		}
	})
}
