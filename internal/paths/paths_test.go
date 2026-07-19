package paths

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// writeJSONL writes lines (already JSON-encoded strings) to a .jsonl file.
func writeJSONL(t *testing.T, path string, lines ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var content string
	for _, l := range lines {
		content += l + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestProjectsRoot(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T) (env string, wantSuffix string)
		envEmpty  bool
		wantExact func(home string) string
	}{
		{
			name: "CLAUDE_CONFIG_DIR with projects subdir wins",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir()
				if err := os.MkdirAll(filepath.Join(dir, "projects"), 0o755); err != nil {
					t.Fatal(err)
				}
				return dir, filepath.Join(dir, "projects")
			},
		},
		{
			name: "CLAUDE_CONFIG_DIR set but no projects subdir falls back to home",
			setup: func(t *testing.T) (string, string) {
				dir := t.TempDir() // no projects subdir
				return dir, ""     // sentinel: expect home fallback
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env, wantSuffix := tt.setup(t)
			t.Setenv("CLAUDE_CONFIG_DIR", env)

			got := ProjectsRoot()
			if wantSuffix != "" {
				if got != wantSuffix {
					t.Fatalf("ProjectsRoot() = %q, want %q", got, wantSuffix)
				}
				return
			}
			// Fallback path: must end in .claude/projects under the real home.
			home, _ := os.UserHomeDir()
			want := filepath.Join(home, ".claude", "projects")
			if got != want {
				t.Fatalf("ProjectsRoot() fallback = %q, want %q", got, want)
			}
		})
	}

	t.Run("unset CLAUDE_CONFIG_DIR falls back to home", func(t *testing.T) {
		t.Setenv("CLAUDE_CONFIG_DIR", "")
		_ = os.Unsetenv("CLAUDE_CONFIG_DIR")
		home, _ := os.UserHomeDir()
		want := filepath.Join(home, ".claude", "projects")
		if got := ProjectsRoot(); got != want {
			t.Fatalf("ProjectsRoot() = %q, want %q", got, want)
		}
	})
}

