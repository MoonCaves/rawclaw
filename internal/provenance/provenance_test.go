package provenance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionIDFor(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		dir        string
		wantSID    string
		wantSub    int
		wantParent string
	}{
		{"top level", "/p/abc.jsonl", "/p", "abc", 0, ""},
		{"subagent with parent", "/p/parent/subagents/child.jsonl", "/p", "parent/child", 1, "parent"},
		{"subagent no parent", "/p/subagents/child.jsonl", "/p", "subagents/child", 1, ""},
		{"nested top level", "/p/sub/dir/x.jsonl", "/p", "x", 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sid, sub, parent := SessionIDFor(tt.path, tt.dir)
			if sid != tt.wantSID || sub != tt.wantSub || parent != tt.wantParent {
				t.Errorf("SessionIDFor(%q,%q) = (%q,%d,%q), want (%q,%d,%q)",
					tt.path, tt.dir, sid, sub, parent, tt.wantSID, tt.wantSub, tt.wantParent)
			}
		})
	}
}

func TestFileFingerprint(t *testing.T) {
	dir := t.TempDir()

	// Golden fingerprint vectors for known inputs, used to pin FileFingerprint.
	small := filepath.Join(dir, "small.bin")
	if err := os.WriteFile(small, []byte("hello world this is a test"), 0o644); err != nil {
		t.Fatal(err)
	}
	large := filepath.Join(dir, "large.bin")
	largeData := make([]byte, 0, 10240)
	for i := 0; i < 40; i++ {
		for b := 0; b < 256; b++ {
			largeData = append(largeData, byte(b))
		}
	}
	if err := os.WriteFile(large, largeData, 0o644); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty.bin")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		size int64
		want string
	}{
		{"small (no tail)", small, 26, "2ed0803756881215"},
		{"large (head+tail)", large, 10240, "ed6c8206f27a1fb4"},
		{"empty", empty, 0, "3eb416223e9e69e6"},
		{"missing file", filepath.Join(dir, "nope.bin"), 100, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FileFingerprint(tt.path, tt.size); got != tt.want {
				t.Errorf("FileFingerprint(%q,%d) = %q, want %q", tt.path, tt.size, got, tt.want)
			}
			if len(tt.want) > 0 && len(FileFingerprint(tt.path, tt.size)) != 16 {
				t.Errorf("fingerprint must be 16 hex chars")
			}
		})
	}
}
