package source

import (
	"os"
	"testing"
)

func TestResumeArgvClaudeExecutableConfiguration(t *testing.T) {
	t.Setenv("CLAUDE_CLI_NAME", "sentinel")
	if err := os.Unsetenv("CLAUDE_CLI_NAME"); err != nil {
		t.Fatal(err)
	}
	if got := ResumeArgv("claude", "S"); len(got) != 3 || got[0] != "claude" || got[1] != "--resume" || got[2] != "S" {
		t.Errorf("Claude ResumeArgv() with unset env = %#v, want default executable and resume args", got)
	}

	for _, tc := range []struct {
		name  string
		value string
	}{
		{"explicit empty", ""},
		{"path with spaces", "/path/with spaces/claude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CLI_NAME", tc.value)
			argv := ResumeArgv("claude", "S")
			if len(argv) != 3 || argv[0] != tc.value || argv[1] != "--resume" || argv[2] != "S" {
				t.Errorf("Claude ResumeArgv() = %#v, want executable preserved as one argv value", argv)
			}
		})
	}
}

func TestResumeCommandQuotesExecutableWord(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{"empty", "", `'' --resume S`},
		{"space", "/path/with spaces/claude", `'/path/with spaces/claude' --resume S`},
		{"apostrophe and bang", "it's!", `'it'\''s'\!'' --resume S`},
		{"reserved word", "if", `'if' --resume S`},
		{"assignment word", "NAME=value", `'NAME=value' --resume S`},
		{"argument-like path", "/tmp/--resume", `'/tmp/--resume' --resume S`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CLI_NAME", tc.value)
			if got := ResumeCommand("claude", "S", ""); got != tc.want {
				t.Errorf("ResumeCommand() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResumeArgvOtherSourcesAndUnknownFallback(t *testing.T) {
	t.Setenv("CLAUDE_CLI_NAME", "/custom/claude")
	for _, tc := range []struct {
		source string
		want   string
	}{
		{"codex", "codex"},
		{"antigravity", "agy"},
		{"goose", "goose"},
		{"pi", "pi"},
		{"opencode", "opencode"},
		{"unknown", "claude"},
	} {
		argv := ResumeArgv(tc.source, "S")
		if len(argv) == 0 || argv[0] != tc.want {
			t.Errorf("ResumeArgv(%q) = %#v, want executable %q", tc.source, argv, tc.want)
		}
	}
}