func TestFirstCWD(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{
			name:  "top-level cwd",
			lines: []string{`{"cwd":"/home/user/proj"}`},
			want:  "/home/user/proj",
		},
		{
			name:  "nested message.cwd",
			lines: []string{`{"message":{"cwd":"/home/user/nested"}}`},
			want:  "/home/user/nested",
		},
		{
			name:  "skips bad lines then finds cwd",
			lines: []string{`not json`, `{"type":"summary"}`, `{"cwd":"/found"}`},
			want:  "/found",
		},
		{
			name:  "empty cwd string skipped",
			lines: []string{`{"cwd":""}`, `{"cwd":"/real"}`},
			want:  "/real",
		},
		{
			name:  "no cwd anywhere",
			lines: []string{`{"type":"user"}`, `{"role":"assistant"}`},
			want:  "",
		},
		{
			name:  "top-level preferred over message",
			lines: []string{`{"cwd":"/top","message":{"cwd":"/inner"}}`},
			want:  "/top",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := filepath.Join(dir, tt.name+".jsonl")
			writeJSONL(t, p, tt.lines...)
			if got := firstCWD(p); got != tt.want {
				t.Fatalf("firstCWD() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("missing file returns empty", func(t *testing.T) {
		if got := firstCWD(filepath.Join(dir, "nope.jsonl")); got != "" {
			t.Fatalf("firstCWD(missing) = %q, want empty", got)
		}
	})
}

func TestFindTranscriptDir(t *testing.T) {
	t.Run("matches by recorded cwd", func(t *testing.T) {
		base := t.TempDir()
		projects := filepath.Join(base, "projects")
		t.Setenv("CLAUDE_CONFIG_DIR", base)
		encoded := filepath.Join(projects, "-home-user-myproj")
		realCwd := t.TempDir() // an existing dir to be the recorded cwd
		// Record the realpath so the match (realpath(rec) == realpath(target)) holds
		// regardless of macOS /tmp→/private/tmp symlinking.
		writeJSONL(t, filepath.Join(encoded, "sess1.jsonl"), `{"cwd":"`+jsonEscape(realpath(realCwd))+`"}`)

		got := FindTranscriptDir(realCwd)
		if got != encoded {
			t.Fatalf("FindTranscriptDir(%q) = %q, want %q", realCwd, got, encoded)
		}
	})

	t.Run("folder guard: a dir merely holding loose jsonl is NOT a transcripts dir", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", base)
		// projects root exists but is empty; the target itself holds a jsonl.
		// The old loose-jsonl fallback fired here — how /tmp got indexed into
		// the real cache on a bare run. Implicit discovery must resolve nothing.
		if err := os.MkdirAll(filepath.Join(base, "projects"), 0o755); err != nil {
			t.Fatal(err)
		}
		target := t.TempDir()
		writeJSONL(t, filepath.Join(target, "x.jsonl"), `{"cwd":"/whatever"}`)

		if got := FindTranscriptDir(target); got != "" {
			t.Fatalf("FindTranscriptDir(loose-jsonl dir) = %q, want empty (implicit discovery)", got)
		}
	})

	t.Run("already-encoded projects child is still returned verbatim", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", base)
		projects := filepath.Join(base, "projects")
		child := filepath.Join(projects, "-home-user-thing")
		if err := os.MkdirAll(child, 0o755); err != nil {
			t.Fatal(err)
		}
		got := FindTranscriptDir(child)
		if got != child && got != realpath(child) {
			t.Fatalf("FindTranscriptDir(projects child) = %q, want %q", got, child)
		}
	})

	t.Run("no match falls back to encoded path when that dir exists", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", base)
		projects := filepath.Join(base, "projects")
		if err := os.MkdirAll(projects, 0o755); err != nil {
			t.Fatal(err)
		}
		// cwd that does not exist on disk → encoded fallback. Encoding is applied
		// to the REALPATH'd target (on macOS /home resolves via a firmlink), so
		// derive the encoded name the same way the code does.
		cwd := "/home/user/ghost.dir"
		enc := encodePath(realpath(cwd))
		encDir := filepath.Join(projects, enc)
		if err := os.MkdirAll(encDir, 0o755); err != nil {
			t.Fatal(err)
		}
		got := FindTranscriptDir(cwd)
		if got != encDir {
			t.Fatalf("FindTranscriptDir(%q) = %q, want %q", cwd, got, encDir)
		}
	})

	t.Run("no match and no encoded dir returns empty", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", base)
		if err := os.MkdirAll(filepath.Join(base, "projects"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := FindTranscriptDir("/home/user/nothing-here"); got != "" {
			t.Fatalf("FindTranscriptDir(none) = %q, want empty", got)
		}
	})
}

// TestFindTranscriptDirExplicit: the explicit --dir opt-in accepts an arbitrary
// jsonl-bearing folder — the escape hatch implicit discovery no longer has —
// while still preferring the ordinary resolution when it answers.
func TestFindTranscriptDirExplicit(t *testing.T) {
	t.Run("loose-jsonl dir accepted verbatim", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", base)
		if err := os.MkdirAll(filepath.Join(base, "projects"), 0o755); err != nil {
			t.Fatal(err)
		}
		target := t.TempDir()
		writeJSONL(t, filepath.Join(target, "x.jsonl"), `{"cwd":"/whatever"}`)

		got := FindTranscriptDirExplicit(target)
		if got != realpath(target) && got != target {
			t.Fatalf("FindTranscriptDirExplicit(loose) = %q, want %q", got, target)
		}
	})

	t.Run("recorded-cwd match wins over the loose fallback", func(t *testing.T) {
		base := t.TempDir()
		projects := filepath.Join(base, "projects")
		t.Setenv("CLAUDE_CONFIG_DIR", base)
		encoded := filepath.Join(projects, "-home-user-myproj")
		workdir := t.TempDir()
		// The working dir ALSO holds a stray .jsonl (a repo carrying jsonl data
		// files): the transcript whose recorded cwd matches must win, not the
		// stray-file fallback.
		writeJSONL(t, filepath.Join(workdir, "data.jsonl"), `{"k":"v"}`)
		writeJSONL(t, filepath.Join(encoded, "sess1.jsonl"), `{"cwd":"`+jsonEscape(realpath(workdir))+`"}`)

		got := FindTranscriptDirExplicit(workdir)
		if got != encoded {
			t.Fatalf("FindTranscriptDirExplicit(workdir) = %q, want %q", got, encoded)
		}
	})

	t.Run("no jsonl and no match resolves empty", func(t *testing.T) {
		base := t.TempDir()
		t.Setenv("CLAUDE_CONFIG_DIR", base)
		if err := os.MkdirAll(filepath.Join(base, "projects"), 0o755); err != nil {
			t.Fatal(err)
		}
		if got := FindTranscriptDirExplicit(t.TempDir()); got != "" {
			t.Fatalf("FindTranscriptDirExplicit(empty dir) = %q, want empty", got)
		}
	})
}

