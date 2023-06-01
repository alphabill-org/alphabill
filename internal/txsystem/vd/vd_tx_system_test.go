package vd

import (
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

func TestRegisterData_Ok(t *testing.T) {
	err := createVD(t).Execute(createTx(t, nil))
	require.NoError(t, err)
}

func TestRegisterData_InvalidOwnerProof(t *testing.T) {
	vd := createVD(t)
	genTx := createTx(t, script.PredicateAlwaysTrue())
	// tx := &txsystem.Transaction{
	// 	SystemId:       DefaultSystemIdentifier,
	// 	UnitId:         make([]byte, 32),
	// 	ClientMetadata: &txsystem.ClientMetadata{Timeout: 2},
	// }
	// genTx, err := txsystem.NewDefaultGenericTransaction(tx)
	// require.NoError(t, err)
	// tx.OwnerProof = script.PredicateAlwaysTrue()

	err := vd.Execute(genTx)
	require.ErrorIs(t, err, ErrOwnerProofPresent)
}

func TestRegisterData_Revert(t *testing.T) {
	vd := createVD(t)
	vdState, err := vd.State()
	require.NoError(t, err)
	err = vd.Execute(createTx(t, nil))
	require.NoError(t, err)
	_, err = vd.State()
	require.ErrorIs(t, err, txsystem.ErrStateContainsUncommittedChanges)
	vd.Revert()
	state, err := vd.State()
	require.NoError(t, err)
	require.Equal(t, vdState, state)
}

func TestRegisterData_WithDuplicate(t *testing.T) {
	vd := createVD(t)
	tx := createTx(t, nil)
	err := vd.Execute(tx)
	require.NoError(t, err)

	// send duplicate
	err = vd.Execute(tx)
	require.ErrorContains(t, err, "data already exists")
}

func createTx(t *testing.T, ownerProof []byte) txsystem.GenericTransaction {
	tx := &txsystem.Transaction{
		SystemId:       DefaultSystemIdentifier,
		UnitId:         hash.Sum256(test.RandomBytes(32)),
		TransactionAttributes: new(anypb.Any),
		ClientMetadata: defaultClientMetadata,
		OwnerProof:     ownerProof,
	}
	tx.FeeProof = script.PredicateArgumentEmpty()

	err := anypb.MarshalFrom(tx.TransactionAttributes, &RegisterDataAttributes{}, proto.MarshalOptions{})
	require.NoError(t, err)

	return &registerDataWrapper{
		wrapper: wrapper{
			transaction: tx,
		},
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
