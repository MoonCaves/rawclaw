package scopes

// The claude-web account scope axis. Imported cloud conversations are
// materialized as raw JSONL transcripts under paths.ClaudeWebRoot()/<account>/
// (see internal/source/claudeweb); each account's transcripts index into their
// OWN cache db, exactly as Codex gives each cwd group its own db — so an account
// is a first-class search scope with zero user input, the account never enters
// the search SQL, and the one-scope-one-db invariant holds. `source` stays flat
// "claude-web"; the account surfaces as the scope's slash-free `acct-<uuid8>`
// label. The cache db is a rebuildable derivative of the transcripts — the raw
// files are the durable truth (Stage 2 rebuilds the cache from them lazily).

import (
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/source"
	"github.com/MoonCaves/rawclaw/internal/source/claudeweb"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// claudeWebDBStem is the shared prefix of every claude-web cache db
// ("claude-web-acct-<uuid8>-<hash8>.db"). orphanClaudeScopes skips this prefix
// (a real Claude project db base is a path-encoding starting with "-", so it
// never collides).
const claudeWebDBStem = claudeweb.ID // "claude-web"

// ClaudeWeb surfaces one scope per imported Claude account, driven by the RAW
// TRANSCRIPT TREE (the source of truth), mirroring Codex: enumerate the
// materialized transcripts, group by account, and (re)build each account's cache
// db from those files via the standard container-index path — so a schema-version
// bump safely REBUILDS from the files (no snowflake). Each scope carries no CWD
// (the export drops the working dir), so it is out of --this-project and present
// on bare/--all; its Project label is the slash-free `acct-<uuid8>`.
//
// FAIL-CLOSED old-format detection: a claude-web cache db with NO backing
// transcript tree is the pre-materialize (slices 01-04) shape — serving it would
// stale-serve today and be EMPTIED by a future rebuild (there are no files to
// rebuild from). Such dbs are NOT served; a clear "re-run import to upgrade"
// warning is surfaced instead.
func ClaudeWeb() []view.Scope {
	ad := claudeweb.New()
	containers, err := ad.Discover()
	if err != nil {
		slog.Warn("scopes: claude-web discover failed", "err", err)
	}

	byAccount := map[string][]source.Container{}
	for _, c := range containers {
		acct := claudeweb.AccountDirName(c.Path)
		byAccount[acct] = append(byAccount[acct], c)
	}
	accts := make([]string, 0, len(byAccount))
	for a := range byAccount {
		accts = append(accts, a)
	}
	sort.Strings(accts)

	out := make([]view.Scope, 0, len(accts))
	liveDBs := make(map[string]struct{}, len(accts))
	for _, acct := range accts {
		dbp := ClaudeWebDBPath(acct)
		liveDBs[dbp] = struct{}{}
		if _, _, ierr := index.EnsureIndexedContainers(dbp, false, byAccount[acct], ad.Messages, claudeweb.ID, ""); ierr != nil {
			slog.Warn("scopes: claude-web index failed", "account", accountSlug(acct), "err", ierr)
			// The db may still hold a prior good index; include the scope so search
			// can open it read-only and degrade gracefully.
		}
		out = append(out, view.Scope{Project: accountSlug(acct), DBP: dbp, Source: claudeweb.ID})
	}
	warnOrphanClaudeWebDBs(liveDBs)
	return out
}

// warnOrphanClaudeWebDBs surfaces a fail-closed warning for every claude-web
// cache db in the cache dir that is NOT backed by a live transcript tree — the
// old DB-only shape. It is not served (only dbs built from the tree above are),
// so it can neither stale-serve nor be rebuilt-empty; the user is told to
// re-import to upgrade.
func warnOrphanClaudeWebDBs(liveDBs map[string]struct{}) {
	entries, _ := filepath.Glob(filepath.Join(store.CacheDir(), claudeWebDBStem+"-acct-*.db"))
	for _, dbp := range entries {
		if _, live := liveDBs[dbp]; live {
			continue
		}
		slog.Warn("claude-web: an old-format index has no backing transcripts and is NOT being searched — re-run `rawclaw import <export>` to upgrade it",
			"db", filepath.Base(dbp))
	}
}

// ClaudeWebDBPath is the cache db for ONE account: "claude-web-acct-<uuid8>-
// <hash8>.db". The readable `acct-<uuid8>` slug is lossy (only 8 chars), so —
// exactly like codexDBPath — a hash of the FULL account key is appended to keep
// the path injective: two accounts sharing the first 8 chars still get distinct
// dbs, so a collision can never mis-route or merge one account's conversations
// into another's. No "/" or ":" ever enters the name. Both `rawclaw import`
// (writer) and ClaudeWeb (reader) resolve through here; `account` is the account
// directory name (the sanitized account uuid, or "unknown").
func ClaudeWebDBPath(account string) string {
	key := claudeWebDBStem + "-" + accountSlug(account) + "-" + cwdHash(account)
	return index.DBPath(key)
}

// accountSlug is the slash-free display/naming segment for an account:
// "acct-<uuid8>", where uuid8 is the first 8 alphanumerics of the account key.
func accountSlug(account string) string {
	if account == "" || account == "unknown" {
		return "acct-unknown" // account-less export (allowed only under the keep default)
	}
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return -1 // drop hyphens, "/", ":", and anything else
		}
	}, account)
	r := []rune(cleaned)
	if len(r) > 8 {
		r = r[:8]
	}
	return "acct-" + string(r)
}

// accountLabelFromDB recovers the `acct-<uuid8>` label from a per-account db
// path by stripping the "claude-web-" prefix and the trailing injective
// "-<hash8>" (mirroring codexOrphanLabel's filename parse). Cheap — no db open.
func accountLabelFromDB(dbp string) string {
	base := strings.TrimSuffix(filepath.Base(dbp), ".db")
	trimmed := strings.TrimPrefix(base, claudeWebDBStem+"-")
	if i := strings.LastIndex(trimmed, "-"); i >= 0 && isHex8(trimmed[i+1:]) {
		trimmed = trimmed[:i] // strip the "-<hash8>" injective suffix
	}
	return trimmed
}
