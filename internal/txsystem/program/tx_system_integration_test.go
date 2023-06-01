package program

import (
	"context"
	"encoding/binary"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/rma"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
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
	}, DefaultProgramsSystemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{scPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { require.NoError(t, abNet.Close()) })

	deployTx := createProgDeploy(t, counterProgramUnitID, counterWasm, uint64ToLEBytes(2))
	require.NoError(t, scPrt.SubmitTx(deployTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(scPrt, deployTx), test.WaitDuration, test.WaitTick)

	callTx := createProgCall(t, counterProgramUnitID, "count", uint64ToLEBytes(1))
	require.NoError(t, scPrt.SubmitTx(callTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(scPrt, callTx), test.WaitDuration, test.WaitTick)
	// counter is initiated to 2 and count is called 3x times each incrementing the count+1
	// counter ID constant comes from counter program
	const cntStateID uint32 = 0xaabbccdd
	counterFileID := CreateStateFileID(counterProgramUnitID, util.Uint32ToBytes(cntStateID))
	u, err := state.GetUnit(counterFileID)
	require.NoError(t, err)
	stateFile, ok := u.Data.(*StateFile)
	require.True(t, ok)
	require.EqualValues(t, 3, binary.LittleEndian.Uint64(stateFile.bytes))

	callTx = createProgCall(t, counterProgramUnitID, "count", uint64ToLEBytes(2))
	// call count again with +2 so 3 + 2 = 5
	require.NoError(t, scPrt.SubmitTx(callTx))
	require.Eventually(t, testpartition.BlockchainContainsTx(scPrt, callTx), test.WaitDuration, test.WaitTick)
	u, err = state.GetUnit(counterFileID)
	require.NoError(t, err)
	stateFile, ok = u.Data.(*StateFile)
	require.True(t, ok)
	require.EqualValues(t, 5, binary.LittleEndian.Uint64(stateFile.bytes))
}

func createTx(unitID *uint256.Int, payloadType string) *types.TransactionOrder {
	unitId32 := unitID.Bytes32()
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   DefaultProgramsSystemIdentifier,
			UnitID:     unitId32[:],
			Type:       payloadType,
			Attributes: nil,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           20,
				MaxTransactionFee: 10,
			},
		},
	}
	return tx
}

func createProgDeploy(t *testing.T, unitID *uint256.Int, wasm []byte, params []byte) *types.TransactionOrder {
	tx := createTx(unitID, ProgramDeploy)
	bt := &PDeployAttributes{
		ProgModule: wasm,
		ProgParams: params,
	}
	rawBytes, err := cbor.Marshal(bt)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return tx
}

func createProgCall(t *testing.T, unitID *uint256.Int, fnName string, params []byte) *types.TransactionOrder {
	tx := createTx(unitID, ProgramCall)
	bt := &PCallAttributes{
		FuncName:  fnName,
		InputData: params,
	}
	rawBytes, err := cbor.Marshal(bt)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return tx
}
