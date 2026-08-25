package lifecycle

import (
	"strings"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/model"
)

func TestEvaluateMathFloor(t *testing.T) {
	longProse := strings.Repeat("x", 401)
	toolMessages := make([]model.Message, 0, 42)
	toolMessages = append(toolMessages, model.Message{Role: "user", Text: "research this deeply"})
	for i := 0; i < 20; i++ {
		toolMessages = append(toolMessages,
			model.Message{Role: "assistant", Text: "[TOOL:search]"},
			model.Message{Role: "user", Text: "[TOOL_RESULT] result"},
		)
	}

	tests := []struct {
		name     string
		messages []model.Message
		want     bool
		stats    FloorStats
	}{
		{
			name: "bounce",
			messages: []model.Message{
				{Role: "user", Text: "/exit"},
				{Role: "assistant", Text: "Goodbye"},
			},
			want:  true,
			stats: FloorStats{TotalMessages: 2, AssistantProseBytes: 7},
		},
		{
			name: "trivial exchange",
			messages: []model.Message{
				{Role: "user", Text: "what is the date?"},
				{Role: "assistant", Text: "Tuesday"},
			},
			want:  true,
			stats: FloorStats{SubstantiveHumanTurns: 1, TotalMessages: 2, AssistantProseBytes: 7},
		},
		{
			name:     "single prompt research",
			messages: append(toolMessages, model.Message{Role: "assistant", Text: longProse}),
			want:     false,
		},
		{
			name: "two substantive turns",
			messages: []model.Message{
				{Role: "user", Text: "let's debug"},
				{Role: "assistant", Text: "first"},
				{Role: "user", Text: "try this"},
				{Role: "assistant", Text: "second"},
			},
			want: false,
		},
		{
			name: "tool and thinking markers do not count as prose",
			messages: []model.Message{
				{Role: "user", Text: "date?"},
				{Role: "assistant", Text: "[TOOL:clock][THINKING:private] Tuesday"},
			},
			want:  true,
			stats: FloorStats{SubstantiveHumanTurns: 1, TotalMessages: 2},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, stats := EvaluateMathFloor(tt.messages)
			if got != tt.want {
				t.Fatalf("EvaluateMathFloor() = %v, want %v; stats=%+v", got, tt.want, stats)
			}
			if tt.stats != (FloorStats{}) && stats != tt.stats {
				t.Fatalf("stats = %+v, want %+v", stats, tt.stats)
			}
		})
	}
}
