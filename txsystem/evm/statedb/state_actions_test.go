package statedb

import (
	"testing"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/holiman/uint256"

	"github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/state"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestCreateAccountAndAddCredit(t *testing.T) {
	tr := state.NewEmptyState()
	address := common.BytesToAddress(test.RandomBytes(20))
	balance := uint256.NewInt(123)
	ownerPredicate := templates.AlwaysFalseBytes()
	// add credit unit to state tree
	err := tr.Apply(CreateAccountAndAddCredit(address, ownerPredicate, balance, 3))
	require.NoError(t, err)
	// verify result
	unitID := address.Bytes()
	u, err := tr.GetUnit(unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	_, ok := u.Data().(*StateObject)
	require.True(t, ok)
	stateDB := NewStateDB(tr, logger.New(t))
	require.Equal(t, balance, stateDB.GetBalance(address))
	abLink := stateDB.GetAlphaBillData(address)
	require.EqualValues(t, 3, abLink.Timeout)
	require.EqualValues(t, 0, abLink.Counter)
	require.EqualValues(t, ownerPredicate, abLink.OwnerPredicate)
}

func TestUpdateEthAccountAddCredit(t *testing.T) {
	tr := state.NewEmptyState()
	address := common.BytesToAddress(test.RandomBytes(20))
	balance := uint256.NewInt(100)
	// add credit unit to state tree
	err := tr.Apply(CreateAccountAndAddCredit(address, templates.AlwaysFalseBytes(), balance, 3))
	require.NoError(t, err)
	// update
	unitID := address.Bytes()
	err = tr.Apply(UpdateEthAccountAddCredit(unitID, balance, 2, templates.AlwaysTrueBytes()))
	require.NoError(t, err)
	stateDB := NewStateDB(tr, logger.New(t))
	require.Equal(t, uint256.NewInt(200), stateDB.GetBalance(address))
	abLink := stateDB.GetAlphaBillData(address)
	require.EqualValues(t, 3, abLink.Timeout)
	require.EqualValues(t, 1, abLink.Counter)
	require.EqualValues(t, templates.AlwaysTrueBytes(), abLink.OwnerPredicate)
}

func TestSetAccountBalance(t *testing.T) {
	tr := state.NewEmptyState()
	address := common.BytesToAddress(test.RandomBytes(20))
	balance := uint256.NewInt(100)
	// add credit unit to state tree
	err := tr.Apply(CreateAccountAndAddCredit(address, templates.AlwaysFalseBytes(), balance, 3))
	require.NoError(t, err)
	// update
	unitID := address.Bytes()
	err = tr.Apply(SetBalance(unitID, uint256.NewInt(300)))
	require.NoError(t, err)
	stateDB := NewStateDB(tr, logger.New(t))
	require.Equal(t, uint256.NewInt(300), stateDB.GetBalance(address))
}
