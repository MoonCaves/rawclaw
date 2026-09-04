package agentproto

import (
	"database/sql"
	"fmt"
	"io"
	"sort"

	"github.com/MoonCaves/rawclaw/internal/index"
	"github.com/MoonCaves/rawclaw/internal/paths"
	"github.com/MoonCaves/rawclaw/internal/query"
	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/scopes"
	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/view"
)

type TopicHit struct {
	Topic   string `json:"topic"`
	Project string `json:"project"`
	ReadRef string `json:"read_ref"`
	Routine bool   `json:"routine,omitempty"`
}

type TopicsResult struct {
	Query string     `json:"query"`
	Hits  []TopicHit `json:"hits"`
	Note  string     `json:"note,omitempty"`
}

const topicsEmptyNote = "no topics tagged yet — a session is tagged via the rawclaw-topic-tagger subagent"

type TopicsOpts struct {
	Limit         int
	Project       string
	Projects      []string
	ProjectDir    string
	IncludePath   string
	ScopeFallback ScopeFn
}

func Topics(query string, scope []view.Scope, opts TopicsOpts) (TopicsResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}

	if con, _, err := index.OpenConsolidated(); err == nil {
		defer con.Close()
		if res, ok := topicsFromStore(con, query, limit, opts); ok {
			return res, nil
		}
	}
	return topicsByFanOut(query, scope, limit, opts)
}

func topicsFromStore(con *sql.DB, query string, limit int, opts TopicsOpts) (TopicsResult, bool) {
	if !store.TopicRowsExist(con) {
		return TopicsResult{}, false
	}
	projects, narrowed, err := resolveStoreProjects(con, opts.Project, opts.Projects, opts.ProjectDir, opts.IncludePath, "")
	if err != nil || (narrowed && len(projects) == 0) {
		return TopicsResult{}, false
	}

	thits, err := store.MatchTopics(con, query, limit, projects)
	if err != nil {
		return TopicsResult{}, false
	}
	routines, _ := store.RoutineVerdictSet(con)
	hits := []TopicHit{}
	for _, h := range thits {
		uuid := store.MessageUUID(con, h.MsgID)
		if uuid == "" {
			continue
		}
		hits = append(hits, TopicHit{
			Topic:   h.Topic,
			Project: h.Project,
			ReadRef: fmtRef(h.SessionID, uuid),
			Routine: routines[h.SessionID],
		})
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Routine != hits[j].Routine {
			return !hits[i].Routine && hits[j].Routine
		}
		return false
	})
	if len(hits) == 0 {
		return TopicsResult{}, false
	}
	return TopicsResult{Query: query, Hits: hits}, true
}

func topicsByFanOut(query string, scope []view.Scope, limit int, opts TopicsOpts) (TopicsResult, error) {
	if scope == nil {
		scope = resolveScope(opts.ScopeFallback)
	}
	if opts.IncludePath != "" {
		scope = scopes.FilterByPath(scope, opts.IncludePath, "")
	}

	hits := []TopicHit{}
	anyTopics := false
	for _, sc := range scope {
		if opts.Project != "" && sc.Project != opts.Project {
			continue
		}
		dbp, _, err := scopes.Resolve(sc, false)
		if err != nil {
			continue
		}
		con, openErr := store.ConnectRO(dbp)
		if openErr != nil {
			continue
		}
		if err := store.EnsureTopicSchema(con); err != nil {
			_ = con.Close()
			continue
		}
		if store.TopicRowsExist(con) {
			anyTopics = true
		}
		topicLimit := max(limit*8, 30)
		thits, _ := store.MatchTopics(con, query, topicLimit, nil)

		routines, _ := store.RoutineVerdictSet(con)
		seenTopic := map[string]struct{}{}
		kept := 0
		for _, h := range thits {
			uuid := store.MessageUUID(con, h.MsgID)
			if uuid == "" {
				continue
			}
			root := retrieve.LineageRoot(con, h.SessionID)
			if root == "" {
				root = h.SessionID
			}
			key := root + "\x00" + h.Topic
			if _, dup := seenTopic[key]; dup {
				continue
			}
			seenTopic[key] = struct{}{}
			hits = append(hits, TopicHit{
				Topic:   h.Topic,
				Project: sc.Project,
				ReadRef: fmtRef(h.SessionID, uuid),
				Routine: routines[h.SessionID],
			})
			kept++
			if kept == limit {
				break
			}
		}
		_ = con.Close()
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Routine != hits[j].Routine {
			return !hits[i].Routine && hits[j].Routine
		}
		return false
	})

	res := TopicsResult{Query: query, Hits: hits}
	if len(hits) == 0 && !anyTopics {
		res.Note = topicsEmptyNote
	}
	return res, nil
}

