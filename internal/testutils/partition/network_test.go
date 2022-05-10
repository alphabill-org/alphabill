package testpartition

import (
	"testing"

	"google.golang.org/protobuf/proto"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/transaction"

	testtransaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/transaction"

	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
)

var systemIdentifier = []byte{1, 2, 4, 1}

func TestNewNetwork_Ok(t *testing.T) {
	network, err := NewNetwork(2, func() txsystem.TransactionSystem {
		return &CounterTxSystem{}
	}, systemIdentifier)
	require.NoError(t, err)
	defer func() {
		err = network.Close()
		require.NoError(t, err)
	}()
	require.NotNil(t, network.RootChain)
	require.Equal(t, 2, len(network.Nodes))

	tx := randomTx(systemIdentifier)
	require.NoError(t, network.SubmitTx(tx))
	require.Eventually(t, BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)

	tx = randomTx(systemIdentifier)
	err = network.BroadcastTx(tx)
	require.Eventually(t, BlockchainContainsTx(tx, network), test.WaitDuration, test.WaitTick)
}

func BlockchainContainsTx(tx *transaction.Transaction, network *AlphabillNetwork) func() bool {
	return func() bool {
		for _, store := range network.BlockStores {
			height, _ := store.Height()
			for i := uint64(0); i <= height; i++ {
				b, err := store.Get(height - i)
				if err != nil || b == nil {
					continue
				}
				for _, t := range b.Transactions {
					if proto.Equal(t, tx) {
						return true
					}
				}
			}

		}
		return false
	}
}

func randomTx(systemIdentifier []byte) *transaction.Transaction {
	tx := testtransaction.RandomBillTransfer()
	tx.SystemId = systemIdentifier
	return tx
}
