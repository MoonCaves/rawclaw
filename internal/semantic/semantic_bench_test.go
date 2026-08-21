package semantic

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"testing"

	"github.com/MoonCaves/rawclaw/internal/store"
)

// BenchmarkCosineKNN benchmarks brute-force cosine KNN computation in pure Go
// across varying message counts (1k, 10k, 50k) and dimensions (384, 1536).
func BenchmarkCosineKNN(b *testing.B) {
	cases := []struct {
		label string
		count int
		dim   int
	}{
		{"1k_384d", 1000, 384},
		{"1k_1536d", 1000, 1536},
		{"10k_384d", 10000, 384},
		{"10k_1536d", 10000, 1536},
		{"50k_384d", 50000, 384},
		{"50k_1536d", 50000, 1536},
	}

	for _, tc := range cases {
		b.Run(tc.label, func(b *testing.B) {
			rng := rand.New(rand.NewSource(42))

			qvec := make([]float64, tc.dim)
			for i := range qvec {
				qvec[i] = float64(rng.Float32()*2.0 - 1.0)
			}

			// Pre-pack vector bytes for all rows
			vecBuf := make([]byte, tc.count*tc.dim*4)
			for i := 0; i < len(vecBuf); i += 4 {
				f := rng.Float32()*2.0 - 1.0
				binary.LittleEndian.PutUint32(vecBuf[i:], math.Float32bits(f))
			}

			stride := tc.dim * 4
			rows := make([]store.VecRow, tc.count)
			for i := 0; i < tc.count; i++ {
				offset := i * stride
				rows[i] = store.VecRow{
					SessionID:   fmt.Sprintf("sess-%05d", i/10),
					ContentHash: fmt.Sprintf("hash-%016x", i),
					MsgID:       i + 1,
					Vec:         vecBuf[offset : offset+stride],
				}
			}

			b.ResetTimer()
			b.ReportAllocs()
			for b.Loop() {
				res := knn(qvec, rows, 20)
				if len(res) == 0 {
					b.Fatal("unexpected empty KNN results")
				}
			}
		})
	}
}
