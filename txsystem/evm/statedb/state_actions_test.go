package statedb

import (
	"math/big"
	"testing"

	"github.com/alphabill-org/alphabill/api/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/state"
	test "github.com/alphabill-org/alphabill/validator/pkg/testutils"
	"github.com/alphabill-org/alphabill/validator/pkg/testutils/logger"
	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestCreateAccountAndAddCredit(t *testing.T) {
	tr := state.NewEmptyState()
	address := common.BytesToAddress(test.RandomBytes(20))
	balance := big.NewInt(123)
	txHash := test.RandomBytes(32)
	// add credit unit to state tree
	err := tr.Apply(CreateAccountAndAddCredit(address, templates.AlwaysFalseBytes(), balance, 3, txHash))
	require.NoError(t, err)
	// verify result
	unitID := address.Bytes()
	u, err := tr.GetUnit(unitID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.EqualValues(t, templates.AlwaysFalseBytes(), u.Bearer())
	stateDB := NewStateDB(tr, logger.New(t))
	require.Equal(t, balance, stateDB.GetBalance(address))
	abLink := stateDB.GetAlphaBillData(address)
	require.EqualValues(t, 3, abLink.Timeout)
	require.Equal(t, txHash, abLink.TxHash)
}

func TestUpdateEthAccountAddCredit(t *testing.T) {
	tr := state.NewEmptyState()
	address := common.BytesToAddress(test.RandomBytes(20))
	balance := big.NewInt(100)
	txHash := test.RandomBytes(32)
	// add credit unit to state tree
	err := tr.Apply(CreateAccountAndAddCredit(address, templates.AlwaysFalseBytes(), balance, 3, txHash))
	require.NoError(t, err)
	// update
	unitID := address.Bytes()
	txHashUpdate := test.RandomBytes(32)
	err = tr.Apply(UpdateEthAccountAddCredit(unitID, balance, 2, txHashUpdate))
	require.NoError(t, err)
	stateDB := NewStateDB(tr, logger.New(t))
	require.Equal(t, big.NewInt(200), stateDB.GetBalance(address))
	abLink := stateDB.GetAlphaBillData(address)
	require.EqualValues(t, 3, abLink.Timeout)
	require.Equal(t, txHashUpdate, abLink.TxHash)
}

func TestSetAccountBalance(t *testing.T) {
	tr := state.NewEmptyState()
	address := common.BytesToAddress(test.RandomBytes(20))
	balance := big.NewInt(100)
	txHash := test.RandomBytes(32)
	// add credit unit to state tree
	err := tr.Apply(CreateAccountAndAddCredit(address, templates.AlwaysFalseBytes(), balance, 3, txHash))
	require.NoError(t, err)
	// update
	unitID := address.Bytes()
	err = tr.Apply(SetBalance(unitID, big.NewInt(300)))
	require.NoError(t, err)
	stateDB := NewStateDB(tr, logger.New(t))
	require.Equal(t, big.NewInt(300), stateDB.GetBalance(address))
}
