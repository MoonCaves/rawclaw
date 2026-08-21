package sources

import (
	"testing"

	"github.com/MoonCaves/rawclaw/internal/model"
	"github.com/MoonCaves/rawclaw/internal/source"
)

type testSource struct{}

func (testSource) Discover() ([]source.Container, error) { return nil, nil }

func (testSource) Messages(source.Container) ([]model.Message, error) { return nil, nil }

func TestRegisteredKeepsAdditionalRuntimeWithoutDuplicates(t *testing.T) {
	orig := source.Registered()
	t.Cleanup(func() {
		source.ResetForTesting(orig)
	})

	const extraID = "future-runtime-test"
	source.Register(source.Registration{
		ID:  extraID,
		New: func() source.Source { return testSource{} },
	})

	first := Registered()
	second := Registered()
	for _, regs := range [][]source.Registration{first, second} {
		counts := map[string]int{}
		for _, reg := range regs {
			counts[reg.ID]++
		}
		for _, id := range []string{"claude", "codex", "antigravity", extraID} {
			if counts[id] != 1 {
				t.Fatalf("registration %q count = %d, want 1", id, counts[id])
			}
		}
	}
}
