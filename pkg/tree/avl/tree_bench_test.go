package avl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkTree_AddNodes(b *testing.B) {
	benchData := []struct {
		treeSize  int
		batchSize int
	}{
		{treeSize: 1_000_000, batchSize: 50_000},
		{treeSize: 10_000_000, batchSize: 50_000},
	}

	for _, bd := range benchData {
		at := newIntTree()

		for i := 0; i < bd.treeSize; i++ {
			require.NoError(b, at.Add(IntKey(i), newIntValue(int64(i))))

		}
		require.NoError(b, at.Commit())
		idCounter := bd.treeSize
		b.ResetTimer()
		b.Run(fmt.Sprintf("tree size=%d; batch-size%d", bd.treeSize, bd.batchSize), func(b *testing.B) {
			b.ReportAllocs()
			for n := 0; n < b.N; n++ {
				for i := idCounter; i < idCounter+bd.batchSize; i++ {
					require.NoError(b, at.Add(IntKey(i), newIntValue(int64(i))))
				}
				require.NoError(b, at.Commit())
				idCounter += bd.batchSize
			}
		})
	}
}
