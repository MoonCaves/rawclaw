package cli

import "regexp"

// defaultArchiveRepo is the repo name a bare-username shorthand assumes
// (`rawclaw archive init you` → git@github.com:you/rawclaw-transcripts.git). It
// is distinctive on purpose — unlikely to collide with a repo the user already
// has — unlike a generic name like "transcripts".
const defaultArchiveRepo = "rawclaw-transcripts"

// repoGuesses expands a shorthand argument to `archive init` into a full git
// remote. Adapted from chezmoi's guessRepoURL table (MIT) — the same idea, with
// two deliberate rawclaw changes: every guess resolves to the SSH form (rawclaw
// runs git with credential prompts disabled, so an unattended HTTPS-with-password
// clone can't work — only SSH keys do), and a bare username defaults the repo to
// defaultArchiveRepo rather than chezmoi's "dotfiles". Ordering matters: the
// two-segment "user/repo" rule (first segment forbids a dot) precedes the
// "host/user" rule (first segment allows a dot), so `user/repo` reads as a
// GitHub repo while `gitlab.com/user` reads as a host + user. Anything that
// already looks like a full URL (contains `@`, `:`, or `//`) matches no rule and
// falls through unchanged.
var repoGuesses = []struct {
	rx   *regexp.Regexp
	repl string
}{
	{regexp.MustCompile(`\A([-0-9A-Za-z]+)\z`), "git@github.com:$1/" + defaultArchiveRepo + ".git"},
	{regexp.MustCompile(`\A([-0-9A-Za-z]+)/([-.0-9A-Z_a-z]+?)(\.git)?\z`), "git@github.com:$1/$2.git"},
	{regexp.MustCompile(`\A([-.0-9A-Za-z]+)/([-0-9A-Za-z]+)\z`), "git@$1:$2/" + defaultArchiveRepo + ".git"},
	{regexp.MustCompile(`\A([-.0-9A-Za-z]+)/([-0-9A-Za-z]+)/([-.0-9A-Za-z]+?)(\.git)?\z`), "git@$1:$2/$3.git"},
	{regexp.MustCompile(`\Asr\.ht/~([a-z_][a-z0-9_-]+)\z`), "git@git.sr.ht:~$1/" + defaultArchiveRepo},
	{regexp.MustCompile(`\Asr\.ht/~([a-z_][a-z0-9_-]+)/([-0-9A-Za-z]+)\z`), "git@git.sr.ht:~$1/$2"},
}

// guessArchiveRemote expands a shorthand into a full SSH git remote, or returns
// arg unchanged when it matches no shorthand (a full URL — `git@host:...`,
// `https://...`, `ssh://...` — always falls through untouched, so power users
// are never boxed in).
func guessArchiveRemote(arg string) string {
	for _, g := range repoGuesses {
		if g.rx.MatchString(arg) {
			return g.rx.ReplaceAllString(arg, g.repl)
		}
	}
	return arg
}
