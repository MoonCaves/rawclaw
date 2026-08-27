package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeCloseoutConfig(t *testing.T, argv []string) {
	t.Helper()
	path := filepath.Join(os.Getenv("HOME"), ".cache", "session-search", "tagger-config.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(closeoutTaggerConfig{Argv: argv})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunCloseout_MissingConfigPrintsManualRecovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sid := "11111111-2222-3333-4444-555555555555"

	var out bytes.Buffer
	if err := runCloseout(&out, sid); err != nil {
		t.Fatalf("runCloseout: %v", err)
	}
	if got, want := out.String(), "rawclaw tag-prep "+sid+"\n"; got != want {
		t.Fatalf("recovery = %q, want %q", got, want)
	}
}

func TestRunCloseout_LaunchesOncePerSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeCloseoutConfig(t, []string{"/absolute/tagger"})
	sid := "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	oldSpawn := spawnCloseout
	oldNow := closeoutNow
	t.Cleanup(func() {
		spawnCloseout = oldSpawn
		closeoutNow = oldNow
	})
	var launches []string
	spawnCloseout = func(sessionID string) error {
		launches = append(launches, sessionID)
		return nil
	}
	closeoutNow = func() time.Time { return time.Unix(100, 0) }

	var first, second bytes.Buffer
	if err := runCloseout(&first, sid); err != nil {
		t.Fatalf("first runCloseout: %v", err)
	}
	if err := runCloseout(&second, sid); err != nil {
		t.Fatalf("second runCloseout: %v", err)
	}
	if len(launches) != 1 || launches[0] != sid {
		t.Fatalf("launches = %v, want one launch for %s", launches, sid)
	}
	if !strings.Contains(second.String(), "already queued") {
		t.Fatalf("duplicate output = %q", second.String())
	}
}

func TestRunCloseout_RequiresFullSessionID(t *testing.T) {
	var out bytes.Buffer
	if err := runCloseout(&out, "deadbeef"); err == nil {
		t.Fatal("runCloseout accepted an 8-character session prefix")
	}
}

func TestRunCloseoutTagger_RejectsFailureAndMalformedOutput(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh available")
	}
	printf, err := exec.LookPath("printf")
	if err != nil {
		t.Skip("no printf available")
	}
	truePath, err := exec.LookPath("true")
	if err != nil {
		t.Skip("no true available")
	}
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{name: "nonzero", argv: []string{sh, "-c", "exit 7"}, want: "unsuccessfully"},
		{name: "malformed", argv: []string{printf, "not-json"}, want: "malformed JSON"},
		{name: "empty", argv: []string{truePath}, want: "empty output"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var log bytes.Buffer
			_, err := runCloseoutTagger(tc.argv, []byte("prep"), &log)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("runCloseoutTagger error = %v, want %q", err, tc.want)
			}
		})
	}
}
