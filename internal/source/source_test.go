package source

import "testing"

// TestResumeCommand_QuotesHostileCWD pins the property that matters: the cwd comes
// out of a transcript and is pasted into a shell, so the emitted command must `cd`
// to the LITERAL directory — never let the shell expand or execute part of the path.
//
// Regression: an earlier fix used strconv.Quote, which emits Go string syntax. Its
// double quotes still permit parameter expansion and command substitution, so
// `cd "/tmp/a $HOME b"` landed in the wrong directory and `cd "/tmp/a ` + "`id`" + ` b"`
// ran a command.
func TestResumeCommand_QuotesHostileCWD(t *testing.T) {
	for _, tc := range []struct {
		name string
		cwd  string
		want string
	}{
		{"plain path stays bare", "/Users/x/proj", "cd /Users/x/proj && claude --resume S"},
		{"space", "/tmp/a b", "cd '/tmp/a b' && claude --resume S"},
		{"dollar is not expanded", "/tmp/a $HOME b", "cd '/tmp/a $HOME b' && claude --resume S"},
		{"backtick is not executed", "/tmp/a `id` b", "cd '/tmp/a `id` b' && claude --resume S"},
		{"dollar-paren is not executed", "/tmp/a $(id) b", "cd '/tmp/a $(id) b' && claude --resume S"},
		{"backslash is literal", `/tmp/a\b`, `cd '/tmp/a\b' && claude --resume S`},
		{"double quote", `/tmp/a"b`, `cd '/tmp/a"b' && claude --resume S`},
		{"single quote is closed, escaped, reopened", "/tmp/it's", `cd '/tmp/it'\''s' && claude --resume S`},
		{"semicolon cannot chain a command", "/tmp/a;rm -rf x", "cd '/tmp/a;rm -rf x' && claude --resume S"},
		{"tilde is not home-expanded", "~/proj", "cd '~/proj' && claude --resume S"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResumeCommand("claude", "S", tc.cwd); got != tc.want {
				t.Errorf("ResumeCommand(cwd=%q)\n got %s\nwant %s", tc.cwd, got, tc.want)
			}
		})
	}
}

// TestResumeCommand_PerSourceVerb covers the render-time resume resolution: the verb
// is derived from the source tool, never persisted, so a runtime renaming its flags
// only has to change here.
func TestResumeCommand_PerSourceVerb(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"claude", "claude --resume S"},
		{"codex", "codex resume S"},
		{"antigravity", "agy --conversation S"},
		{"", "claude --resume S"}, // unknown source falls back
	} {
		if got := ResumeCommand(tc.src, "S", ""); got != tc.want {
			t.Errorf("ResumeCommand(src=%q) = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestShellQuote_EmptyString guards the one input where returning the value bare
// would vanish an argument entirely rather than pass an empty one.
func TestShellQuote_EmptyString(t *testing.T) {
	if got := shellQuote(""); got != "''" {
		t.Errorf("shellQuote(%q) = %q, want %q", "", got, "''")
	}
}
