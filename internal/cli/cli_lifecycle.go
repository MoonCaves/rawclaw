package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/MoonCaves/rawclaw/internal/adapters"
	"github.com/MoonCaves/rawclaw/internal/embed"
	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/semantic"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

func runReindexVectors(ctx context.Context, w io.Writer, o *Options) error {
	if !index.FTS5OK() {
		fmt.Fprintln(w, "--reindex-vectors needs FTS5.")
		return nil
	}
	emb := adapters.GetEmbedder()
	if emb == nil {
		fmt.Fprintln(w, "No embedder configured. Set RAWCLAW_EMBED_ENDPOINT (+ RAWCLAW_EMBED_MODEL), e.g.\n"+
			"  export RAWCLAW_EMBED_ENDPOINT=http://localhost:11434/api/embeddings\n"+
			"  export RAWCLAW_EMBED_MODEL=nomic-embed-text")
		return nil
	}

	var scope []view.Scope
	if o.ThisProject {
		sc, _, ok := thisScope(w, o)
		if ok {
			scope = sc
		}
	} else {
		scope = allScope(ctx, o.Source, o.Reindex)
	}

	// Index each scope FIRST, for its side effect only: resolving a scope folds
	// its rows into the consolidated store. The vectors are not written here.
	for _, s := range scope {
		if _, _, err := scopes.Resolve(s, false); err != nil {
			fmt.Fprintf(w, "  %s: skipped (%s)\n", s.Project, err)
		}
	}

	// Then embed ONCE, against the store a search actually opens. Embedding each
	// scope's own db instead is what made this command a no-op: default search
	// reads the consolidated store, so vectors written per project were never
	// read by anything and the command still printed a success line.
	total, err := reindexConsolidated(ctx, emb)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "\nSemantic index updated: +%d new vectors in %s. Run a normal search to use it (RRF-fused).\n",
		total, index.ConsolidatedPath())
	return nil
}

// reindexConsolidated embeds every not-yet-vectored message in the consolidated
// store (open read-write → vector index → close). One store, one pass: the
// per-project databases are a staging cache the reader never opens.
func reindexConsolidated(ctx context.Context, emb embed.Embedder) (int, error) {
	con, err := store.ConnectRW(index.ConsolidatedPath())
	if err != nil {
		return 0, err
	}
	defer con.Close()
	return semantic.VecIndex(ctx, con, emb, 0)
}

// statsJSON is one project's stats record, in emit order.
type statsJSON struct {
	Sessions  int    `json:"sessions"`
	Subagents int    `json:"subagents"`
	Messages  int    `json:"messages"`
	User      int    `json:"user"`
	Assistant int    `json:"assistant"`
	First     string `json:"first"`
	Last      string `json:"last"`
}

func toStatsJSON(s store.CorpusStats) statsJSON {
	return statsJSON{s.Sessions, s.Subagents, s.Messages, s.User, s.Assistant, s.First, s.Last}
}

// runStats prints the corpus overview for this project, or the all-projects aggregate
// under --all.
func runStats(ctx context.Context, w io.Writer, o *Options) error {
	if !index.FTS5OK() {
		fmt.Fprintln(w, "--stats needs FTS5.")
		return nil
	}

	if (o.All || o.Source != "") && !o.ThisProject {
		return runStatsFleet(ctx, w, o)
	}

	sc, td, ok := thisScope(w, o)
	if !ok {
		return nil
	}
	_ = sc
	dbp, _, _, err := index.EnsureIndexed(td, o.Reindex)
	if err != nil {
		return fmt.Errorf("stats ensure-indexed: %w", err)
	}
	s, err := store.GetCorpusStats(dbp)
	if err != nil {
		return fmt.Errorf("stats corpus: %w", err)
	}
	if o.JSON {
		return EmitJSON(w, struct {
			Scope   string `json:"scope"`
			Project string `json:"project"`
			statsJSON
		}{"project", paths.ProjectLabel(td), toStatsJSON(s)})
	}
	fmt.Fprintf(w, "%s — session stats\n\n", paths.ProjectLabel(td))
	fmt.Fprintf(w, "  sessions   %d  (+%d subagent threads)\n", s.Sessions, s.Subagents)
	fmt.Fprintf(w, "  messages   %d  (%d user / %d assistant)\n", s.Messages, s.User, s.Assistant)
	fmt.Fprintf(w, "  span       %s → %s\n", orQ(s.First), orQ(s.Last))
	return nil
}

// projectStat is a per-project stats row carrying its project label.
type projectStat struct {
	statsJSON
	Project string `json:"project"`
}

// runStatsFleet computes and prints the --all stats aggregate across all projects.
func runStatsFleet(ctx context.Context, w io.Writer, o *Options) error {
	tot := store.CorpusStats{}
	nProjects := 0
	var per []projectStat

	for _, sc := range allScope(ctx, o.Source, o.Reindex) {
		dbp, _, err := scopes.Resolve(sc, o.Reindex)
		if err != nil {
			continue
		}
		s, err := store.GetCorpusStats(dbp)
		if err != nil {
			continue
		}
		nProjects++
		tot.Sessions += s.Sessions
		tot.Subagents += s.Subagents
		tot.Messages += s.Messages
		tot.User += s.User
		tot.Assistant += s.Assistant
		if s.First != "" && (tot.First == "" || s.First < tot.First) {
			tot.First = s.First
		}
		if s.Last != "" && s.Last > tot.Last {
			tot.Last = s.Last
		}
		per = append(per, projectStat{toStatsJSON(s), sc.Project})
	}

	if o.JSON {
		type totalJSON struct {
			Projects int `json:"projects"`
			statsJSON
		}
		return EmitJSON(w, struct {
			Scope    string        `json:"scope"`
			Total    totalJSON     `json:"total"`
			Projects []projectStat `json:"projects"`
		}{"all", totalJSON{nProjects, toStatsJSON(tot)}, per})
	}

	fmt.Fprintf(w, "RawClaw corpus — %d projects\n\n", nProjects)
	fmt.Fprintf(w, "  sessions   %d  (+%d subagent threads)\n", tot.Sessions, tot.Subagents)
	fmt.Fprintf(w, "  messages   %d  (%d user / %d assistant)\n", tot.Messages, tot.User, tot.Assistant)
	fmt.Fprintf(w, "  span       %s → %s\n", orQ(tot.First), orQ(tot.Last))
	return nil
}

// runBrowse handles the no-query case: list recent sessions for this project,
// or — under --all or a path scope — across the projects those flags select
// (the same scope enumeration search uses). An explicit --this-project wins
// over --all, same precedence runStats applies.
//
// --include-path / --exclude-path are structural SCOPE flags: they bound which
// projects a run covers, and they compose with the rest rather than being
// consumed by one shape. They name projects by working dir, so — like search,
// whose universe is every project unless --this-project — a path flag browses
// ACROSS projects; --this-project first narrows the universe to the cwd and the
// predicate then applies to that one project alone. Browse used to accept both
// flags and browse the cwd anyway: `rawclaw --include-path myproject` answered
// from /tmp with two throwaway /tmp sessions under the header "2 most-recent
// sessions on tmp", i.e. a different question's answer wearing the caller's
// flags. A flag accepted and silently ignored is the worst outcome for an agent
// caller, which trusts it and moves on wrong.
//
// --source is the same kind of structural scope flag: `rawclaw --source
// antigravity` used to drop the flag and browse the cwd's Claude sessions —
// so it routes through the scoped path too, where allScope already filters
// the universe by runtime.
