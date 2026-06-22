// Package render holds the terminal output formatters for the discovery /
// scroll / browse shapes. Pure formatters over the view-layer structs; no engine
// dependencies. Writers are passed in (not os.Stdout) so output is testable.
package render

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/MoonCaves/rawclaw/internal/retrieve"
	"github.com/MoonCaves/rawclaw/internal/view"
)

// sid8 returns up to the first 8 bytes of s, with no padding when the string
// is shorter.
func sid8(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// fmtMsg formats a single message line: an anchored message is marked with a
// "▶" star, others with a blank, followed by "[role #id] text".
func fmtMsg(m view.ViewMsg) string {
	star := " "
	if m.Anchor {
		star = "▶"
	}
	return fmt.Sprintf("     %s [%s #%d] %s", star, m.Role, m.ID, m.Text)
}

// PrintScroll renders one scroll window.
func PrintScroll(w io.Writer, s *view.ScrollResult) {
	if s == nil || s.View == nil {
		fmt.Fprintln(w, "Nothing to scroll (session or message id not found).")
		return
	}

	v := s.View
	fmt.Fprintf(
		w,
		"━━ %s · %s · around #%d (%d before / %d after) ━━\n",
		sid8(s.SessionID),
		s.Project,
		s.Around,
		v.MessagesBefore,
		v.MessagesAfter,
	)
	for _, m := range v.Window {
		fmt.Fprintln(w, fmtMsg(m))
	}
}

// PrintBrowse renders the recent-sessions list for a project.
func PrintBrowse(w io.Writer, rows []view.BrowseRow, project string) {
	if len(rows) == 0 {
		fmt.Fprintf(w, "No sessions on %s.\n", project)
		return
	}

	fmt.Fprintf(w, "%d most-recent sessions on %s:\n\n", len(rows), project)
	for _, r := range rows {
		fmt.Fprintf(w, "  · %s · %d msgs · %s\n", sid8(r.SessionID), r.N, r.Preview)
	}
}

// bm25Field renders the BM25Rank for a human line. -1 means bm25 did not order
// this hit (a recency overlay, or the multi-term post-resort case where the
// pre-resort bm25 ordinal is not recoverable) — we say so plainly rather than
// printing a misleading number.
func bm25Field(rank int) string {
	if rank < 0 {
		return "n/a"
	}
	return fmt.Sprintf("#%d", rank)
}

// FmtScoreExplain renders one honest ScoreExplain as a compact two-line block.
// It states the regime (Method) and the REAL inputs — it never invents a
// composite score, because RawClaw's ranking has none. The leading rank is the
// hit's actual ordinal position (Final).
func FmtScoreExplain(e retrieve.ScoreExplain) string {
	return fmt.Sprintf(
		"     rank %d · %s\n"+
			"        bm25-order=%s · coverage=%d/%d term(s) · recency-overlay=%s",
		e.Final,
		e.Method,
		bm25Field(e.BM25Rank),
		e.Coverage,
		len(e.Terms),
		yesNo(e.Recency != 0),
	)
}

// yesNo maps a bool to a compact human flag.
func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// PrintDebugSearch renders the scoring explainer: one hit header + its honest
// ScoreExplain block, in result order. It is the clean entrypoint the cli calls
// behind --debug-search. hits[i] is explained by explains[i]; any tail with no
// matching explain is rendered with a "(no breakdown)" marker rather than
// panicking on a short slice.
func PrintDebugSearch(w io.Writer, hits []retrieve.Hit, explains []retrieve.ScoreExplain) {
	if len(hits) == 0 {
		fmt.Fprintln(w, "No matches to explain. (--debug-search shows WHY each hit ranked where it did.)")
		return
	}

	fmt.Fprintf(w, "%d hit(s) · scoring explainer (LLM-free; the REAL ranking, no invented blend):\n\n", len(hits))
	for i, h := range hits {
		iso := h.ISO
		if iso == "" {
			iso = "?"
		}
		fmt.Fprintf(w, "━━ %s · %s ━━\n", iso, sid8(h.SessionID))
		if i < len(explains) {
			fmt.Fprintln(w, FmtScoreExplain(explains[i]))
		} else {
			fmt.Fprintln(w, "     (no breakdown)")
		}
		fmt.Fprintln(w)
	}
}

// debugSearchHit is the JSON DTO for one explained hit — Hit carries no json
// tags, so we project the fields the explainer needs into a tagged shape and
// nest the ScoreExplain (which IS json-tagged) under "score".
type debugSearchHit struct {
	SessionID string                `json:"session_id"`
	ISO       string                `json:"iso"`
	Role      string                `json:"role"`
	Snippet   string                `json:"snippet"`
	Score     retrieve.ScoreExplain `json:"score"`
}

// DebugSearchJSON marshals the explained hits for --debug-search --json. It
// returns indented JSON so an agent (or a human) can read the breakdown. A hit
// with no matching explain gets a zero-value score with an empty Method, which
// is honestly distinguishable from a real regime.
func DebugSearchJSON(hits []retrieve.Hit, explains []retrieve.ScoreExplain) ([]byte, error) {
	out := make([]debugSearchHit, 0, len(hits))
	for i, h := range hits {
		var sc retrieve.ScoreExplain
		if i < len(explains) {
			sc = explains[i]
		}
		out = append(out, debugSearchHit{
			SessionID: h.SessionID,
			ISO:       h.ISO,
			Role:      h.Role,
			Snippet:   h.Snippet,
			Score:     sc,
		})
	}
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal debug-search json: %w", err)
	}
	return b, nil
}
