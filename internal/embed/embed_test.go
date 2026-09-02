package embed_test

import (
	"math"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/embed"
)

func TestNormalize(t *testing.T) {
	got := embed.Normalize([]float64{3, 4})
	if len(got) != 2 || math.Abs(got[0]-0.6) > 1e-12 || math.Abs(got[1]-0.8) > 1e-12 {
		t.Fatalf("Normalize([3 4]) = %v, want [0.6 0.8]", got)
	}
}

func TestNormalizeZeroAndEmpty(t *testing.T) {
	for _, input := range [][]float64{nil, {}, {0, 0}} {
		got := embed.Normalize(input)
		if len(got) != len(input) {
			t.Errorf("Normalize(%v) length = %d, want %d", input, len(got), len(input))
		}
		for i := range got {
			if got[i] != 0 {
				t.Errorf("Normalize(%v)[%d] = %v, want 0", input, i, got[i])
			}
		}
	}
}

func TestNormalizeDoesNotMutateInput(t *testing.T) {
	input := []float64{3, 4}
	embed.Normalize(input)
	if input[0] != 3 || input[1] != 4 {
		t.Fatalf("Normalize mutated input: %v", input)
	}
}
