package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/store"
)

// TestIsConsolidatedSourceRejectsHardLinkAlias is a red proof: a hard link is
// the same SQLite file but has a distinct pathname, so EvalSymlinks/string
// equality must not be used as the self-source identity check.
func TestIsConsolidatedSourceRejectsHardLinkAlias(t *testing.T) {
	newCfgRoot(t)
	dst := index.ConsolidatedPath()
	con, err := store.ConnectRW(dst)
	if err != nil {
		t.Fatalf("create consolidated store: %v", err)
	}
	if err := store.Rebuild(con); err != nil {
		_ = con.Close()
		t.Fatalf("rebuild consolidated store: %v", err)
	}
	if err := con.Close(); err != nil {
		t.Fatalf("close consolidated store: %v", err)
	}

	alias := filepath.Join(t.TempDir(), "consolidated-hardlink.db")
	if err := os.Link(dst, alias); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if !isConsolidatedSource(alias) {
		t.Fatal("isConsolidatedSource rejected a hard-link alias of consolidated.db")
	}
}
