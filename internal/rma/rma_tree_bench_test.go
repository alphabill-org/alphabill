package rma

import (
	"crypto"
	"fmt"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/holiman/uint256"
)

func BenchmarkSetNode(b *testing.B) {

	benchData := []struct {
		treeSize      int
		batchSize     int
		includeRevert bool
	}{
		{treeSize: 1_000_000, batchSize: 50_000, includeRevert: false},
		{treeSize: 1_000_000, batchSize: 50_000, includeRevert: true},
		{treeSize: 10_000_000, batchSize: 50_000, includeRevert: false},
		{treeSize: 10_000_000, batchSize: 50_000, includeRevert: true},
	}

	content := &Unit{
		Bearer:    Predicate(test.RandomBytes(32)),
		Data:      TestData(uint64(156165464)),
		StateHash: test.RandomBytes(32),
	}

	for _, bd := range benchData {
		at, _ := New(&Config{HashAlgorithm: crypto.SHA256})

		for i := 0; i < bd.treeSize; i++ {
			at.setNode(uint256.NewInt(uint64(i)), content)
			if i%bd.batchSize == 0 {
				at.Commit()
			}
		}
		at.Commit()

		idCounter := bd.treeSize

		b.ReportAllocs()
		b.Run(fmt.Sprintf("tree(%s)-%d-batch-%d", incRevertLabel(bd.includeRevert), bd.treeSize, bd.batchSize), func(b *testing.B) {

			for n := 0; n < b.N; n++ {
				for i := idCounter; i < idCounter+bd.batchSize; i++ {
					at.setNode(uint256.NewInt(uint64(i)), content)
				}
				if bd.includeRevert {
					at.Revert()
				}
				// Always commit after the batch
				at.Commit()
				idCounter += bd.batchSize
			}
		})
	}
}

func incRevertLabel(includeRevert bool) string {
	if includeRevert {
		return "+reverting"
	}
	return ""
}
