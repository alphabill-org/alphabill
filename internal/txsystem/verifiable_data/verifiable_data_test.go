package verifiable_data

import (
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"
)

var vdSystemIdentifier = []byte{0, 0, 0, 1}

func TestRegisterData_Ok(t *testing.T) {
	sm, err := createVD(t).Execute(createTx())
	require.NoError(t, err)
	require.NotNil(t, sm)
}

func TestRegisterData_InvalidOwnerProof(t *testing.T) {
	vd := createVD(t)
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           txType,
			SystemID:       vdSystemIdentifier,
			UnitID:         make([]byte, 32),
			ClientMetadata: &types.ClientMetadata{Timeout: 2},
		},
	}
	tx.OwnerProof = script.PredicateAlwaysTrue()

	sm, err := vd.Execute(tx)
	require.ErrorIs(t, err, ErrOwnerProofPresent)
	require.Nil(t, sm)
}

func TestRegisterData_InvalidPayloadType(t *testing.T) {
	vd := createVD(t)
	sm, err := vd.Execute(&types.TransactionOrder{
		Payload: &types.Payload{
			Type:           "test",
			SystemID:       vdSystemIdentifier,
			UnitID:         make([]byte, 32),
			ClientMetadata: &types.ClientMetadata{Timeout: 2},
		},
	})
	require.ErrorContains(t, err, "invalid transaction payload type: got test, expected reg")
	require.Nil(t, sm)
}

func TestRegisterData_Revert(t *testing.T) {
	vd := createVD(t)
	vdState, err := vd.StateSummary()
	require.NoError(t, err)
	sm, err := vd.Execute(createTx())
	require.NoError(t, err)
	require.NotNil(t, sm)

	_, err = vd.StateSummary()
	require.ErrorIs(t, err, txsystem.ErrStateContainsUncommittedChanges)
	require.NotEqual(t, vdState, vd.getState())
	vd.Revert()
	state, err := vd.StateSummary()
	require.NoError(t, err)
	require.Equal(t, vdState, state)
}

func TestRegisterData_WithDuplicate(t *testing.T) {
	vd := createVD(t)
	tx := createTx()
	sm, err := vd.Execute(tx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// send duplicate
	sm, err = vd.Execute(tx)
	require.ErrorContains(t, err, "could not add item")
	require.Nil(t, sm)
}

func createTx() *types.TransactionOrder {
	hasher := crypto.SHA256.New()
	hasher.Write(test.RandomBytes(32))
	id := hasher.Sum(nil)
	return &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           txType,
			SystemID:       vdSystemIdentifier,
			UnitID:         id,
			ClientMetadata: &types.ClientMetadata{Timeout: 2},
		},
	}
}

func createVD(t *testing.T) *txSystem {
	vd, err := New(vdSystemIdentifier)
	require.NoError(t, err)
	return vd
}
