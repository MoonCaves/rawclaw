package source

import (
	"os"
	"slices"
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

func TestResumeCommandQuotesAllArgvWords(t *testing.T) {
	for _, tc := range []struct {
		name       string
		executable string
		sessionID  string
		want       string
	}{
		{"empty executable", "", "S", `'' '--resume' 'S'`},
		{"executable path with spaces", "/path/with spaces/claude", "S", `'/path/with spaces/claude' '--resume' 'S'`},
		{"apostrophe and bang", "it's!", "S", `'it'\''s'\!'' '--resume' 'S'`},
		{"reserved word", "if", "S", `'if' '--resume' 'S'`},
		{"assignment word", "NAME=value", "S", `'NAME=value' '--resume' 'S'`},
		{"argument-like path", "/tmp/--resume", "S", `'/tmp/--resume' '--resume' 'S'`},
		{"session path with spaces", "claude", "session with spaces", `'claude' '--resume' 'session with spaces'`},
		{"session shell syntax", "claude", "$(id)", `'claude' '--resume' '$(id)'`},
		{"empty session id", "claude", "", `'claude' '--resume' ''`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CLI_NAME", tc.executable)
			if got := ResumeCommand("claude", tc.sessionID, ""); got != tc.want {
				t.Errorf("ResumeCommand() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResumeArgvOtherSourcesAndUnknownFallback(t *testing.T) {
	t.Setenv("CLAUDE_CLI_NAME", "/custom/claude")
	for _, tc := range []struct {
		source string
		want   []string
	}{
		{"codex", []string{"codex", "resume", "S"}},
		{"antigravity", []string{"agy", "--conversation", "S"}},
		{"goose", []string{"goose", "session", "--resume", "--session-id", "S"}},
		{"pi", []string{"pi", "--session", "S"}},
		{"opencode", []string{"opencode", "--session", "S"}},
		{"hermes", []string{"hermes", "--resume", "S"}},
		{"unknown", []string{"claude", "--resume", "S"}},
	} {
		argv := ResumeArgv(tc.source, "S")
		if !slices.Equal(argv, tc.want) {
			t.Errorf("ResumeArgv(%q) = %#v, want %#v", tc.source, argv, tc.want)
		}
	}
}

func TestResumeCommandHermesQuotesSessionAndCWD(t *testing.T) {
	t.Setenv("CLAUDE_CLI_NAME", "/custom/claude")
	want := "cd '/tmp/project $(id)' && 'hermes' '--resume' '$(id)'"
	if got := ResumeCommand("hermes", "$(id)", "/tmp/project $(id)"); got != want {
		t.Errorf("ResumeCommand(hermes) = %q, want %q", got, want)
	}
}

// Lifted from openclaw/openclaw src/agents/cli-runner/helpers.system-prompt-resume.test.ts L53-L69
// @6d7347b2da3f3ea44a85ac6fd7f3c15d9ab2caf8.
// rawclaw: assert only session-level fork and nofork resume behavior; omit checkpoint arguments.
func TestResumeCommandWithForkQuotesAllWordsAndPreservesNoFork(t *testing.T) {
	for _, tc := range []struct {
		name       string
		executable string
		sessionID  string
		cwd        string
		forkResume bool
		want       string
	}{
		{
			name:       "no fork keeps empty executable and hostile values quoted",
			executable: "",
			sessionID:  "$(id)",
			cwd:        "/tmp/project $(id)",
			want:       "cd '/tmp/project $(id)' && '' '--resume' '$(id)'",
		},
		{
			name:       "fork appends one argument after quoted resume values",
			executable: "/path/with spaces/claude",
			sessionID:  "O'Reilly!",
			cwd:        "/tmp/dir's $HOME!",
			forkResume: true,
			want:       "cd '/tmp/dir'\\''s $HOME!' && '/path/with spaces/claude' '--resume' 'O'\\''Reilly'\\!'' '--fork-session'",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CLI_NAME", tc.executable)
			got, err := ResumeCommandWithFork("claude", tc.sessionID, tc.cwd, tc.forkResume)
			if err != nil {
				t.Fatalf("ResumeCommandWithFork() error = %v", err)
			}
			if got != tc.want {
				t.Errorf("ResumeCommandWithFork() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Lifted from openclaw/openclaw src/agents/cli-runner/helpers.system-prompt-resume.test.ts L75-L77
// @6d7347b2da3f3ea44a85ac6fd7f3c15d9ab2caf8.
func TestResumeCommandWithForkRejectsUnsupportedSources(t *testing.T) {
	for _, sourceTool := range []string{"codex", "antigravity", "goose", "pi", "opencode", "hermes", "unknown"} {
		t.Run(sourceTool, func(t *testing.T) {
			got, err := ResumeCommandWithFork(sourceTool, "session", "/project", true)
			if err == nil || err.Error() != "CLI backend does not support forked session resume" {
				t.Fatalf("ResumeCommandWithFork() error = %v, want unsupported-source error", err)
			}
			if got != "" {
				t.Errorf("ResumeCommandWithFork() command = %q, want empty", got)
			}
		})
	}
}

func TestResumeCommandWithForkKeepsUnknownNoForkFallback(t *testing.T) {
	t.Setenv("CLAUDE_CLI_NAME", "/custom/claude")
	got, err := ResumeCommandWithFork("unknown", "session", "", false)
	if err != nil {
		t.Fatalf("ResumeCommandWithFork() error = %v", err)
	}
	if want := "'claude' '--resume' 'session'"; got != want {
		t.Errorf("ResumeCommandWithFork(unknown, false) = %q, want %q", got, want)
	}
}