func TestContainedJSONL(t *testing.T) {
	t.Run("recursive, excludes symlink-out", func(t *testing.T) {
		root := t.TempDir()
		// Inside files at multiple depths.
		writeJSONL(t, filepath.Join(root, "a.jsonl"), `{}`)
		writeJSONL(t, filepath.Join(root, "sub", "b.jsonl"), `{}`)
		writeJSONL(t, filepath.Join(root, "sub", "deep", "c.jsonl"), `{}`)

		// An outside file, symlinked into the root — must be EXCLUDED.
		outside := t.TempDir()
		writeJSONL(t, filepath.Join(outside, "escape.jsonl"), `{}`)
		link := filepath.Join(root, "escape.jsonl")
		if err := os.Symlink(filepath.Join(outside, "escape.jsonl"), link); err != nil {
			t.Skipf("symlink unsupported: %v", err)
		}

		got := ContainedJSONL(root)
		// All three inside files present; the symlinked escape absent.
		for _, want := range []string{
			filepath.Join(root, "a.jsonl"),
			filepath.Join(root, "sub", "b.jsonl"),
			filepath.Join(root, "sub", "deep", "c.jsonl"),
		} {
			if !containsResolved(got, want) {
				t.Errorf("ContainedJSONL missing %q; got %v", want, got)
			}
		}
		if containsResolved(got, link) {
			t.Errorf("ContainedJSONL leaked symlink-out %q; got %v", link, got)
		}
	})

	t.Run("empty dir returns empty slice not nil", func(t *testing.T) {
		root := t.TempDir()
		got := ContainedJSONL(root)
		if got == nil {
			t.Fatal("ContainedJSONL returned nil, want non-nil empty slice")
		}
		if len(got) != 0 {
			t.Fatalf("ContainedJSONL(empty) = %v, want []", got)
		}
	})
}

