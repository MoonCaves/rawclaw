package cli

import "testing"

// TestGuessArchiveRemote adapts chezmoi's guessRepoURL table to rawclaw's rules:
// SSH-form output, a bare username defaulting to rawclaw-transcripts, and full
// URLs passing through untouched.
func TestGuessArchiveRemote(t *testing.T) {
	for _, tc := range []struct {
		arg, want string
	}{
		// bare username → github + default repo
		{"you", "git@github.com:you/rawclaw-transcripts.git"},
		// user/repo → github
		{"you/notes", "git@github.com:you/notes.git"},
		{"you/notes.git", "git@github.com:you/notes.git"},
		{"user/.dotfiles", "git@github.com:user/.dotfiles.git"},
		// host/user → host + default repo (dot in first segment routes here, not user/repo)
		{"gitlab.com/you", "git@gitlab.com:you/rawclaw-transcripts.git"},
		{"codeberg.org/you", "git@codeberg.org:you/rawclaw-transcripts.git"},
		// host/user/repo
		{"gitlab.com/you/notes", "git@gitlab.com:you/notes.git"},
		{"gitlab.com/you/notes.git", "git@gitlab.com:you/notes.git"},
		// sourcehut special-case
		{"sr.ht/~you", "git@git.sr.ht:~you/rawclaw-transcripts"},
		{"sr.ht/~you/notes", "git@git.sr.ht:~you/notes"},
		// full URLs pass through unchanged — the critical zero-behavior-change property
		{"git@github.com:you/notes.git", "git@github.com:you/notes.git"},
		{"https://gitlab.com/you/notes.git", "https://gitlab.com/you/notes.git"},
		{"ssh://git@example.com:22/you/notes.git", "ssh://git@example.com:22/you/notes.git"},
		{"github.com:you/notes.git", "github.com:you/notes.git"}, // scp-style: `:` blocks every rule → passthrough
		// uppercase + dotted repo name still resolves
		{"You/My.Notes", "git@github.com:You/My.Notes.git"},
		// degenerate inputs pass through untouched (init then fails at clone with a clear git error)
		{"you/", "you/"},
		{"/notes", "/notes"},
		{"", ""},
		{"a/b/c/d", "a/b/c/d"},
	} {
		if got := guessArchiveRemote(tc.arg); got != tc.want {
			t.Errorf("guessArchiveRemote(%q) = %q, want %q", tc.arg, got, tc.want)
		}
	}
}
