package vd

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var feeCreditID = uint256.NewInt(420)
var defaultClientMetadata = &txsystem.ClientMetadata{
	Timeout:           20,
	MaxFee:            10,
	FeeCreditRecordId: util.Uint256ToBytes(feeCreditID),
}

func TestVDPartition_Ok(t *testing.T) {
	vdPart, err := testpartition.NewPartition(3, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := NewTxSystem(
			WithSystemIdentifier(DefaultSystemIdentifier),
			WithTrustBase(map[string]crypto.Verifier{"test": nil}),
			WithState(newStateWithFeeCredit(t, feeCreditID)),
		)
		require.NoError(t, err)
		return system
	}, DefaultSystemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{vdPart})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { abNet.Close() })
	tx, err := createVDTransaction()
	require.NoError(t, err)
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	err = vdPart.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(vdPart, tx), test.WaitDuration, test.WaitTick)

	tx, err = createVDTransaction()
	require.NoError(t, err)
	err = vdPart.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(vdPart, tx), test.WaitDuration, test.WaitTick)
}

func TestVDPartition_OnePartitionNodeIsDown(t *testing.T) {
	t.SkipNow() // TODO AB-996
	vdPart, err := testpartition.NewPartition(6, func(trustBase map[string]crypto.Verifier) txsystem.TransactionSystem {
		system, err := NewTxSystem(
			WithSystemIdentifier(DefaultSystemIdentifier),
			WithTrustBase(map[string]crypto.Verifier{"test": nil}),
		)
		require.NoError(t, err)
		return system
	}, DefaultSystemIdentifier)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{vdPart})
	require.NoError(t, err)
	require.NoError(t, abNet.Start())
	t.Cleanup(func() { abNet.Close() })
	// killing the leader node can fail the test if all subsequent leader candidates happen to be the killed node within the timeout
	require.ErrorIs(t, vdPart.Nodes[5].Stop(), context.Canceled) // shut down the node

	tx, err := createVDTransaction()
	require.NoError(t, err)
	fmt.Printf("Submitting tx: %v, UnitId=%x\n", tx, tx.UnitId)
	err = vdPart.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContains(vdPart, func(actualTx *txsystem.Transaction) bool {
		return bytes.Equal(tx.UnitId, actualTx.UnitId)
	}), test.WaitDuration*2, test.WaitTick)
}

func createVDTransaction() (*txsystem.Transaction, error) {
	tx := &txsystem.Transaction{
		SystemId:       DefaultSystemIdentifier,
		UnitId:         hash.Sum256(test.RandomBytes(32)),
		TransactionAttributes: new(anypb.Any),
		ClientMetadata: defaultClientMetadata,
		OwnerProof:     nil,
	}
	tx.FeeProof = script.PredicateArgumentEmpty()

	err := anypb.MarshalFrom(tx.TransactionAttributes, &RegisterDataAttributes{}, proto.MarshalOptions{})
	if err != nil {
		return nil, err
	}
	return tx, nil
}

func newStateWithFeeCredit(t *testing.T, feeCreditID *uint256.Int) *rma.Tree {
	state := rma.NewWithSHA256()
	require.NoError(t, state.AtomicUpdate(
		fc.AddCredit(feeCreditID, script.PredicateAlwaysTrue(), &fc.FeeCreditRecord{
			Balance: 100,
			Hash:    make([]byte, 32),
			Timeout: 1000,
		}, make([]byte, 32)),
	))
	state.Commit()
	return state
}
