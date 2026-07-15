package codex_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/source/codex"
)

// TestCodexRealDataSearchable drives the full Codex path end-to-end against the
// real ~/.codex/sessions: discover → generic ingest into a TEMP db → search via
// the real retrieve engine. Gated by RAWCLAW_CODEX_INTEGRATION so normal
// `go test` never depends on real local data or touches the shared cache.
func TestCodexRealDataSearchable(t *testing.T) {
	if os.Getenv("RAWCLAW_CODEX_INTEGRATION") == "" {
		t.Skip("set RAWCLAW_CODEX_INTEGRATION=1 to run against real ~/.codex/sessions")
	}

	a := codex.New()
	cs, err := a.Discover()
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(cs) == 0 {
		t.Skip("no codex sessions on this machine")
	}

	dbp := filepath.Join(t.TempDir(), "codex.db")
	n, _, err := index.EnsureIndexedContainers(dbp, true, cs, a.Messages)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	t.Logf("discovered %d containers, indexed %d sessions", len(cs), n)
	if n == 0 {
		t.Fatal("indexed 0 sessions from a non-empty corpus")
	}

	// A term ubiquitous in coding-agent transcripts; default params hide subagents.
	hits := retrieve.Search(dbp, "error", 5, retrieve.SearchParams{})
	if len(hits) == 0 {
		t.Fatal("search 'error' returned 0 hits across real Codex corpus")
	}
	t.Logf("search 'error' -> %d hits; top session=%s", len(hits), hits[0].SessionID)
}
