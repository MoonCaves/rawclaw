package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/MoonCaves/rawclaw/internal/semantic"
)

// vectorTopupLogMax caps the receipt log's growth (512 KB), matching autosyncLogMax.
const vectorTopupLogMax = 512 * 1024

// spawnVectorTopup connects the semantic package's top-up trigger to the detached child spawner.
var spawnVectorTopup = spawnVectorTopupChild

func init() {
	semantic.SetSpawnVectorTopup(func(dbp string) {
		spawnVectorTopup(dbp)
	})
}

// spawnVectorTopupChild fires the detached self-invocation (`vector-topup --dbp <dbp>`):
// own session (setsid), stdin closed, stdout+stderr appended to vector-topup.log in
// the state dir. Started with a bare exec.Command; start-and-release — parent never waits.
func spawnVectorTopupChild(dbp string) {
	exe, err := selfExe()
	if err != nil {
		return
	}
	logf, err := openVectorTopupLog()
	if err != nil {
		return
	}
	defer logf.Close() // parent's handle only; the child holds its own

	cmd := exec.Command(exe, "vector-topup", "--dbp", dbp)
	detach(cmd)
	cmd.Stdin = nil
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		return
	}
	_ = cmd.Process.Release() // one-shot parent: never Wait on a detached child
}

// openVectorTopupLog opens the receipt log for append, rotating an oversized log
// to a single .old generation first.
func openVectorTopupLog() (*os.File, error) {
	p := semantic.VectorTopupLogPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return nil, fmt.Errorf("create %s: %w", filepath.Dir(p), err)
	}
	if st, err := os.Stat(p); err == nil && st.Size() > vectorTopupLogMax {
		_ = os.Rename(p, p+".old")
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", p, err)
	}
	return f, nil
}
