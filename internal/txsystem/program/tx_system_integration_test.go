package program

import (
	"context"
	"encoding/binary"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/stretchr/testify/require"
)

func uint64ToLEBytes(i uint64) []byte {
	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, i)
	return bytes
}

func TestSmartContractPartition_Ok(t *testing.T) {
	ctx := context.Background()
	var state *rma.Tree
	scPrt, err := testpartition.NewPartition(3, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		state = rma.NewWithSHA256()
		system, err := New(ctx, []byte{0, 0, 0, 3}, WithState(state))
		require.NoError(t, err)
		return system
	}, DefaultSmartContractSystemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{scPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { require.NoError(t, abNet.Close()) })

	txDeployOrder := testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(counterProgramUnitID.Bytes()),
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
		testtransaction.WithUnitId(counterProgramUnitID.Bytes()),
		testtransaction.WithSystemID(DefaultSmartContractSystemIdentifier),
		testtransaction.WithAttributes(&PCallAttributes{
			Function: "count",
			Input:    uint64ToLEBytes(1),
		}),
		testtransaction.WithOwnerProof(nil),
	)
	require.NoError(t, scPrt.SubmitTx(txCallOrder))
	require.Eventually(t, testpartition.BlockchainContainsTx(scPrt, txCallOrder), test.WaitDuration, test.WaitTick)
	// counter is initiated to 2 and count is called 3x times each incrementing the count+1
	// counter ID constant comes from counter program
	const cntStateID uint32 = 0xaabbccdd
	counterFileID := CreateStateFileID(counterProgramUnitID, util.Uint32ToBytes(cntStateID))
	u, err := state.GetUnit(counterFileID)
	require.NoError(t, err)
	stateFile, ok := u.Data.(*StateFile)
	require.True(t, ok)
	require.EqualValues(t, 3, binary.LittleEndian.Uint64(stateFile.bytes))
	txCallOrder = testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(counterProgramUnitID.Bytes()),
		testtransaction.WithSystemID(DefaultSmartContractSystemIdentifier),
		testtransaction.WithAttributes(&PCallAttributes{
			Function: "count",
			Input:    uint64ToLEBytes(2),
		}),
		testtransaction.WithOwnerProof(nil),
	)
	// call count again with +2 so 3 + 2 = 5
	require.NoError(t, scPrt.SubmitTx(txCallOrder))
	require.Eventually(t, testpartition.BlockchainContainsTx(scPrt, txCallOrder), test.WaitDuration, test.WaitTick)
	u, err = state.GetUnit(counterFileID)
	require.NoError(t, err)
	stateFile, ok = u.Data.(*StateFile)
	require.True(t, ok)
	require.EqualValues(t, 5, binary.LittleEndian.Uint64(stateFile.bytes))
	// fails invalid input
	txCallOrder = testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(counterProgramUnitID.Bytes()),
		testtransaction.WithSystemID(DefaultSmartContractSystemIdentifier),
		testtransaction.WithAttributes(&PCallAttributes{
			Function: "count",
			Input:    []byte{3},
		}),
		testtransaction.WithOwnerProof(nil),
	)
	require.Never(t, testpartition.BlockchainContainsTx(scPrt, txCallOrder), test.WaitDuration, test.WaitTick)
	// fails - no such function
	txCallOrder = testtransaction.NewTransaction(t,
		testtransaction.WithUnitId(counterProgramUnitID.Bytes()),
		testtransaction.WithSystemID(DefaultSmartContractSystemIdentifier),
		testtransaction.WithAttributes(&PCallAttributes{
			Function: "counter",
			Input:    uint64ToLEBytes(2),
		}),
		testtransaction.WithOwnerProof(nil),
	)
	require.Never(t, testpartition.BlockchainContainsTx(scPrt, txCallOrder), test.WaitDuration, test.WaitTick)

}
