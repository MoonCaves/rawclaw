package cli

import (
	"context"
	"strings"
	"testing"
)

func TestRunTagPublishChild_SkipsConsolidatedSelf(t *testing.T) {
	var out strings.Builder
	if err := runTagPublishChild(context.Background(), &out, ""); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "skipped invalid/self") {
		t.Fatalf("receipt = %q, want self-skip receipt", out.String())
	}
}

func TestSpawnTagPublishChild_SkipsEmptyAndSelf(t *testing.T) {
	old := selfExe
	called := false
	selfExe = func() (string, error) { called = true; return "", nil }
	t.Cleanup(func() { selfExe = old })
	if err := spawnTagPublishChild(""); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("empty source resolved executable")
	}
}
