package scopes

import "testing"

// TestGooseOptedIn pins the opt-in gate: goose discovery is OFF by default,
// on when the user sets RAWCLAW_GOOSE, and on when they explicitly ask with
// --source goose. (The expensive walk must never run un-asked; see GooseOptedIn.)
func TestGooseOptedIn(t *testing.T) {
	t.Setenv("RAWCLAW_GOOSE", "")
	if GooseOptedIn("") {
		t.Error("goose discovery ran with no opt-in — the default must be off")
	}
	if !GooseOptedIn("goose") {
		t.Error("--source goose must count as opt-in")
	}
	t.Setenv("RAWCLAW_GOOSE", "1")
	if !GooseOptedIn("") {
		t.Error("RAWCLAW_GOOSE=1 must enable goose discovery")
	}
	t.Setenv("RAWCLAW_GOOSE", "off")
	if GooseOptedIn("") {
		t.Error("RAWCLAW_GOOSE=off must disable goose discovery")
	}
}
