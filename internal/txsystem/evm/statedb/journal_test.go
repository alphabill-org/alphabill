package statedb

import (
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func Test_journal_append(t *testing.T) {
	j := newJournal()
	require.EqualValues(t, 0, j.length())
	require.Len(t, j.getModifiedUnits(), 0)
	addr := common.BytesToAddress(test.RandomBytes(20))
	j.append(&addr)
	require.EqualValues(t, 1, j.length())
	j.append(&addr)
	require.EqualValues(t, 2, j.length())
	// only unique addresses are counted
	require.Len(t, j.getModifiedUnits(), 1)
}

func Test_journal_revert(t *testing.T) {
	j := newJournal()
	j.revert(0)
	j.revert(4)
	require.Len(t, j.getModifiedUnits(), 0)
	addr := common.BytesToAddress(test.RandomBytes(20))
	j.append(&addr)
	require.EqualValues(t, 1, j.length())
	j.append(&addr)
	require.EqualValues(t, 2, j.length())
	addr2 := common.BytesToAddress(test.RandomBytes(20))
	j.append(&addr2)
	require.EqualValues(t, 3, j.length())
	// only unique addresses are counted
	require.Len(t, j.getModifiedUnits(), 2)
	j.revert(3)
	require.EqualValues(t, 3, j.length())
	require.Len(t, j.getModifiedUnits(), 2)
	j.revert(2)
	require.EqualValues(t, 2, j.length())
	require.Len(t, j.getModifiedUnits(), 1)

}
