package agentproto

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

type sessionCand struct {
	SessionID string
	Project   string
	dbp       string
}

type ErrAmbiguousSession struct {
	Prefix     string
	Candidates []sessionCand
}

func (e *ErrAmbiguousSession) Error() string {
	ids := make([]string, 0, len(e.Candidates))
	for _, c := range e.Candidates {
		ids = append(ids, fmt.Sprintf("%s (%s)", sid8(c.SessionID), c.Project))
	}
	return fmt.Sprintf("ambiguous session prefix %q — %d matches: %s; give a longer prefix",
		e.Prefix, len(e.Candidates), strings.Join(ids, ", "))
}

type ErrSessionNotFound struct{ Prefix string }

func (e *ErrSessionNotFound) Error() string {
	return fmt.Sprintf("session %q not found in scope", e.Prefix)
}

func LooksLikeSessionID(s string) bool {
	if len(s) < 8 {
		return false
	}
	hex := 0
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
			hex++
		case r == '-':
		default:
			return false
		}
	}
	return hex >= 8
}

func locateSession(scope []view.Scope, more ScopeFn, session8 string) (dbp, fullSID, proj string, err error) {
	return locateSessionWithCatalog(scope, more, session8, false)
}

func locateSessionGuarded(scope []view.Scope, more ScopeFn, session8 string) (dbp, fullSID, proj string, err error) {
	return locateSessionWithCatalog(scope, more, session8, true)
}

func locateSessionWithCatalog(scope []view.Scope, more ScopeFn, session8 string, guarded bool) (dbp, fullSID, proj string, err error) {
	everywhere := scope == nil
	if !everywhere && len(scope) == 0 {
		return "", "", "", &ErrSessionNotFound{Prefix: session8}
	}
	if cands := oneStoreCands(scope, session8); len(cands) > 0 {
		return decideSession(cands, session8)
	}
	if guarded {
		if cands := catalogCands(scope, session8); len(cands) > 0 {
			return decideSession(cands, session8)
		}
	}
	if everywhere {
		scope = resolveScope(more)
	}
	return decideSession(sweepScopes(scope, session8), session8)
}

func catalogCands(scope []view.Scope, session8 string) []sessionCand {
	hits := paths.ResolveSession(session8)
	if len(hits) == 0 {
		return nil
	}
	projects := scopeProjects(scope)
	var narrowed []view.Scope
	for _, hit := range hits {
		if projects != nil && !slices.Contains(projects, hit.Project) {
			continue
		}
		tdir := paths.ProjectDirOf(hit.Path)
		if tdir == "" {
			return nil
		}
		narrowed = append(narrowed, view.Scope{Project: hit.Project, TDir: tdir})
	}
	if len(narrowed) == 0 {
		return nil
	}
	return sweepScopes(narrowed, session8)
}

func decideSession(cands []sessionCand, session8 string) (dbp, fullSID, proj string, err error) {
	switch len(cands) {
	case 0:
		return "", "", "", &ErrSessionNotFound{Prefix: session8}
	case 1:
		c := cands[0]
		return c.dbp, c.SessionID, c.Project, nil
	default:
		return "", "", "", &ErrAmbiguousSession{Prefix: session8, Candidates: cands}
	}
}

func oneStoreCands(scope []view.Scope, session8 string) []sessionCand {
	dbp := index.ConsolidatedPath()
	con, err := store.ConnectRO(dbp)
	if err != nil {
		slog.Warn("the consolidated store cannot be opened, so this lookup falls back to the per-project indexes; run `rawclaw consolidate` to build it",
			"path", dbp, "err", err)
		return nil
	}
	defer con.Close()

	var filled int
	switch err := con.QueryRow("SELECT EXISTS(SELECT 1 FROM sessions)").Scan(&filled); {
	case err != nil:
		slog.Warn("the consolidated store is missing or unreadable, so this lookup falls back to the per-project indexes; run `rawclaw consolidate` to build it",
			"path", dbp, "err", err)
		return nil
	case filled == 0:
		slog.Warn("the consolidated store is empty, so this lookup falls back to the per-project indexes; run `rawclaw consolidate` to fill it",
			"path", dbp)
		return nil
	}

	projects := scopeProjects(scope)
	rows, err := store.SessionRowsByPrefix(con, session8, false, projects, 2)
	if err != nil {
		slog.Warn("the consolidated store could not be queried, so this lookup falls back to the per-project indexes", "path", dbp, "err", err)
		return nil
	}
	if len(rows) == 0 {
		rows, err = store.SessionRowsByPrefix(con, session8, true, projects, 2)
		if err != nil {
			return nil
		}
	}
	cands := make([]sessionCand, 0, len(rows))
	for _, r := range rows {
		cands = append(cands, sessionCand{SessionID: r.ID, Project: r.Project, dbp: dbp})
	}
	return cands
}

func LocateConsolidatedSession(session8 string) (dbp, fullSID string, err error) {
	dbp, fullSID, _, err = decideSession(oneStoreCands(nil, session8), session8)
	return dbp, fullSID, err
}

func scopeProjects(scope []view.Scope) []string {
	if scope == nil {
		return nil
	}
	out := make([]string, 0, len(scope))
	seen := make(map[string]bool, len(scope))
	for _, sc := range scope {
		if sc.Project == "" {
			return nil
		}
		if !seen[sc.Project] {
			seen[sc.Project] = true
			out = append(out, sc.Project)
		}
	}
	return out
}

func sweepScopes(scope []view.Scope, session8 string) []sessionCand {
	collect := func(excludeSub bool) []sessionCand {
		var cs []sessionCand
		for _, sc := range scope {
			dbpC, _, ensureErr := scopes.Resolve(sc, false)
			if ensureErr != nil {
				continue
			}
			con, openErr := store.ConnectRO(dbpC)
			if openErr != nil {
				continue
			}
			sids, qErr := store.SessionsByPrefix(con, session8, !excludeSub, 2)
			_ = con.Close()
			if qErr != nil {
				continue
			}
			for _, sid := range sids {
				cs = append(cs, sessionCand{SessionID: sid, Project: sc.Project, dbp: dbpC})
			}
		}
		return cs
	}

	cands := collect(true)
	if len(cands) == 0 {
		cands = collect(false)
	}
	return firstRowPerSession(cands)
}

func firstRowPerSession(cands []sessionCand) []sessionCand {
	if len(cands) < 2 {
		return cands
	}
	seen := make(map[string]bool, len(cands))
	out := make([]sessionCand, 0, len(cands))
	for _, c := range cands {
		if seen[c.SessionID] {
			continue
		}
		seen[c.SessionID] = true
		out = append(out, c)
	}
	return out
}

func LocateSession(session8 string, scope []view.Scope, more ScopeFn) (dbPath, fullSID string, err error) {
	dbp, sid, _, err := locateSession(scope, more, normalizeSessionArg(session8))
	return dbp, sid, err
}

func LocateSessionGuarded(session8 string, scope []view.Scope, more ScopeFn) (dbPath, fullSID string, err error) {
	dbp, sid, _, err := locateSessionGuarded(scope, more, normalizeSessionArg(session8))
	return dbp, sid, err
}
