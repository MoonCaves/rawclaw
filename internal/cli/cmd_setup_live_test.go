package cli

import (
	"strings"
	"testing"
)

func TestSetupLiveCmd_Help(t *testing.T) {
	root := NewRootCmd(BuildInfo{})
	out, err := runCmd(t, root, "", "setup", "live", "--help")
	if err != nil {
		t.Fatalf("setup live --help error: %v", err)
	}
	if !strings.Contains(out, "Provision a remote machine") {
		t.Errorf("output = %q, want help description", out)
	}
	if !strings.Contains(out, "10 universal checks") {
		t.Errorf("output = %q, want 10 universal checks mention", out)
	}
}
