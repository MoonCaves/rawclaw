//go:build unix

package cli

import (
	"os/exec"
	"testing"
)

// TestDetach_NewSession: detach must put the child in its own session
// (setsid). This is the mechanical property behind "a hung push can never
// hold a search open": the child shares no session, no process group, no
// terminal with the invoking tool call.
func TestDetach_NewSession(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("true")
	detach(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.Setsid {
		t.Fatalf("SysProcAttr = %+v, want Setsid", cmd.SysProcAttr)
	}
}
