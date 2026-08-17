package scopes

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAntigravityDBPath_Injective(t *testing.T) {
	t.Parallel()
	pairs := [][2]string{
		{"/Users/a/b", "/Users/a-b"},
		{"/Users/a.b", "/Users/a-b"},
		{"/x/y", "/x.y"},
	}
	for _, p := range pairs {
		if got0, got1 := antigravityDBPath(p[0]), antigravityDBPath(p[1]); got0 == got1 {
			t.Errorf("antigravityDBPath collision: %q and %q both -> %q", p[0], p[1], got0)
		}
	}
}

func TestAntigravityDBPath_Prefixed(t *testing.T) {
	t.Parallel()
	base := filepath.Base(antigravityDBPath("/Users/octocat/proj"))
	if !strings.HasPrefix(base, "antigravity-") {
		t.Errorf("antigravityDBPath base %q lacks the antigravity- prefix", base)
	}
}

func TestAntigravityLabel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		cwd  string
		want string
	}{
		{"", "antigravity"},
		{"/home/user/code/proj", "proj"},
		{"/home/user/code/proj/", "proj"},
	}
	for _, tt := range tests {
		if got := antigravityLabel(tt.cwd); got != tt.want {
			t.Errorf("antigravityLabel(%q) = %q, want %q", tt.cwd, got, tt.want)
		}
	}
}

func TestAntigravityOrphanLabel(t *testing.T) {
	t.Parallel()
	dbp := antigravityDBPath("/home/user/myproject")
	base := filepath.Base(dbp)
	label := antigravityOrphanLabel(base)
	if label != "myproject" {
		t.Errorf("antigravityOrphanLabel(%q) = %q, want myproject", base, label)
	}
}