func resolveStoreProjects(con *sql.DB, project string, projectsFilter []string, projectDir, includePath, excludePath string) (projects []string, narrowed bool, err error) {
	if project == "" && len(projectsFilter) == 0 && projectDir == "" && includePath == "" && excludePath == "" {
		return nil, false, nil
	}
	scopeRows, err := store.DistinctScopes(con)
	if err != nil {
		return nil, true, err
	}

	keep := map[string]bool{}
	for _, sr := range scopeRows {
		keep[sr.Project] = false
	}
	if includePath != "" || excludePath != "" {
		pred := query.PathPredicate(includePath, excludePath)
		for _, sr := range scopeRows {
			if pred(sr.CWD) {
				keep[sr.Project] = true
			}
		}
	} else if projectDir != "" {
		targetDir := paths.Realpath(paths.ExpandHome(projectDir))
		gitRoot := paths.GitRoot(targetDir)
		for _, sr := range scopeRows {
			scCWD := paths.Realpath(sr.CWD)
			matched := false
			if gitRoot != "" {
				if scGitRoot := paths.GitRoot(scCWD); scGitRoot != "" && scGitRoot == gitRoot {
					matched = true
				}
			}
			if !matched && scCWD != "" && scCWD == targetDir {
				matched = true
			}
			if matched {
				keep[sr.Project] = true
			}
		}
		hasAny := false
		for _, v := range keep {
			if v {
				hasAny = true
				break
			}
		}
		if !hasAny {
			lbl := paths.ProjectLabel(targetDir)
			for k := range keep {
				if k == lbl || (project != "" && k == project) {
					keep[k] = true
				}
			}
		}
	} else {
		for k := range keep {
			keep[k] = true
		}
	}
	if len(projectsFilter) > 0 {
		projSet := make(map[string]bool, len(projectsFilter))
		for _, p := range projectsFilter {
			projSet[p] = true
		}
		for k := range keep {
			if !projSet[k] {
				keep[k] = false
			}
		}
	} else if project != "" && projectDir == "" {
		for k := range keep {
			if k != project {
				keep[k] = false
			}
		}
	}

	for _, sr := range scopeRows {
		if keep[sr.Project] {
			keep[sr.Project] = false
			projects = append(projects, sr.Project)
		}
	}
	sort.Strings(projects)
	return projects, true, nil
}

func TopicsAndRender(w io.Writer, query string, scope []view.Scope, opts TopicsOpts, wantJSON bool) error {
	result, err := Topics(query, scope, opts)
	if err != nil {
		return err
	}
	if wantJSON {
		return emit(w, result)
	}
	renderTopics(w, result)
	return nil
}

func renderTopics(w io.Writer, r TopicsResult) {
	if len(r.Hits) == 0 {
		if r.Note != "" {
			fmt.Fprintf(w, "%s\n", r.Note)
			return
		}
		fmt.Fprintf(w, "No topics matching '%s'. Try a different concept word, or widen scope.\n", r.Query)
		return
	}
	fmt.Fprintf(w, "%d topic(s) matching '%s':\n\n", len(r.Hits), r.Query)
	for _, h := range r.Hits {
		routine := ""
		if h.Routine {
			routine = " · routine"
		}
		fmt.Fprintf(w, "  %s  ·  %s%s  ·  read ref=%s\n", h.Topic, h.Project, routine, h.ReadRef)
	}
}
