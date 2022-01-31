package state

import (
	"crypto"
	"fmt"
	"testing"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"
	"github.com/holiman/uint256"
)

func BenchmarkSetNode(b *testing.B) {

	benchData := []struct {
		treeSize      int
		batchSize     int
		trackChanges  bool
		includeRevert bool
	}{
		{treeSize: 1_000_000, batchSize: 50_000, trackChanges: false},
		{treeSize: 1_000_000, batchSize: 50_000, trackChanges: true, includeRevert: false},
		{treeSize: 1_000_000, batchSize: 50_000, trackChanges: true, includeRevert: true},
		{treeSize: 10_000_000, batchSize: 50_000, trackChanges: false},
		{treeSize: 10_000_000, batchSize: 50_000, trackChanges: true, includeRevert: false},
		{treeSize: 10_000_000, batchSize: 50_000, trackChanges: true, includeRevert: true},
	}

	content := &Unit{
		Bearer:    Predicate(test.RandomBytes(32)),
		Data:      TestData(uint64(156165464)),
		StateHash: test.RandomBytes(32),
	}

	for _, bd := range benchData {
		at, _ := New(&Config{HashAlgorithm: crypto.SHA256, RecordingDisabled: !bd.trackChanges})

		for i := 0; i < bd.treeSize; i++ {
			at.setNode(uint256.NewInt(uint64(i)), content)
			if i%bd.batchSize == 0 {
				at.Commit()
			}
		}
		at.Commit()

		idCounter := bd.treeSize

		b.ReportAllocs()
		b.Run(fmt.Sprintf("tree(%s%s)-%d-batch-%d", trackingLabel(bd.trackChanges), incRevertLabel(bd.includeRevert), bd.treeSize, bd.batchSize), func(b *testing.B) {

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

func trackingLabel(trackChanges bool) string {
	if trackChanges {
		return "tracking"
	}
	return "no-track"
}

func incRevertLabel(includeRevert bool) string {
	if includeRevert {
		return "+reverting"
	}
	return ""
}
