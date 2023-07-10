package vd

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/state"
	fcunit "github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var feeCreditID = uint256.NewInt(420)
var defaultClientMetadata = &types.ClientMetadata{
	Timeout:           20,
	MaxTransactionFee: 10,
	FeeCreditRecordID: util.Uint256ToBytes(feeCreditID),
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
	tx := createVDTransaction()
	err = vdPart.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(vdPart, tx), test.WaitDuration, test.WaitTick)

	tx = createVDTransaction()
	err = vdPart.SubmitTx(tx)
	require.NoError(t, err)
	require.Eventually(t, testpartition.BlockchainContainsTx(vdPart, tx), test.WaitDuration, test.WaitTick)
}

func createVDTransaction() *types.TransactionOrder {
	attrBytes, _ := cbor.Marshal(&RegisterDataAttributes{})
	return &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           PayloadTypeRegisterData,
			SystemID:       DefaultSystemIdentifier,
			UnitID:         hash.Sum256(test.RandomBytes(32)),
			ClientMetadata: defaultClientMetadata,
			Attributes:     attrBytes,
		},
		FeeProof: script.PredicateArgumentEmpty(),
	}
}

func newStateWithFeeCredit(t *testing.T, feeCreditID *uint256.Int) *state.State {
	s := state.NewEmptyState()
	require.NoError(t, s.Apply(
		fcunit.AddCredit(util.Uint256ToBytes(feeCreditID), script.PredicateAlwaysTrue(), &fcunit.FeeCreditRecord{
			Balance: 100,
			Hash:    make([]byte, 32),
			Timeout: 1000,
		}),
	))
	_, _, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit())
	return s
}
