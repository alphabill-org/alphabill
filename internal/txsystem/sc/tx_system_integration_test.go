package sc

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

func TestSmartContractPartition_Ok(t *testing.T) {
	scPrt, err := testpartition.NewPartition(t, 3, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := New([]byte{0, 0, 0, 3})
		require.NoError(t, err)
		return system
	}, DefaultSmartContractSystemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{scPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start(t))
	t.Cleanup(func() { require.NoError(t, abNet.Close()) })

	txOrder := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitId(unlockMoneyBillProgramID),
		testtransaction.WithSystemID(DefaultSmartContractSystemIdentifier),
		testtransaction.WithAttributes(&SCallAttributes{
			Input: []byte{0, 0, 0, 3},
		}),
		testtransaction.WithOwnerProof(nil),
	)
	require.NoError(t, scPrt.SubmitTx(txOrder))
	// TODO scall transaction order isn't implemented (AB-679)
	require.Never(t, testpartition.BlockchainContainsTx(scPrt, txOrder), test.WaitDuration, test.WaitTick)
}
