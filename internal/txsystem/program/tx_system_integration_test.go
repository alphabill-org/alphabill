package program

import (
	"context"
	"encoding/binary"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

func uint64ToLEBytes(i uint64) []byte {
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, i)
	return bytes
}

func TestSmartContractPartition_Ok(t *testing.T) {
	ctx := context.Background()
	scPrt, err := testpartition.NewPartition(3, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		system, err := New(ctx, []byte{0, 0, 0, 3})
		require.NoError(t, err)
		return system
	}, DefaultSmartContractSystemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{scPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { require.NoError(t, abNet.Close()) })

	txDeployOrder := testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(counterProgramID),
		testtransaction.WithSystemID(DefaultSmartContractSystemIdentifier),
		testtransaction.WithAttributes(&PDeployAttributes{
			Program:  counterWasm,
			InitData: uint64ToLEBytes(2),
		}),
		testtransaction.WithOwnerProof(nil),
	)
	require.NoError(t, scPrt.SubmitTx(txDeployOrder))
	require.Eventually(t, testpartition.BlockchainContainsTx(scPrt, txDeployOrder), test.WaitDuration, test.WaitTick)
	txCallOrder := testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(counterProgramID),
		testtransaction.WithSystemID(DefaultSmartContractSystemIdentifier),
		testtransaction.WithAttributes(&PCallAttributes{
			Function: "count",
			Input:    uint64ToLEBytes(2),
		}),
		testtransaction.WithOwnerProof(nil),
	)
	require.NoError(t, scPrt.SubmitTx(txCallOrder))
	require.Eventually(t, testpartition.BlockchainContainsTx(scPrt, txCallOrder), test.WaitDuration, test.WaitTick)
}
