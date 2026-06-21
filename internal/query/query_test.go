package query

import (
	"reflect"
	"testing"
)

func TestParseTerms(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"plain two words", "api key", []string{"api", "key"}},
		{"lowercases bares", "API Key", []string{"api", "key"}},
		{"strips trailing star", "rotat*", []string{"rotat"}},
		{"drops bool keywords", "api AND key OR foo NOT bar", []string{"api", "key", "foo", "bar"}},
		{"quoted phrase lowercased", `"API Key" rotation`, []string{"api key", "rotation"}},
		// "" has an empty body: it doesn't match `"[^"]+"`, so it survives in
		// the remainder as the literal two-char token `""`.
		{"empty quoted phrase survives as bare", `"" foo`, []string{`""`, "foo"}},
		// "   " has a non-empty (whitespace) body: filtered from phrases by the
		// strip() guard AND removed from the remainder — so only foo remains.
		{"whitespace-only quote dropped", `"   " foo`, []string{"foo"}},
		{"phrases before bares", `"alpha beta" gamma`, []string{"alpha beta", "gamma"}},
		{"empty query", "", []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTerms(tt.in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseTerms(%q) = %#v, want %#v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSanitizeFTS5Query(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain unchanged", "api key rotation", "api key rotation"},
		{"drops structural chars", "api (key) +rotation", "api  key   rotation"},
		{"quoted run is protected verbatim", `foo"bar"baz`, `foo"bar"baz`}, // "bar" is a phrase, set aside and restored verbatim
		{"collapse run star", "foo*** bar", "foo* bar"},
		{"strip leading bare star", "* foo", "foo"}, // leading '*' removed, then trimmed
		{"strip leading bool", "AND foo bar", "foo bar"},
		{"strip trailing bool", "foo bar OR", "foo bar"},
		{"strip leading and trailing bool", "OR foo NOT", "foo"},
		{"quote dotted id", "session_id lookup", `"session_id" lookup`},
		{"quote hyphenated id", "rate-limit error", `"rate-limit" error`},
		{"quote dotted chain", "a.b.c thing", `"a.b.c" thing`},
		{"protect quoted phrase verbatim", `"exact phrase"`, `"exact phrase"`},
		{"phrase plus dotted id", `"keep me" session_id`, `"keep me" "session_id"`},
		// FIX 3: a trailing '*' on a dotted/hyphenated identifier must land OUTSIDE
		// the quote so FTS5 reads it as a valid prefix on the phrase's tokens.
		{"prefix star on hyphenated id", "self-update*", `"self-update"*`},
		{"prefix star on dotted id", "os.Rename*", `"os.Rename"*`},
		{"prefix star on underscore id", "is_subagent*", `"is_subagent"*`},
		// FIX 5: a path-like token (one with a '/') is quoted so FTS5 reads it as a
		// phrase of its tokens instead of dropping '/' as a separator and matching
		// nothing.
		{"quote tilde path", "~/.claude/projects", `"~/.claude/projects"`},
		{"quote relative path", ".claude/projects", `".claude/projects"`},
		{"two paths keep textual order", "a/b c.d/e.f", `"a/b" "c.d/e.f"`},
		{"path then quoted phrase order", `src/main.go "exact phrase"`, `"src/main.go" "exact phrase"`},
		{"quoted phrase then path order", `"exact phrase" src/main.go`, `"exact phrase" "src/main.go"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFTS5Query(tt.in)
			if got != tt.want {
				t.Errorf("SanitizeFTS5Query(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStripStopwords(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"drops common words", "what is the api key", "api key"},
		{"keeps content words", "api key rotation", "api key rotation"},
		{"drops bool stopword", "api and key", "api key"},
		{"preserves quoted phrase", `"the api" key`, `"the api" key`},
		{"strips star then checks stop", "the* api", "api"},
		{"all stopwords", "the of to in on", ""},
		{"quoted stopword kept verbatim", `"is" foo`, `"is" foo`},
		// FIX 3: a quoted phrase carrying an attached prefix-'*' (the sanitizer's
		// output for `self-update*`) must survive intact — the sentinel-bearing
		// token is restored in place, never dropped as a bare NUL.
		{"prefix star on quoted id survives", `"self-update"*`, `"self-update"*`},
		{"prefix star on dotted id survives", `"os.Rename"* lookup`, `"os.Rename"* lookup`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripStopwords(tt.in)
			if got != tt.want {
				t.Errorf("StripStopwords(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestHasSearchableToken(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"api key", true},
		{"AND OR NOT", false},
		{"and or not", false},
		{"AND foo", true},
		{"", false},
		{"   ", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := HasSearchableToken(tt.in); got != tt.want {
				t.Errorf("HasSearchableToken(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestMakeSnippet(t *testing.T) {
	t.Run("no text returns not ok", func(t *testing.T) {
		if s, ok := MakeSnippet("", []string{"foo"}); ok || s != "" {
			t.Errorf("MakeSnippet(\"\", ...) = %q,%v want \"\",false", s, ok)
		}
	})

	t.Run("term absent returns not ok", func(t *testing.T) {
		if s, ok := MakeSnippet("hello world", []string{"absent"}); ok || s != "" {
			t.Errorf("expected not ok, got %q,%v", s, ok)
		}
	})

	t.Run("highlights single term", func(t *testing.T) {
		s, ok := MakeSnippet("the api key was rotated", []string{"api"})
		if !ok {
			t.Fatal("expected ok")
		}
		want := "the >>>api<<< key was rotated"
		if s != want {
			t.Errorf("got %q, want %q", s, want)
		}
	})

	t.Run("case-insensitive highlight", func(t *testing.T) {
		s, ok := MakeSnippet("The API Key", []string{"api"})
		if !ok {
			t.Fatal("expected ok")
		}
		want := "The >>>API<<< Key"
		if s != want {
			t.Errorf("got %q, want %q", s, want)
		}
	})

	t.Run("collapses whitespace", func(t *testing.T) {
		s, ok := MakeSnippet("alpha   beta\tgamma", []string{"beta"})
		if !ok {
			t.Fatal("expected ok")
		}
		want := "alpha >>>beta<<< gamma"
		if s != want {
			t.Errorf("got %q, want %q", s, want)
		}
	})

	t.Run("longest term highlighted first then shorter nests", func(t *testing.T) {
		// "api key" (len 7) is highlighted before "api" (len 3). The second
		// pass then re-marks the "api" inside the first markup, producing the
		// nested >>>>>> markers seen below.
		s, ok := MakeSnippet("the api key here", []string{"api", "api key"})
		if !ok {
			t.Fatal("expected ok")
		}
		want := "the >>>>>>api<<< key<<< here"
		if s != want {
			t.Errorf("got %q, want %q", s, want)
		}
	})

	t.Run("empty term skipped", func(t *testing.T) {
		s, ok := MakeSnippet("find the token", []string{"", "token"})
		if !ok {
			t.Fatal("expected ok")
		}
		want := "find the >>>token<<<"
		if s != want {
			t.Errorf("got %q, want %q", s, want)
		}
	})
}

func TestBooleanToFTS5(t *testing.T) {
	t.Run("plain matches sanitize and no operators", func(t *testing.T) {
		plain := "api_key rotation"
		expr, used := BooleanToFTS5(plain)
		want := SanitizeFTS5Query(plain)
		if expr != want {
			t.Errorf("expr = %q, want sanitize result %q", expr, want)
		}
		if used {
			t.Errorf("usedOperators = true, want false for plain query")
		}
	})

	t.Run("and-not operators", func(t *testing.T) {
		expr, used := BooleanToFTS5("a && !b")
		if !used {
			t.Errorf("usedOperators = false, want true")
		}
		if !containsAll(expr, "AND", "NOT", "b") {
			t.Errorf("expr %q missing AND/NOT/b", expr)
		}
	})

	t.Run("or operator", func(t *testing.T) {
		expr, used := BooleanToFTS5("foo || bar")
		if !used {
			t.Errorf("usedOperators = false, want true")
		}
		if !containsAll(expr, "OR") {
			t.Errorf("expr %q missing OR", expr)
		}
	})

	t.Run("quoted phrase preserved with operators", func(t *testing.T) {
		expr, used := BooleanToFTS5(`"api key" && rotation`)
		if !used {
			t.Errorf("usedOperators = false, want true")
		}
		if !containsAll(expr, `"api key"`) {
			t.Errorf("expr %q lost quoted phrase", expr)
		}
	})

	t.Run("standalone quoted phrase is plain path", func(t *testing.T) {
		raw := `"exact phrase"`
		expr, used := BooleanToFTS5(raw)
		want := SanitizeFTS5Query(raw)
		if expr != want {
			t.Errorf("expr = %q, want %q", expr, want)
		}
		if used {
			t.Errorf("usedOperators = true, want false")
		}
		if !containsAll(expr, `"exact phrase"`) {
			t.Errorf("expr %q lost phrase", expr)
		}
	})

	t.Run("bang mid-word is not NOT", func(t *testing.T) {
		// '!' preceded by a word char is not an operator; foo!bar has no ops.
		_, used := BooleanToFTS5("foo!bar")
		if used {
			t.Errorf("usedOperators = true for mid-word '!', want false")
		}
	})

	// FIX 4: a spelled-out infix NOT is exclusion (documented in the search
	// skill). It must route through the boolean path (used=true) and survive into
	// the FTS5 expr verbatim — NOT through the plain path where StripStopwords
	// would silently drop "not".
	t.Run("infix NOT is an operator", func(t *testing.T) {
		expr, used := BooleanToFTS5("deploy NOT staging")
		if !used {
			t.Fatalf("usedOperators = false for infix NOT, want true")
		}
		if !containsAll(expr, "deploy", "NOT", "staging") {
			t.Errorf("expr %q missing deploy/NOT/staging", expr)
		}
	})

	t.Run("infix NOT bypasses the stopword path", func(t *testing.T) {
		// The defining bug: on the plain path, StripStopwords drops "not", so the
		// exclusion no-ops (note that StripStopwords IS destructive to "not"). The
		// boolean (RawMatch) path never calls StripStopwords, so the expr keeps NOT.
		if got := StripStopwords("deploy NOT staging"); containsAll(got, "NOT") {
			t.Fatalf("precondition: StripStopwords kept NOT (%q) — bug premise invalid", got)
		}
		expr, used := BooleanToFTS5("deploy NOT staging")
		if !used || !containsAll(expr, "NOT") {
			t.Errorf("boolean path must keep NOT: expr=%q used=%v", expr, used)
		}
	})

	t.Run("lowercase infix not is NOT an operator", func(t *testing.T) {
		// FTS5 only treats an UPPERCASE NOT as the operator; lowercase "not" is a
		// stopword, not exclusion — so it stays on the plain path.
		_, used := BooleanToFTS5("deploy not staging")
		if used {
			t.Errorf("usedOperators = true for lowercase 'not', want false")
		}
	})

	t.Run("leading NOT is not an operator", func(t *testing.T) {
		// A query that is only a leading/trailing NOT carries no exclusion target;
		// it stays on the plain path (the leading bool is stripped by sanitize).
		_, used := BooleanToFTS5("NOT staging")
		if used {
			t.Errorf("usedOperators = true for leading-only NOT, want false")
		}
	})
}

func TestPathPredicate(t *testing.T) {
	t.Run("include match", func(t *testing.T) {
		inc := PathPredicate(`/work/apps/`, "")
		if !inc("/Users/dev/work/apps/mm") {
			t.Error("expected match")
		}
		if inc("/Users/dev/work/tools/cli") {
			t.Error("expected no match")
		}
	})

	t.Run("exclude blocks", func(t *testing.T) {
		exc := PathPredicate("", `subagents`)
		if !exc("/Users/dev/work/apps/mm") {
			t.Error("non-subagent should pass")
		}
		if exc("/some/subagents/thread") {
			t.Error("subagent should be blocked")
		}
	})

	t.Run("both filters", func(t *testing.T) {
		both := PathPredicate(`apps`, `subagents`)
		if !both("/work/apps/mm") {
			t.Error("expected pass")
		}
		if both("/work/apps/subagents/x") {
			t.Error("expected excluded")
		}
		if both("/work/tools/cli") {
			t.Error("expected no-include reject")
		}
	})

	t.Run("no filters always true", func(t *testing.T) {
		p := PathPredicate("", "")
		if !p("/anything") || !p("") {
			t.Error("empty filters must always pass")
		}
	})

	t.Run("bad regex degrades to literal substring", func(t *testing.T) {
		// Must not panic; falls back to literal containment.
		bad := PathPredicate(`[unclosed`, `[bad`)
		_ = bad("/some/[unclosed/path") // just must not crash
		// literal include "[unclosed" present, literal exclude "[bad" absent → true
		if !bad("/some/[unclosed/path") {
			t.Error("literal include should match")
		}
		if bad("/some/[bad/path") {
			t.Error("literal exclude should block (include absent anyway)")
		}
	})
}

func TestMinMessagesOK(t *testing.T) {
	tests := []struct {
		count, min int
		want       bool
	}{
		{10, 3, true},
		{2, 3, false},
		{3, 3, true},
		{7, 8, false}, // subagent-typical thread excluded at min 8
	}
	for _, tt := range tests {
		if got := MinMessagesOK(tt.count, tt.min); got != tt.want {
			t.Errorf("MinMessagesOK(%d,%d) = %v, want %v", tt.count, tt.min, got, tt.want)
		}
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
