package live

import (
	"bytes"
	"encoding/json"
	"regexp"
	"testing"
	"time"
)

// utcClockRe matches the marked-UTC per-message clock ("HH:MM:SSZ") the
// timefmt seam renders on the live stream — the explicit Z is the point.
var utcClockRe = regexp.MustCompile(`\[\d{2}:\d{2}:\d{2}Z (user|assistant)\]`)

// utcInstantRe matches a marked-UTC RFC3339 instant at seconds precision.
var utcInstantRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)

// TestServeSession_ClockIsMarkedUTC pins the time-rendering policy for the
// live stream (an agent-parsed surface): every per-message clock carries the
// explicit Z marker, never a bare "HH:MM:SS".
func TestServeSession_ClockIsMarkedUTC(t *testing.T) {
	claudeRoot, _ := newServeHome(t)
	writeClaudeSession(t, claudeRoot, "-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/proj-a", time.Now().Add(-5*time.Second), "question", "answer")

	var buf bytes.Buffer
	if err := ServeSession(&buf, "aaaa1111", 0, false, false); err != nil {
		t.Fatalf("ServeSession: %v", err)
	}
	out := buf.String()
	if !utcClockRe.MatchString(out) {
		t.Errorf("live stream clock is not marked UTC (want [HH:MM:SSZ role]):\n%s", out)
	}
	// The unmarked form must be gone: any clock present must be the Z form.
	if bare := regexp.MustCompile(`\[\d{2}:\d{2}:\d{2} (user|assistant)\]`); bare.MatchString(out) {
		t.Errorf("live stream still renders an unmarked clock:\n%s", out)
	}
}

// TestServeSession_JSONLastActivityMarkedUTC pins the policy for the --json
// shape: last_activity is a marked-UTC RFC3339 instant.
func TestServeSession_JSONLastActivityMarkedUTC(t *testing.T) {
	claudeRoot, _ := newServeHome(t)
	writeClaudeSession(t, claudeRoot, "-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/proj-a", time.Now().Add(-5*time.Second), "question", "answer")

	var buf bytes.Buffer
	if err := ServeSession(&buf, "aaaa1111", 0, false, true); err != nil {
		t.Fatalf("ServeSession --json: %v", err)
	}
	var out struct {
		LastActivity string `json:"last_activity"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("session json: %v\n%s", err, buf.String())
	}
	if !utcInstantRe.MatchString(out.LastActivity) {
		t.Errorf("last_activity = %q, want marked-UTC RFC3339 (…Z)", out.LastActivity)
	}
}

// TestLocalSessions_LastActivityMarkedUTC pins the policy for the live list
// (`live --serve` JSON): LastActivity is a marked-UTC RFC3339 instant.
func TestLocalSessions_LastActivityMarkedUTC(t *testing.T) {
	claudeRoot, _ := newServeHome(t)
	writeClaudeSession(t, claudeRoot, "-proj-a", "aaaa1111-0000-0000-0000-000000000001",
		"/home/u/proj-a", time.Now().Add(-5*time.Second), "question")

	rows := localSessions()
	if len(rows) != 1 {
		t.Fatalf("localSessions: got %d rows, want 1", len(rows))
	}
	if !utcInstantRe.MatchString(rows[0].LastActivity) {
		t.Errorf("LastActivity = %q, want marked-UTC RFC3339 (…Z)", rows[0].LastActivity)
	}
}
