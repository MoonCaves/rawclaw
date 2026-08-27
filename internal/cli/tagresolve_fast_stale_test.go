package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/view"
)

func TestRunTagWriteFastPathRefreshesStaleTDirSource(t *testing.T) {
	root := newCfgRoot(t)
	const sid = "7f3e1c20-aaaa-bbbb-cccc-0000000abcde"
	first := "11111111-aaaa-bbbb-cccc-000000000001"
	second := "22222222-aaaa-bbbb-cccc-000000000002"
	dir := writeTaggableSession(t, root, "proj-stale-fast", sid, first)
	f, err := os.OpenFile(filepath.Join(dir, sid+".jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"type":"user","uuid":"` + second + `","timestamp":"2026-06-01T10:00:01Z","message":{"role":"user","content":"new message after indexing"}}` + "\n"); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	old := spawnTagPublish
	spawnTagPublish = func(string, string) error { return nil }
	t.Cleanup(func() { spawnTagPublish = old })
	var out strings.Builder
	err = runTagWriteCmd(&out, strings.NewReader(`[{"start_uuid":"`+second[:8]+`","topic":"new","summary":"new"}]`), sid[:8], []view.Scope{{Project: "proj-stale-fast", TDir: dir}}, nil, false, "", false)
	if err != nil {
		t.Fatalf("tag-write through stale TDir fast path: %v", err)
	}
}
