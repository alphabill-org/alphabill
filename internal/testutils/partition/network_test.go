package testpartition

import (
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	testtxsystem "github.com/alphabill-org/alphabill/internal/testutils/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

var systemIdentifier = []byte{1, 2, 4, 1}

func TestNewNetwork_Ok(t *testing.T) {
	network, err := NewNetwork(3, func(_ map[string]crypto.Verifier) txsystem.TransactionSystem {
		return &testtxsystem.CounterTxSystem{}
	}, systemIdentifier)
	require.NoError(t, err)
	defer func() {
		err = network.Close()
		require.NoError(t, err)
	}()
	require.NotNil(t, network.RootChain)
	require.Equal(t, 3, len(network.Nodes))

	tx := testtransaction.NewTransaction(t, testtransaction.WithSystemID(systemIdentifier))
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	require.NoError(t, network.SubmitTx(tx))
	require.Eventually(t, BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)

	tx = testtransaction.NewTransaction(t, testtransaction.WithSystemID(systemIdentifier))
	fmt.Printf("Broadcasting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	err = network.BroadcastTx(tx)
	require.Eventually(t, BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)
}
