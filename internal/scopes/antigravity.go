package scopes

import (
	"github.com/MoonCaves/rawclaw/internal/source/antigravity"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// Antigravity discovers Antigravity sessions, groups them by recorded cwd,
// ingests each group into its own db (namespaced with prefix "antigravity-"),
// and returns eager scopes carrying that db + cwd — unioned with
// orphanAntigravityScopes for retained history after transcript purge.
func Antigravity(reindex bool) []view.Scope {
	return containerScopes(antigravity.ID, antigravity.New(), antigravityLabel, reindex)
}

// RefreshAntigravityCWD refreshes the Antigravity index db for a given working dir.
func RefreshAntigravityCWD(cwd string) {
	refreshContainerCWD(antigravity.ID, antigravity.New(), cwd)
}

func antigravityDBPath(cwd string) string {
	return containerDBPath(antigravity.ID, cwd)
}

func antigravityLabel(cwd string) string {
	return defaultContainerLabel(antigravity.ID, cwd)
}

func antigravityOrphanLabel(dbFileName string) string {
	return containerOrphanLabel(antigravity.ID, dbFileName)
}