func TestProjectLabel(t *testing.T) {
	tests := []struct {
		name    string
		dirName string
		lines   []string
		want    string
	}{
		{
			name:    "basename of recorded cwd",
			dirName: "-home-user-myproj",
			lines:   []string{`{"cwd":"/home/user/myproj"}`},
			want:    "myproj",
		},
		{
			name:    "trailing slash stripped",
			dirName: "-home-user-x",
			lines:   []string{`{"cwd":"/home/user/coolproj/"}`},
			want:    "coolproj",
		},
		{
			name:    "no cwd falls back to encoded dir basename",
			dirName: "-home-user-encoded",
			lines:   []string{`{"type":"user"}`},
			want:    "-home-user-encoded",
		},
		{
			name:    "root cwd falls back to enc",
			dirName: "-enc",
			lines:   []string{`{"cwd":"/"}`},
			want:    "-enc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			tdir := filepath.Join(base, tt.dirName)
			writeJSONL(t, filepath.Join(tdir, "s.jsonl"), tt.lines...)
			if got := ProjectLabel(tdir); got != tt.want {
				t.Fatalf("ProjectLabel() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("no jsonl uses encoded basename", func(t *testing.T) {
		base := t.TempDir()
		tdir := filepath.Join(base, "-bare-dir")
		if err := os.MkdirAll(tdir, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := ProjectLabel(tdir); got != "-bare-dir" {
			t.Fatalf("ProjectLabel(no jsonl) = %q, want %q", got, "-bare-dir")
		}
	})
}

func TestAllProjectDirs(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", base)
	projects := filepath.Join(base, "projects")

	withJSONL := filepath.Join(projects, "-a")
	writeJSONL(t, filepath.Join(withJSONL, "s.jsonl"), `{}`)
	another := filepath.Join(projects, "-b")
	writeJSONL(t, filepath.Join(another, "t.jsonl"), `{}`)

	// A dir with NO top-level jsonl (only nested) must be excluded.
	noTop := filepath.Join(projects, "-c")
	writeJSONL(t, filepath.Join(noTop, "sub", "nested.jsonl"), `{}`)

	// A plain file (not a dir) must be excluded.
	if err := os.WriteFile(filepath.Join(projects, "loosefile"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := AllProjectDirs()
	want := []string{withJSONL, another}
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("AllProjectDirs() = %v, want %v", got, want)
	}
}

func TestProjectCWD(t *testing.T) {
	t.Run("returns recorded cwd verbatim (not basename)", func(t *testing.T) {
		base := t.TempDir()
		tdir := filepath.Join(base, "-home-user-proj")
		writeJSONL(t, filepath.Join(tdir, "s.jsonl"), `{"cwd":"/home/user/proj"}`)
		if got := ProjectCWD(tdir); got != "/home/user/proj" {
			t.Fatalf("ProjectCWD() = %q, want %q", got, "/home/user/proj")
		}
	})

	t.Run("falls back to encoded dir basename", func(t *testing.T) {
		base := t.TempDir()
		tdir := filepath.Join(base, "-encoded-name")
		writeJSONL(t, filepath.Join(tdir, "s.jsonl"), `{"type":"user"}`)
		if got := ProjectCWD(tdir); got != "-encoded-name" {
			t.Fatalf("ProjectCWD() = %q, want %q", got, "-encoded-name")
		}
	})
}

func TestResolveSession(t *testing.T) {
	base := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", base)
	projects := filepath.Join(base, "projects")

	tdir := filepath.Join(projects, "-home-user-proj")
	writeJSONL(t, filepath.Join(tdir, "a1b2c3d4-full-session.jsonl"), `{"cwd":"/home/user/proj"}`)
	writeJSONL(t, filepath.Join(tdir, "deadbeef-other.jsonl"), `{"cwd":"/home/user/proj"}`)
	// A subagent thread under subagents/ MUST be skipped (top-level glob only).
	writeJSONL(t, filepath.Join(tdir, "subagents", "a1b2c3d4-sub.jsonl"), `{"cwd":"/home/user/proj"}`)

	t.Run("prefix match, top-level only", func(t *testing.T) {
		hits := ResolveSession("a1b2c3d4")
		if len(hits) != 1 {
			t.Fatalf("ResolveSession got %d hits, want 1: %+v", len(hits), hits)
		}
		h := hits[0]
		if h.SessionID != "a1b2c3d4-full-session" {
			t.Errorf("SessionID = %q", h.SessionID)
		}
		if h.CWD != "/home/user/proj" {
			t.Errorf("CWD = %q", h.CWD)
		}
		if h.Project != "proj" {
			t.Errorf("Project = %q, want proj", h.Project)
		}
	})

	t.Run("no match returns empty non-nil", func(t *testing.T) {
		hits := ResolveSession("zzzzzzzz")
		if hits == nil {
			t.Fatal("ResolveSession returned nil, want empty slice")
		}
		if len(hits) != 0 {
			t.Fatalf("ResolveSession(none) = %+v, want empty", hits)
		}
	})
}

func TestExpandUser(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"tilde alone", "~", home},
		{"tilde slash", "~/.claude", filepath.Join(home, ".claude")},
		{"no tilde unchanged", "/abs/path", "/abs/path"},
		{"tilde-user unchanged", "~bob/x", "~bob/x"},
		{"relative unchanged", "rel/path", "rel/path"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := expandHome(tt.in); got != tt.want {
				t.Fatalf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPyBasename(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"/home/user/proj", "proj"},
		{"proj", "proj"},
		{"", ""},
		{"/home/user/", ""},
		{"/", ""},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := baseName(tt.in); got != tt.want {
				t.Fatalf("baseName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// jsonEscape escapes a string for embedding inside a JSON string literal in
// test fixtures (handles the backslashes/quotes a tmp path might contain).
func jsonEscape(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '\\':
			out = append(out, '\\', '\\')
		case '"':
			out = append(out, '\\', '"')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

// containsResolved reports whether want (or its realpath) is present in got
// (comparing realpaths to survive macOS /tmp→/private/tmp symlinking).
func containsResolved(got []string, want string) bool {
	wantRP := realpath(want)
	for _, g := range got {
		if g == want || realpath(g) == wantRP {
			return true
		}
	}
	return false
}
