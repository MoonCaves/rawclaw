package scopes

import (
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
