package cli

import (
	"log/slog"
	"os"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/sources"
)

// refreshActiveSessions ensures that recent/active sessions (including the
// caller's current session, if any) have their latest appended turns folded
// into the consolidated store before search or browse executes.
func refreshActiveSessions(currentSession string) {
	if strings.EqualFold(os.Getenv("RAWCLAW_ACTIVE_INGEST"), "off") {
		return
	}
	if currentSession == "" || strings.EqualFold(currentSession, "off") {
		return
	}
	regs := sources.Registered()
	refreshOneSession(currentSession, regs)
}

func refreshOneSession(sessionArg string, regs []source.Registration) bool {
	match, ok := catalogIngestSource(sessionArg, regs)
	if !ok {
		matches, err := resolveIngestMatches(sessionArg, regs)
		if err != nil || len(matches) == 0 {
			return false
		}
		match = matches[0]
	}

	rawPath := backingPath(match.container.Path)
	st, err := os.Stat(rawPath)
	if err != nil || st.Size() == 0 {
		return false
	}

	n, err := ingestContainerWithRetry(match)
	if err != nil {
		slog.Debug("active session refresh failed", "session", match.container.ID, "err", err)
		return false
	}
	if n > 0 {
		slog.Debug("active session refreshed", "session", match.container.ID, "new_messages", n)
	}
	return true
}
