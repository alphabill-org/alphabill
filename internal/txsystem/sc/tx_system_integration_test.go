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
	network, err := testpartition.NewNetwork(3, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := New([]byte{0, 0, 0, 3})
		require.NoError(t, err)
		return system
	}, DefaultSmartContractSystemIdentifier)
	require.NoError(t, err)
	require.NotNil(t, network)

	txOrder := testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(unlockMoneyBillProgramID),
		testtransaction.WithSystemID(DefaultSmartContractSystemIdentifier),
		testtransaction.WithAttributes(&SCallAttributes{
			Input: []byte{0, 0, 0, 3},
		}),
		testtransaction.WithOwnerProof(nil),
	)
	require.NoError(t, network.SubmitTx(txOrder))
	// TODO scall transaction order isn't implemented (AB-679)
	require.Never(t, testpartition.BlockchainContainsTx(txOrder, network), test.WaitDuration, test.WaitTick)
}
