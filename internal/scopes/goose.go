package scopes

import (
	"os"
	"path/filepath"

	"github.com/MoonCaves/rawclaw/internal/source/goose"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// Goose discovers Goose sessions, groups them by recorded cwd,
// ingests each group into its own db (namespaced with prefix "goose-"),
// and returns eager scopes carrying that db + cwd — unioned with
// orphanGooseScopes for retained history after transcript purge.
func Goose(reindex bool) []view.Scope {
	return containerScopes(goose.ID, goose.New(), gooseLabel, reindex)
}

// GooseOrphanScopes surfaces already-indexed goose-*.db files without
// touching goose's filesystem discovery — a pure glob + read of local index
// state, no adapter.Discover() walk. This is what backs the opt-in doc
// promise below: an opted-out Goose() call skips the eager half, but this
// half always runs, so previously indexed history is never hidden by opting
// out later.
func GooseOrphanScopes() []view.Scope {
	return orphanContainerScopes(goose.ID, nil)
}

// GooseOptedIn reports whether goose discovery should run at all. Goose is the
// one source whose discovery WALKS the filesystem and opens every candidate
// SQLite file it finds — expensive on a real machine and pure waste for the
// many users who never ran goose. So it is opt-in: RAWCLAW_GOOSE=1 in the
// environment, or an explicit `--source goose` on the invocation (an explicit
// ask IS the opt-in). Already-indexed goose history is NOT gated by this —
// orphan goose-*.db scopes keep serving archived sessions either way, because
// an archive must never hide what it already holds.
func GooseOptedIn(sourceFilter string) bool {
	if sourceFilter == "goose" {
		return true
	}
	switch os.Getenv("RAWCLAW_GOOSE") {
	case "", "0", "off", "false":
		return false
	}
	return true
}

// RefreshGooseCWD refreshes the Goose index db for a given working dir.
func RefreshGooseCWD(cwd string) {
	refreshContainerCWD(goose.ID, goose.New(), cwd)
}

// gooseLabel formats a human project label for a Goose scope.
func gooseLabel(cwd string) string {
	if cwd == "" {
		return "goose (unscoped)"
	}
	return filepath.Base(cwd)
}
