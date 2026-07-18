// Package provenance mints and derives the identity facts stamped onto indexed
// rows (D3): the machine's stable self-minted id, the cheap file content
// fingerprint used as a change watermark, and the session id derived from a
// transcript path. It imports only internal/store (for the shared state dir).
package provenance

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// FileFingerprint is a cheap content fingerprint (sha1 of first 4KB + "|" + last
// 4KB, hex[:16]) catching a same-mtime+same-size in-place rewrite at either end.
// Returns "" on any I/O error.
func FileFingerprint(path string, size int64) string {
	fh, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer fh.Close()

	head := make([]byte, 4096)
	n, err := io.ReadFull(fh, head)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return ""
	}
	head = head[:n]

	var tail []byte
	if size > 8192 {
		if _, err := fh.Seek(-4096, io.SeekEnd); err != nil {
			return ""
		}
		tail = make([]byte, 4096)
		m, err := io.ReadFull(fh, tail)
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
			return ""
		}
		tail = tail[:m]
	}

	h := sha1.New()
	h.Write(head)
	h.Write([]byte("|"))
	h.Write(tail)
	return hex.EncodeToString(h.Sum(nil))[:16]
}

var (
	machineIDOnce  sync.Once
	machineIDValue string
)

// MachineID returns this machine's stable, self-minted id (D3): a persisted
// 128-bit random hex value in rawclaw's state dir, NOT the (mutable, collision-
// prone) hostname. Minted and persisted on first use, then cached for the
// process. On any I/O failure it degrades to an in-memory random id so provenance
// is still stamped and indexing never blocks.
func MachineID() string {
	machineIDOnce.Do(func() { machineIDValue = loadOrMintMachineID() })
	return machineIDValue
}

// machineIDPath is <cacheHome>/session-search/machine-id — the same state dir
// that holds the caches and the tombstone sidecar.
func machineIDPath() string {
	return filepath.Join(store.CacheDir(), "machine-id")
}

// loadOrMintMachineID reads the persisted id, or mints + persists a fresh one.
func loadOrMintMachineID() string {
	p := machineIDPath()
	if b, err := os.ReadFile(p); err == nil {
		if id := strings.TrimSpace(string(b)); id != "" {
			return id
		}
	}
	id := randomHex()
	_ = os.WriteFile(p, []byte(id+"\n"), 0o644) // best-effort; re-mint next run on failure
	return id
}

// randomHex returns 32 hex chars (128 bits) of crypto-random. On the (extremely
// unlikely) entropy-source failure it falls back to a pid+time value — still
// stable-enough for a single run to keep provenance stamped.
func randomHex() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(fmt.Sprintf("pid%d-%d", os.Getpid(), time.Now().UnixNano())))
	}
	return hex.EncodeToString(b)
}

// SessionIDFor returns the unique session id for a transcript path, plus whether
// it is a subagent (1/0) and its parent id. Top-level: filename stem. Subagent
// (under a subagents/ subdir): "<parent>/<stem>".
func SessionIDFor(path, transcriptDir string) (sid string, isSubagent int, parent string) {
	stem := stemOf(path)
	rel, err := filepath.Rel(transcriptDir, path)
	if err != nil {
		rel = path
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	for i, p := range parts {
		if p == "subagents" {
			if i > 0 {
				par := parts[i-1]
				return par + "/" + stem, 1, par
			}
			return "subagents/" + stem, 1, "" // empty parent -> SQL NULL
		}
	}
	return stem, 0, ""
}

// stemOf returns the filename with its final extension stripped.
func stemOf(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}
