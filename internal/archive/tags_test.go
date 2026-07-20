package archive

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTagTestArchive builds an Archive pointed at a temp clone, with the given
// exporter injected. White-box: the tag export path needs no git remote.
func newTagTestArchive(t *testing.T, exporter TagExporter) *Archive {
	t.Helper()
	return &Archive{
		cfg:        Config{Name: "box-a", Remote: "unused://remote"},
		clone:      t.TempDir(),
		machineID:  "machine-a-id",
		exportTags: exporter,
	}
}

// readTagFile reads and decodes a written tag file.
func readTagFile(t *testing.T, path string) TagFile {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tag file %s: %v", path, err)
	}
	var tf TagFile
	if err := json.Unmarshal(b, &tf); err != nil {
		t.Fatalf("decode tag file %s: %v", path, err)
	}
	return tf
}

func TestExportOwnTags_WritesPerMachinePath(t *testing.T) {
	a := newTagTestArchive(t, func() ([]TagFile, error) {
		return []TagFile{
			{
				SessionID:     "sess-aaaa",
				OriginMachine: "SPOOFED", // must be overwritten with our id
				Segments: []TagSegment{
					{StartUUID: "u1", EndUUID: "u2", Topic: "auth flow", Summary: "s", TaggedAt: 10},
					{StartUUID: "u3", Topic: "cleanup"},
				},
				Verdict: &TagVerdict{Verdict: "routine", Source: "agent", TaggedAt: 11},
			},
			{SessionID: "sess-bbbb", Verdict: &TagVerdict{Verdict: "routine", Source: "floor", TaggedAt: 5}},
			{SessionID: ""}, // keyless — must be skipped
		}, nil
	})

	n, err := a.exportOwnTags()
	if err != nil {
		t.Fatalf("exportOwnTags: %v", err)
	}
	if n != 2 {
		t.Errorf("wrote %d tag files, want 2 (keyless skipped)", n)
	}

	// Files live under this machine's dir, never a shared/overwritable path.
	tagsDir := filepath.Join(a.clone, "box-a", "tags")
	aaa := filepath.Join(tagsDir, "sess-aaaa.json")
	tf := readTagFile(t, aaa)

	if tf.OriginMachine != "machine-a-id" {
		t.Errorf("origin_machine = %q, want the archive's own id (exporter value must not win)", tf.OriginMachine)
	}
	if len(tf.Segments) != 2 || tf.Segments[0].Topic != "auth flow" || tf.Segments[1].StartUUID != "u3" {
		t.Errorf("segments round-trip wrong: %+v", tf.Segments)
	}
	if tf.Verdict == nil || tf.Verdict.Source != "agent" {
		t.Errorf("verdict round-trip wrong: %+v", tf.Verdict)
	}

	// The verdict-only session wrote a file with no segments.
	tfb := readTagFile(t, filepath.Join(tagsDir, "sess-bbbb.json"))
	if len(tfb.Segments) != 0 || tfb.Verdict == nil || tfb.Verdict.Source != "floor" {
		t.Errorf("verdict-only file wrong: %+v", tfb)
	}

	// Atomic write leaves no temp files behind.
	entries, err := os.ReadDir(tagsDir)
	if err != nil {
		t.Fatalf("read tags dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tag-") {
			t.Errorf("leftover temp file %s (write not atomic/cleaned)", e.Name())
		}
	}
	if len(entries) != 2 {
		t.Errorf("tags dir holds %d entries, want 2", len(entries))
	}
}

func TestExportOwnTags_SubagentSlashID_AndTempOutsideTree(t *testing.T) {
	// B2: a subagent session id contains "/" (provenance "<parent>/<child>"), so
	// the tag file lands in a nested subdir — must not crash the rename.
	// B1: the temp is staged under .git, never in the tracked tags tree.
	a := newTagTestArchive(t, func() ([]TagFile, error) {
		return []TagFile{
			{SessionID: "parent/child-sub", Segments: []TagSegment{{StartUUID: "u1", Topic: "sub work"}}},
		}, nil
	})
	if n, err := a.exportOwnTags(); err != nil || n != 1 {
		t.Fatalf("export subagent tag: got (%d, %v), want (1, nil)", n, err)
	}
	nested := filepath.Join(a.clone, "box-a", "tags", "parent", "child-sub.json")
	if _, err := os.Stat(nested); err != nil {
		t.Errorf("subagent tag not written to nested path %s: %v", nested, err)
	}
	// No stray temp anywhere under the tracked machine dir (B1: staged under .git).
	_ = filepath.WalkDir(filepath.Join(a.clone, "box-a"), func(p string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() && strings.HasPrefix(d.Name(), ".tag-") {
			t.Errorf("stray temp in tracked tree: %s", p)
		}
		return nil
	})
}

func TestExportOwnTags_NilExporterAndEmpty(t *testing.T) {
	// nil exporter: a clean no-op, no dir created.
	a := newTagTestArchive(t, nil)
	if n, err := a.exportOwnTags(); err != nil || n != 0 {
		t.Fatalf("nil exporter: got (%d, %v), want (0, nil)", n, err)
	}
	if _, err := os.Stat(a.tagsDir()); !os.IsNotExist(err) {
		t.Errorf("nil exporter created a tags dir: %v", err)
	}

	// Exporter returning nothing: also a no-op.
	a2 := newTagTestArchive(t, func() ([]TagFile, error) { return nil, nil })
	if n, err := a2.exportOwnTags(); err != nil || n != 0 {
		t.Fatalf("empty exporter: got (%d, %v), want (0, nil)", n, err)
	}
}
