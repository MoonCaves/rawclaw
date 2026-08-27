package cli

import (
	"strings"
	"testing"
)

func TestCloseoutHelpAndRecovery(t *testing.T) {
	help, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "closeout", "--help")
	if err != nil || !strings.Contains(help, "closeout <full-session-id>") ||
		!strings.Contains(help, "rawclaw tag-prep <full-session-id>") ||
		!strings.Contains(help, "rawclaw tag-write <full-session-id>") {
		t.Fatalf("closeout help: err=%v output=%s", err, help)
	}

	sid := "11111111-2222-3333-4444-555555555555"
	out, err := runCmd(t, NewRootCmd(BuildInfo{}), "", "closeout", sid)
	if err != nil || !strings.Contains(out, "rawclaw tag-prep "+sid) {
		t.Fatalf("closeout recovery: err=%v output=%s", err, out)
	}
}
