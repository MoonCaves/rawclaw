package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestThisProject_GitWorktreeScoping verifies that when rawclaw is executed with
// --this-project from within a Git worktree, it resolves the canonical Git root
// repository and finds transcripts from both the main checkout and sibling worktrees.
func TestThisProject_GitWorktreeScoping(t *testing.T) {
	root := newCfgRoot(t)

	// 1. Setup primary git repo
	mainRepo := t.TempDir()
	gitDir := filepath.Join(mainRepo, ".git")
	wtMeta1 := filepath.Join(gitDir, "worktrees", "wt-1")
	wtMeta2 := filepath.Join(gitDir, "worktrees", "wt-2")
	if err := os.MkdirAll(wtMeta1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(wtMeta2, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta1, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtMeta2, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. Setup worktree directories
	wtDir1 := t.TempDir()
	if err := os.WriteFile(filepath.Join(wtDir1, ".git"), []byte("gitdir: "+wtMeta1+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wtDir2 := t.TempDir()
	if err := os.WriteFile(filepath.Join(wtDir2, ".git"), []byte("gitdir: "+wtMeta2+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 3. Write session in main repo
	mainProjectDir := "-" + strings.ReplaceAll(strings.TrimPrefix(mainRepo, "/"), "/", "-")
	dirMain := filepath.Join(root, mainProjectDir)
	if err := os.MkdirAll(dirMain, 0o755); err != nil {
		t.Fatal(err)
	}
	rawJSON1, _ := json.Marshal("main repo needle keyword")
	line1 := `{"type":"user","uuid":"uuid-main-000000000001","timestamp":"2026-06-01T10:00:00Z","cwd":"` + mainRepo + `",` +
		`"message":{"role":"user","content":` + string(rawJSON1) + `}}`
	p1 := filepath.Join(dirMain, "sess-main-000000000001.jsonl")
	if err := os.WriteFile(p1, []byte(line1+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 4. Write session in worktree 1
	wt1ProjectDir := "-" + strings.ReplaceAll(strings.TrimPrefix(wtDir1, "/"), "/", "-")
	dirWT1 := filepath.Join(root, wt1ProjectDir)
	if err := os.MkdirAll(dirWT1, 0o755); err != nil {
		t.Fatal(err)
	}
	rawJSON2, _ := json.Marshal("worktree one needle keyword")
	line2 := `{"type":"user","uuid":"uuid-wt1-000000000001","timestamp":"2026-06-01T11:00:00Z","cwd":"` + wtDir1 + `",` +
		`"message":{"role":"user","content":` + string(rawJSON2) + `}}`
	p2 := filepath.Join(dirWT1, "sess-wt1-000000000001.jsonl")
	if err := os.WriteFile(p2, []byte(line2+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 5. Search from worktree 2 for the session recorded in main repo
	outMain, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--this-project", "--dir", wtDir2, "main repo needle")
	if err != nil {
		t.Fatalf("search from wt2 for main failed: %v\nout: %s", err, outMain)
	}
	if !strings.Contains(outMain, "sess-mai") {
		t.Fatalf("search from wt2 did not find main repo session:\n%s", outMain)
	}

	// 6. Search from worktree 2 for the session recorded in worktree 1 (sibling worktree)
	outWT1, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "--this-project", "--dir", wtDir2, "worktree one needle")
	if err != nil {
		t.Fatalf("search from wt2 for wt1 failed: %v\nout: %s", err, outWT1)
	}
	if !strings.Contains(outWT1, "sess-wt1") {
		t.Fatalf("search from wt2 did not find sibling worktree 1 session:\n%s", outWT1)
	}
}
