package archive

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBundle_ExportAndInitRoundTrip(t *testing.T) {
	home1 := newTestHome(t)
	bare := initBareRepo(t)

	ctx := context.Background()
	a, err := Init(ctx, bare, "machine-1")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	seedTranscripts(t, home1)
	if _, err := a.PushLocal(ctx); err != nil {
		t.Fatalf("PushLocal: %v", err)
	}

	bundlePath := filepath.Join(t.TempDir(), "archive.bundle")
	if err := a.ExportBundle(ctx, bundlePath); err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	fi, err := os.Stat(bundlePath)
	if err != nil {
		t.Fatalf("stat exported bundle: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("exported bundle is empty")
	}

	// Switch to a fresh home (second machine) and seed from bundle.
	newTestHome(t)

	if _, err := InitFromBundle(ctx, bundlePath, bare); err != nil {
		t.Fatalf("InitFromBundle: %v", err)
	}

	loaded, err := Load()
	if err != nil || loaded == nil {
		t.Fatalf("Load after InitFromBundle = (%v, %v), want configured", loaded, err)
	}
	if loaded.Remote() != bare {
		t.Errorf("loaded.Remote() = %q, want %q", loaded.Remote(), bare)
	}

	gitDir := filepath.Join(loaded.ClonePath(), ".git")
	sentinelPath := filepath.Join(gitDir, cloneSentinel)
	if _, err := os.Stat(sentinelPath); err != nil {
		t.Errorf("cloneSentinel %s missing: %v", sentinelPath, err)
	}

	originURL := strings.TrimSpace(gitT(t, loaded.ClonePath(), "remote", "get-url", "origin"))
	if originURL != bare {
		t.Errorf("git remote get-url origin = %q, want %q", originURL, bare)
	}

	// Verify the seeded clone has the transcripts committed by machine-1.
	if _, err := os.Stat(filepath.Join(loaded.ClonePath(), "machine-1")); err != nil {
		t.Errorf("machine-1 directory missing in seeded clone: %v", err)
	}
}

func TestBundle_ExportRefusesMissingClone(t *testing.T) {
	newTestHome(t)

	a := newArchive(Config{Remote: "git@example.com:test.git", Name: "machine-1"})
	bundlePath := filepath.Join(t.TempDir(), "missing.bundle")

	err := a.ExportBundle(context.Background(), bundlePath)
	if err == nil {
		t.Fatal("ExportBundle succeeded on missing clone, want error")
	}
	if !strings.Contains(err.Error(), "does not exist") && !strings.Contains(err.Error(), "stat") {
		t.Errorf("error = %v, want missing clone error", err)
	}
}

func TestBundle_ExportRefusesDirectory(t *testing.T) {
	newTestHome(t)
	bare := initBareRepo(t)

	ctx := context.Background()
	a, err := Init(ctx, bare, "machine-1")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	dirPath := t.TempDir()
	err = a.ExportBundle(ctx, dirPath)
	if err == nil {
		t.Fatal("ExportBundle succeeded with directory destination, want error")
	}
	if !strings.Contains(err.Error(), "directory") {
		t.Errorf("error = %v, want directory error", err)
	}
}

func TestBundle_ExportRefusesUnpushedCommits(t *testing.T) {
	newTestHome(t)
	bare := initBareRepo(t)

	ctx := context.Background()
	a, err := Init(ctx, bare, "machine-1")
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Create an unpushed commit in the clone.
	gitT(t, a.ClonePath(), "commit", "--allow-empty", "-m", "unpushed local commit")

	bundlePath := filepath.Join(t.TempDir(), "unpushed.bundle")
	err = a.ExportBundle(ctx, bundlePath)
	if err == nil {
		t.Fatal("ExportBundle succeeded with unpushed commits, want error")
	}
	if !strings.Contains(err.Error(), "unpushed") {
		t.Errorf("error = %v, want unpushed commits error", err)
	}
}

func TestBundle_InitFromBundleValidation(t *testing.T) {
	newTestHome(t)
	ctx := context.Background()

	t.Run("empty bundle path", func(t *testing.T) {
		if _, err := InitFromBundle(ctx, "", "git@example.com:repo.git"); err == nil {
			t.Error("InitFromBundle succeeded with empty bundle path, want error")
		}
	})

	t.Run("empty remote url", func(t *testing.T) {
		bundlePath := filepath.Join(t.TempDir(), "empty.bundle")
		_ = os.WriteFile(bundlePath, []byte("data"), 0o644)
		if _, err := InitFromBundle(ctx, bundlePath, ""); err == nil {
			t.Error("InitFromBundle succeeded with empty remote url, want error")
		}
	})

	t.Run("nonexistent bundle file", func(t *testing.T) {
		if _, err := InitFromBundle(ctx, "/path/to/nonexistent.bundle", "git@example.com:repo.git"); err == nil {
			t.Error("InitFromBundle succeeded with nonexistent bundle file, want error")
		}
	})

	t.Run("invalid bundle file", func(t *testing.T) {
		invalidBundle := filepath.Join(t.TempDir(), "invalid.bundle")
		if err := os.WriteFile(invalidBundle, []byte("not-a-git-bundle"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := InitFromBundle(ctx, invalidBundle, "git@example.com:repo.git"); err == nil {
			t.Error("InitFromBundle succeeded with invalid bundle file, want error")
		}
	})

	t.Run("already initialized", func(t *testing.T) {
		bare := initBareRepo(t)
		a, err := Init(ctx, bare, "machine-1")
		if err != nil {
			t.Fatalf("Init: %v", err)
		}
		bundlePath := filepath.Join(t.TempDir(), "test.bundle")
		if err := a.ExportBundle(ctx, bundlePath); err != nil {
			t.Fatalf("ExportBundle: %v", err)
		}
		if _, err := InitFromBundle(ctx, bundlePath, bare); err == nil {
			t.Error("InitFromBundle succeeded when already initialized, want error")
		}
	})
}
