package lifecycle

import (
	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/parse"
)

// FloorStats are the transcript metrics used by the deterministic routine
// floor. TotalMessages deliberately counts every parsed record: tool results
// and hook banners are part of the conservative message-count guard.
type FloorStats struct {
	SubstantiveHumanTurns int
	TotalMessages         int
	AssistantProseBytes   int
}

// EvaluateMathFloor marks only obviously small sessions as routine. The two
// conditions are intentionally conjunctive: a single substantive prompt does
// not pass when the transcript grew large or produced substantial prose.
func EvaluateMathFloor(messages []model.Message) (bool, FloorStats) {
	stats := FloorStats{TotalMessages: len(messages)}
	for _, message := range messages {
		switch message.Role {
		case "user":
			if parse.IsSubstantive(message.Text) {
				stats.SubstantiveHumanTurns++
			}
		case "assistant":
			prose := parse.StripThinking(parse.StripTools(message.Text))
			stats.AssistantProseBytes += len(prose)
		}
	}

	routine := (stats.SubstantiveHumanTurns == 0 && stats.TotalMessages <= 2) ||
		(stats.SubstantiveHumanTurns <= 1 && stats.TotalMessages <= 4 && stats.AssistantProseBytes <= 400)
	return routine, stats
}
