package vd

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/txsystem"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func TestRegisterData_Ok(t *testing.T) {
	_, err := createVD(t).Execute(createTx(t, script.PredicateArgumentEmpty()))
	require.NoError(t, err)
}

func TestRegisterData_InvalidOwnerProof(t *testing.T) {
	vd := createVD(t)
	tx := createTx(t, nil)
	tx.OwnerProof = script.PredicateArgumentEmpty()
	_, err := vd.Execute(tx)
	require.ErrorIs(t, err, ErrOwnerProofPresent)
}

func TestRegisterData_Revert(t *testing.T) {
	vd := createVD(t)
	vdState, err := vd.StateSummary()
	require.NoError(t, err)
	_, err = vd.Execute(createTx(t, script.PredicateArgumentEmpty()))
	require.NoError(t, err)
	_, err = vd.StateSummary()
	require.ErrorIs(t, err, txsystem.ErrStateContainsUncommittedChanges)
	vd.Revert()
	state, err := vd.StateSummary()
	require.NoError(t, err)
	require.Equal(t, vdState, state)
}

func TestRegisterData_WithDuplicate(t *testing.T) {
	vd := createVD(t)
	tx := createTx(t, script.PredicateArgumentEmpty())
	_, err := vd.Execute(tx)
	require.NoError(t, err)

	// send duplicate
	_, err = vd.Execute(tx)
	require.ErrorContains(t, err, "data already exists")
}

func createTx(t *testing.T, feeProof []byte) *types.TransactionOrder {
	attrBytes, _ := cbor.Marshal(&RegisterDataAttributes{})
	return &types.TransactionOrder{
		Payload: &types.Payload{
			Type:           PayloadTypeRegisterData,
			SystemID:       DefaultSystemIdentifier,
			UnitID:         hash.Sum256(test.RandomBytes(32)),
			ClientMetadata: defaultClientMetadata,
			Attributes:     attrBytes,
		},
		FeeProof: feeProof,
	}
}

func createVD(t *testing.T) *txsystem.GenericTxSystem {
	vd, err := NewTxSystem(
		WithSystemIdentifier(DefaultSystemIdentifier),
		WithTrustBase(map[string]abcrypto.Verifier{"test": nil}),
		WithState(newStateWithFeeCredit(t, feeCreditID)),
	)
	require.NoError(t, err)
	return vd
}
