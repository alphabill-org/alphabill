package statedb

import (
	"testing"

	abstate "github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/testutils"
	"github.com/alphabill-org/alphabill/testutils/logger"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func Test_journal_append(t *testing.T) {
	j := newJournal()
	require.EqualValues(t, 0, j.length())
	require.Len(t, j.getModifiedUnits(), 0)
	addr := common.BytesToAddress(test.RandomBytes(20))
	j.append(accountChange{account: &addr})
	require.EqualValues(t, 1, j.length())
	j.append(accountChange{account: &addr})
	require.EqualValues(t, 2, j.length())
	// only unique addresses are counted
	require.Len(t, j.getModifiedUnits(), 1)
}

func Test_journal_revert(t *testing.T) {
	tree := abstate.NewEmptyState()
	s := NewStateDB(tree, logger.New(t))
	s.journal.revert(s, 0)
	s.journal.revert(s, 4)
	require.Len(t, s.journal.getModifiedUnits(), 0)
	addr := common.BytesToAddress(test.RandomBytes(20))
	s.journal.append(accountChange{account: &addr})
	require.EqualValues(t, 1, s.journal.length())
	s.journal.append(accountChange{account: &addr})
	require.EqualValues(t, 2, s.journal.length())
	addr2 := common.BytesToAddress(test.RandomBytes(20))
	s.journal.append(accountChange{account: &addr2})
	require.EqualValues(t, 3, s.journal.length())
	s.AddRefund(10)
	require.EqualValues(t, 4, s.journal.length())
	var key, val common.Hash
	key.SetBytes([]byte{1, 2, 3})
	val.SetBytes([]byte{5, 6, 7})
	s.SetTransientState(addr, key, val)
	require.EqualValues(t, 5, s.journal.length())
	// only unique addresses are counted
	require.Len(t, s.journal.getModifiedUnits(), 2)
	s.journal.revert(s, 3)
	// refund and transient storage values have been reverted
	require.EqualValues(t, 0, s.refund)
	require.EqualValues(t, common.Hash{}, s.GetTransientState(addr, key))
	require.EqualValues(t, 3, s.journal.length())
	require.Len(t, s.journal.getModifiedUnits(), 2)
	s.journal.revert(s, 2)
	require.EqualValues(t, 2, s.journal.length())
	require.Len(t, s.journal.getModifiedUnits(), 1)
}
