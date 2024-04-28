package orchestration

import (
	"testing"

	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"
	"github.com/alphabill-org/alphabill-go-sdk/txsystem/orchestration"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

func TestAddVar_AddNewUnit_OK(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	opts, err := defaultOptions()
	require.NoError(t, err)
	opts.state = state.NewEmptyState()
	opts.ownerPredicate = templates.NewP2pkh256BytesFromKey(pubKey)

	module, err := NewModule(opts)
	require.NoError(t, err)
	execFn := module.handleAddVarTx().ExecuteFunc()

	// execute addVar tx
	unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
	attr := orchestration.AddVarAttributes{}
	txo := createAddVarTx(t, signer, attr, testtransaction.WithUnitID(unitID))
	serverMetadata, err := execFn(txo, &txsystem.TxExecutionContext{CurrentBlockNr: 11})
	require.NoError(t, err)
	require.NotNil(t, serverMetadata)

	// verify state is updated
	u, err := opts.state.GetUnit(txo.UnitID(), false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &orchestration.VarData{}, u.Data())
	unitData := u.Data().(*orchestration.VarData)

	// verify epoch number is 0
	require.EqualValues(t, 0, unitData.EpochNumber)

	// and owner is the correct predicate
	require.Equal(t, opts.ownerPredicate, u.Bearer())

	// and tx processing result contains the validator assignment record from the tx
	processingDetails, err := types.Cbor.Marshal(attr.Var)
	require.NoError(t, err)
	require.EqualValues(t, serverMetadata.ProcessingDetails, processingDetails)

}

func TestAddVar_UpdateExistingUnit_OK(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	opts, err := defaultOptions()
	require.NoError(t, err)
	opts.state = state.NewEmptyState()
	opts.ownerPredicate = templates.NewP2pkh256BytesFromKey(pubKey)

	module, err := NewModule(opts)
	require.NoError(t, err)
	execFn := module.handleAddVarTx().ExecuteFunc()

	// add existing unit
	unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
	err = opts.state.Apply(state.AddUnit(unitID, opts.ownerPredicate, &orchestration.VarData{EpochNumber: 0}))
	require.NoError(t, err)

	// exec addVar tx
	attr := orchestration.AddVarAttributes{Var: orchestration.ValidatorAssignmentRecord{EpochNumber: 1}}
	txo := createAddVarTx(t, signer, attr, testtransaction.WithUnitID(unitID))
	sm, err := execFn(txo, &txsystem.TxExecutionContext{CurrentBlockNr: 11})
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify state is updated
	u, err := opts.state.GetUnit(txo.UnitID(), false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &orchestration.VarData{}, u.Data())
	unitData := u.Data().(*orchestration.VarData)

	// verify epoch number is incremented by one
	require.EqualValues(t, 1, unitData.EpochNumber)

	// and owner predicate remains the same
	require.Equal(t, opts.ownerPredicate, u.Bearer())
}

func TestAddVar_NOK(t *testing.T) {
	// create module
	opts, err := defaultOptions()
	require.NoError(t, err)
	opts.state = state.NewEmptyState()
	opts.ownerPredicate = templates.NewP2pkh256BytesFromKey(test.RandomBytes(32))

	module, err := NewModule(opts)
	require.NoError(t, err)
	execFn := module.handleAddVarTx().ExecuteFunc()

	// execute addVar tx with empty owner proof to simulate error
	txo := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(orchestration.NewVarID(nil, test.RandomBytes(32))),
		testtransaction.WithSystemID(orchestration.DefaultSystemID),
		testtransaction.WithPayloadType(orchestration.PayloadTypeAddVAR),
		testtransaction.WithAttributes(orchestration.AddVarAttributes{}),
	)
	serverMetadata, err := execFn(txo, &txsystem.TxExecutionContext{CurrentBlockNr: 11})
	require.ErrorContains(t, err, "invalid 'addVar' tx")
	require.Nil(t, serverMetadata)
}

func TestAddVar_Validation(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)

	opts, err := defaultOptions()
	require.NoError(t, err)
	opts.state = state.NewEmptyState()
	opts.ownerPredicate = templates.NewP2pkh256BytesFromKey(pubKey)

	module, err := NewModule(opts)
	require.NoError(t, err)
	validateFn := module.validateAddVarTx

	unitID := orchestration.NewVarID(nil, test.RandomBytes(32))

	tests := []struct {
		name    string
		tx      *types.TransactionOrder
		attr    *orchestration.AddVarAttributes
		unit    *state.Unit
		wantErr string
	}{
		{
			name:    "Ok",
			tx:      createAddVarTx(t, signer, orchestration.AddVarAttributes{}, testtransaction.WithUnitID(unitID)),
			attr:    &orchestration.AddVarAttributes{},
			wantErr: "",
		},
		{
			name:    "InvalidUnitIdType",
			tx:      createAddVarTx(t, signer, orchestration.AddVarAttributes{}, testtransaction.WithUnitID([]byte{})),
			attr:    &orchestration.AddVarAttributes{},
			unit:    &state.Unit{},
			wantErr: "invalid unit identifier: type is not VAR type",
		},
		{
			name:    "InvalidUnitDataType",
			tx:      createAddVarTx(t, signer, orchestration.AddVarAttributes{}, testtransaction.WithUnitID(unitID)),
			attr:    &orchestration.AddVarAttributes{},
			unit:    &state.Unit{},
			wantErr: "invalid unit data type",
		},
		{
			name:    "InvalidEpochNumber_ExistingUnit",
			tx:      createAddVarTx(t, signer, orchestration.AddVarAttributes{}, testtransaction.WithUnitID(unitID)),
			attr:    &orchestration.AddVarAttributes{Var: orchestration.ValidatorAssignmentRecord{EpochNumber: 5}},
			unit:    state.NewUnit(nil, &orchestration.VarData{EpochNumber: 5}),
			wantErr: "invalid epoch number, must increment by 1, got 5 expected 6",
		},
		{
			name:    "InvalidEpochNumber_NewUnit",
			tx:      createAddVarTx(t, signer, orchestration.AddVarAttributes{}, testtransaction.WithUnitID(unitID)),
			attr:    &orchestration.AddVarAttributes{Var: orchestration.ValidatorAssignmentRecord{EpochNumber: 1}},
			wantErr: "invalid epoch number, must be 0 for new units, got 1",
		},
		{
			name: "InvalidOwnerPredicate",
			tx: testtransaction.NewTransactionOrder(t,
				testtransaction.WithUnitID(unitID),
				testtransaction.WithSystemID(orchestration.DefaultSystemID),
				testtransaction.WithPayloadType(orchestration.PayloadTypeAddVAR),
				testtransaction.WithAttributes(orchestration.AddVarAttributes{}),
				testtransaction.WithOwnerProof([]byte{1}),
			),
			attr:    &orchestration.AddVarAttributes{},
			wantErr: "invalid owner proof",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFn(tt.tx, tt.attr, tt.unit)
			if tt.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func createAddVarTx(t *testing.T, signer crypto.Signer, attr orchestration.AddVarAttributes, options ...testtransaction.Option) *types.TransactionOrder {
	t.Helper()
	txo := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(orchestration.DefaultSystemID),
		testtransaction.WithPayloadType(orchestration.PayloadTypeAddVAR),
		testtransaction.WithAttributes(attr),
	)
	for _, o := range options {
		require.NoError(t, o(txo))
	}
	err := txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer))
	require.NoError(t, err)
	return txo
}
