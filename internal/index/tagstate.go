package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/MoonCaves/rawclaw/internal/store"
)

type tagState struct {
	segments map[string][]store.TopicSegment
	verdicts []store.Verdict
}

func readTagState(dbp string) (tagState, error) {
	state := tagState{segments: make(map[string][]store.TopicSegment)}
	if _, err := os.Stat(dbp); errors.Is(err, os.ErrNotExist) {
		return state, nil
	} else if err != nil {
		return state, err
	}
	con, err := store.ConnectRO(dbp)
	if err != nil {
		return state, err
	}
	defer con.Close()

	rows, err := con.Query(`SELECT session_id,start_uuid,end_uuid,topic,summary,tagged_at,origin_machine FROM topic_segment ORDER BY id`)
	if err != nil {
		return state, nil // pre-topic-schema store
	}
	defer rows.Close()
	for rows.Next() {
		var s store.TopicSegment
		var end, topic, summary, origin sql.NullString
		var taggedAt sql.NullFloat64
		if err := rows.Scan(&s.SessionID, &s.StartUUID, &end, &topic, &summary, &taggedAt, &origin); err != nil {
			return state, fmt.Errorf("scan topic segment: %w", err)
		}
		s.EndUUID, s.Topic, s.Summary = end.String, topic.String, summary.String
		s.TaggedAt, s.OriginMachine = taggedAt.Float64, origin.String
		state.segments[s.SessionID] = append(state.segments[s.SessionID], s)
	}
	if err := rows.Err(); err != nil {
		return state, fmt.Errorf("iterate topic segments: %w", err)
	}

	vrows, err := con.Query(`SELECT session_id,verdict,source,origin_machine,tagged_at FROM session_verdict`)
	if err != nil {
		return state, nil
	}
	defer vrows.Close()
	for vrows.Next() {
		var v store.Verdict
		var origin sql.NullString
		var taggedAt sql.NullFloat64
		if err := vrows.Scan(&v.SessionID, &v.Verdict, &v.Source, &origin, &taggedAt); err != nil {
			return state, fmt.Errorf("scan session verdict: %w", err)
		}
		v.OriginMachine, v.TaggedAt = origin.String, taggedAt.Float64
		state.verdicts = append(state.verdicts, v)
	}
	return state, vrows.Err()
}

func restoreTagState(con *sql.DB, state tagState) error {
	for sessionID, segments := range state.segments {
		if err := store.ReplaceSessionSegments(con, sessionID, segments); err != nil {
			return err
		}
	}
	for _, verdict := range state.verdicts {
		if err := store.UpsertVerdict(con, verdict); err != nil {
			return err
		}
	}
	return nil
}
