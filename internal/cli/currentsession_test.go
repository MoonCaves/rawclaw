package cli

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestCurrentSessionResolution: the flag is the contract, but the env fallback is
// what makes the exclusion actually fire — an agent running `rawclaw "query"` in
// a shell does not know it just typed something, and will not pass a flag. "off"
// is the way back to searching your own live turn.
func TestCurrentSessionResolution(t *testing.T) {
	tests := []struct {
		name      string
		flag      string
		claudeEnv string
		agyEnv    string
		want      string
	}{
		{name: "flag wins over claude", flag: "aaaa1111", claudeEnv: "bbbb2222", want: "aaaa1111"},
		{name: "flag wins over agy", flag: "aaaa1111", agyEnv: "cccc3333", want: "aaaa1111"},
		{name: "flag wins over both", flag: "aaaa1111", claudeEnv: "bbbb2222", agyEnv: "cccc3333", want: "aaaa1111"},
		{name: "claude env fallback", flag: "", claudeEnv: "bbbb2222", want: "bbbb2222"},
		{name: "agy env fallback", flag: "", agyEnv: "cccc3333", want: "cccc3333"},
		{name: "both env vars: claude wins by documented precedence", flag: "", claudeEnv: "bbbb2222", agyEnv: "cccc3333", want: "bbbb2222"},
		{name: "neither", flag: "", want: ""},
		{name: "off disables claude env", flag: "off", claudeEnv: "bbbb2222", want: ""},
		{name: "off disables agy env", flag: "off", agyEnv: "cccc3333", want: ""},
		{name: "off disables both envs", flag: "off", claudeEnv: "bbbb2222", agyEnv: "cccc3333", want: ""},
		{name: "OFF is case-insensitive", flag: "OFF", agyEnv: "cccc3333", want: ""},
		{name: "whitespace is not a session", flag: "  ", claudeEnv: "  ", agyEnv: "  ", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CODE_SESSION_ID", tc.claudeEnv)
			t.Setenv("ANTIGRAVITY_CONVERSATION_ID", tc.agyEnv)
			o := &Options{CurrentSession: tc.flag}
			if got := o.currentSession(); got != tc.want {
				t.Errorf("currentSession() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestCurrentSessionMultipleEnvsWarning verifies that when multiple runtime env
// vars are present, a structured warning is emitted with the chosen and ignored
// variables and the first in documented order is chosen.
func TestCurrentSessionMultipleEnvsWarning(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "claude-session-123")
	t.Setenv("ANTIGRAVITY_CONVERSATION_ID", "agy-session-456")

	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	orig := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(orig)

	o := &Options{}
	got := o.currentSession()
	if got != "claude-session-123" {
		t.Fatalf("currentSession() = %q, want %q", got, "claude-session-123")
	}

	out := buf.String()
	if !strings.Contains(out, "multiple session environment variables set; using documented precedence") {
		t.Errorf("expected warning log missing from output: %s", out)
	}
	if !strings.Contains(out, `"chosen_env":"CLAUDE_CODE_SESSION_ID"`) ||
		!strings.Contains(out, `"chosen_session":"claude-session-123"`) ||
		!strings.Contains(out, `"ignored_envs":"ANTIGRAVITY_CONVERSATION_ID"`) {
		t.Errorf("expected warning attributes missing from output: %s", out)
	}
}
