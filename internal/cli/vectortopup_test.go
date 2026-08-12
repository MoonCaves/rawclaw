package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/MoonCaves/rawclaw/internal/semantic"
)

// countVectorTopupSpawns swaps the spawn seam for a counter so gate tests observe
// spawn decisions without forking processes.
func countVectorTopupSpawns(t *testing.T) (*int, *string) {
	t.Helper()
	calls := 0
	lastDBP := ""
	old := spawnVectorTopup
	spawnVectorTopup = func(dbp string) {
		calls++
		lastDBP = dbp
	}
	t.Cleanup(func() { spawnVectorTopup = old })
	return &calls, &lastDBP
}

// TestVectorTopup_UnconfiguredMeansZeroSpawns: no embedder configured →
// zero child spawns on invocation.
func TestVectorTopup_UnconfiguredMeansZeroSpawns(t *testing.T) {
	newCfgRoot(t)
	t.Setenv("RAWCLAW_EMBED_ENDPOINT", "")
	calls, _ := countVectorTopupSpawns(t)

	td := t.TempDir()
	f := filepath.Join(td, "test.jsonl")
	_ = os.WriteFile(f, []byte(`{"type":"user","timestamp":"2026-08-12T10:00:00Z","message":{"content":"Hello world text for testing"}}`+"\n"), 0o644)

	root := NewRootCmd(BuildInfo{})
	_, _ = runCmd(t, root, "", "--dir", td, "Hello")

	if *calls != 0 {
		t.Errorf("spawns with no embedder = %d, want 0", *calls)
	}
}

// TestVectorTopup_NoVectorFlagMeansZeroSpawns: --no-vector passed →
// zero top-up spawns even with RAWCLAW_EMBED_ENDPOINT set.
func TestVectorTopup_NoVectorFlagMeansZeroSpawns(t *testing.T) {
	newCfgRoot(t)
	t.Setenv("RAWCLAW_EMBED_ENDPOINT", "http://localhost:11434/api/embeddings")
	calls, _ := countVectorTopupSpawns(t)

	td := t.TempDir()
	f := filepath.Join(td, "test.jsonl")
	_ = os.WriteFile(f, []byte(`{"type":"user","timestamp":"2026-08-12T10:00:00Z","message":{"content":"Hello world text for testing"}}`+"\n"), 0o644)

	root := NewRootCmd(BuildInfo{})
	_, _ = runCmd(t, root, "", "--no-vector", "--dir", td, "Hello")

	if *calls != 0 {
		t.Errorf("spawns under --no-vector = %d, want 0", *calls)
	}
}

// TestVectorTopup_TriggersOnIndexEnsure: when RAWCLAW_EMBED_ENDPOINT is configured,
// bringing a scope current triggers a top-up spawn.
func TestVectorTopup_TriggersOnIndexEnsure(t *testing.T) {
	newCfgRoot(t)
	t.Setenv("RAWCLAW_EMBED_ENDPOINT", "http://localhost:11434/api/embeddings")
	calls, _ := countVectorTopupSpawns(t)

	td := t.TempDir()
	f := filepath.Join(td, "test.jsonl")
	_ = os.WriteFile(f, []byte(`{"type":"user","timestamp":"2026-08-12T10:00:00Z","message":{"content":"Hello world text for testing"}}`+"\n"), 0o644)

	root := NewRootCmd(BuildInfo{})
	_, err := runCmd(t, root, "", "--dir", td, "Hello")
	if err != nil {
		t.Fatalf("search error: %v", err)
	}

	if *calls != 1 {
		t.Errorf("spawns after index ensure = %d, want 1", *calls)
	}
}

// TestSpawnVectorTopupChild_DetachedChildRunsWithLog: the real spawn path, with a
// fake binary: the detached child runs `vector-topup --dbp <dbp>` and logs receipt.
func TestSpawnVectorTopupChild_DetachedChildRunsWithLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh-script fake child")
	}

	script := filepath.Join(t.TempDir(), "fake-rawclaw")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"child-argv $*\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldExe := selfExe
	selfExe = func() (string, error) { return script, nil }
	t.Cleanup(func() { selfExe = oldExe })

	dbp := filepath.Join(t.TempDir(), "test_child.db")
	spawnVectorTopupChild(dbp)

	deadline := time.Now().Add(5 * time.Second)
	want := "child-argv vector-topup --dbp " + dbp
	for {
		b, _ := os.ReadFile(semantic.VectorTopupLogPath())
		if strings.Contains(string(b), want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("receipt log never showed %q; log:\n%s", want, b)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
