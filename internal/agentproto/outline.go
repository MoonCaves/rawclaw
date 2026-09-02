package agentproto

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/MoonCaves/rawclaw/internal/store"
	"github.com/MoonCaves/rawclaw/internal/timefmt"
	"github.com/MoonCaves/rawclaw/internal/view"
)

const outlineBookendScan = 40
const outlineDispCap = 300

type OutlineResult struct {
	Project      string         `json:"project"`
	SessionID    string         `json:"session_id"`
	ISO          string         `json:"iso"`
	MessageCount int            `json:"message_count"`
	Start        []view.ViewMsg `json:"start"`
	End          []view.ViewMsg `json:"end"`
	MidCount     int            `json:"mid_count"`
	Topics       []string       `json:"topics,omitempty"`
	Subagents    []SubagentInfo `json:"subagents,omitempty"`
}

type OutlineOpts struct {
	IncludeTools     bool
	IncludeThinking  bool
	IncludeSubagents bool
	ScopeFallback    ScopeFn
}

func Outline(session8 string, scope []view.Scope, includeTools bool) (*OutlineResult, error) {
	return outline(session8, scope, nil, OutlineOpts{IncludeTools: includeTools})
}

func OutlineWith(session8 string, scope []view.Scope, opts OutlineOpts) (*OutlineResult, error) {
	return outline(session8, scope, opts.ScopeFallback, opts)
}

func outline(session8 string, scope []view.Scope, more ScopeFn, opts OutlineOpts) (*OutlineResult, error) {
	session8 = normalizeSessionArg(session8)

	dbp, fullSID, proj, locErr := locateSession(scope, more, session8)
	if locErr != nil {
		return nil, locErr
	}

	con, err := store.ConnectRO(dbp)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", dbp, err)
	}
	defer con.Close()

	iso, nmsg := sessionMeta(con, fullSID)

	startRows, err := store.BookendMessages(con, fullSID, 0, false, true, outlineBookendScan)
	if err != nil {
		return nil, fmt.Errorf("outline start rows: %w", err)
	}
	endRows, err := store.BookendMessages(con, fullSID, 0, false, false, outlineBookendScan)
	if err != nil {
		return nil, fmt.Errorf("outline end rows: %w", err)
	}

	startRows = view.FilterDisplayableWith(startRows, OutlineBookend, opts.IncludeTools, opts.IncludeThinking)
	endRows = view.FilterDisplayableWith(endRows, OutlineBookend, opts.IncludeTools, opts.IncludeThinking)

	startIDs := map[int]struct{}{}
	for _, r := range startRows {
		startIDs[r.ID] = struct{}{}
	}

	endMsgs := []store.Msg{}
	for _, r := range view.Reversed(endRows) {
		if _, dup := startIDs[r.ID]; dup {
			continue
		}
		endMsgs = append(endMsgs, r)
	}

	startOut := view.RenderMsgsWith(startRows, opts.IncludeTools, opts.IncludeThinking, outlineDispCap)
	endOut := view.RenderMsgsWith(endMsgs, opts.IncludeTools, opts.IncludeThinking, outlineDispCap)

	lastStartID := 0
	if len(startRows) > 0 {
		lastStartID = startRows[len(startRows)-1].ID
	}
	firstEndID := 0
	if len(endMsgs) > 0 {
		firstEndID = endMsgs[0].ID
	}
	midCount, cErr := store.CountMessagesBetween(con, fullSID, lastStartID, firstEndID)
	if cErr != nil {
		midCount = 0
	}

	var topics []string
	if segs, terr := store.TopicsForSession(con, fullSID); terr == nil {
		for _, s := range segs {
			if s.Topic != "" {
				topics = append(topics, s.Topic)
			}
		}
	}

	var subagents []SubagentInfo
	if opts.IncludeSubagents {
		if subs, err := store.SubagentsForSession(con, fullSID); err == nil {
			for _, s := range subs {
				subagents = append(subagents, SubagentInfo{
					SessionID:    s.ID,
					MessageCount: s.MessageCount,
				})
			}
		}
	}

	return &OutlineResult{
		Project:      proj,
		SessionID:    fullSID,
		ISO:          iso,
		MessageCount: nmsg,
		Start:        startOut,
		End:          endOut,
		MidCount:     midCount,
		Topics:       topics,
		Subagents:    subagents,
	}, nil
}

func sessionMeta(con *sql.DB, fullSID string) (iso string, nmsg int) {
	lastTS, mc, ok := store.SessionMeta(con, fullSID)
	if !ok {
		return "", 0
	}
	if lastTS != 0 {
		iso = timefmt.UTC(time.Unix(int64(lastTS), 0))
	}
	return iso, mc
}
