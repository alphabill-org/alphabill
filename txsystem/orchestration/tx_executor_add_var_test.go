package orchestration

import (
	"testing"

	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
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
	unitID := NewVarID(nil, test.RandomBytes(32))
	attr := AddVarAttributes{}
	txo := createAddVarTx(t, signer, attr, testtransaction.WithUnitID(unitID))
	serverMetadata, err := execFn(txo, &txsystem.TxExecutionContext{CurrentBlockNr: 11})
	require.NoError(t, err)
	require.NotNil(t, serverMetadata)

	// verify state is updated
	u, err := opts.state.GetUnit(txo.UnitID(), false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &VarData{}, u.Data())
	unitData := u.Data().(*VarData)

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
	unitID := NewVarID(nil, test.RandomBytes(32))
	err = opts.state.Apply(state.AddUnit(unitID, opts.ownerPredicate, &VarData{EpochNumber: 0}))
	require.NoError(t, err)

	// exec addVar tx
	attr := AddVarAttributes{Var: ValidatorAssignmentRecord{EpochNumber: 1}}
	txo := createAddVarTx(t, signer, attr, testtransaction.WithUnitID(unitID))
	sm, err := execFn(txo, &txsystem.TxExecutionContext{CurrentBlockNr: 11})
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify state is updated
	u, err := opts.state.GetUnit(txo.UnitID(), false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.IsType(t, &VarData{}, u.Data())
	unitData := u.Data().(*VarData)

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
		testtransaction.WithUnitID(NewVarID(nil, test.RandomBytes(32))),
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeAddVAR),
		testtransaction.WithAttributes(AddVarAttributes{}),
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

	unitID := NewVarID(nil, test.RandomBytes(32))

	tests := []struct {
		name    string
		tx      *types.TransactionOrder
		attr    *AddVarAttributes
		unit    *state.Unit
		wantErr string
	}{
		{
			name:    "Ok",
			tx:      createAddVarTx(t, signer, AddVarAttributes{}, testtransaction.WithUnitID(unitID)),
			attr:    &AddVarAttributes{},
			wantErr: "",
		},
		{
			name:    "InvalidUnitIdType",
			tx:      createAddVarTx(t, signer, AddVarAttributes{}, testtransaction.WithUnitID([]byte{})),
			attr:    &AddVarAttributes{},
			unit:    &state.Unit{},
			wantErr: "invalid unit identifier: type is not VAR type",
		},
		{
			name:    "InvalidUnitDataType",
			tx:      createAddVarTx(t, signer, AddVarAttributes{}, testtransaction.WithUnitID(unitID)),
			attr:    &AddVarAttributes{},
			unit:    &state.Unit{},
			wantErr: "invalid unit data type",
		},
		{
			name:    "InvalidEpochNumber_ExistingUnit",
			tx:      createAddVarTx(t, signer, AddVarAttributes{}, testtransaction.WithUnitID(unitID)),
			attr:    &AddVarAttributes{Var: ValidatorAssignmentRecord{EpochNumber: 5}},
			unit:    state.NewUnit(nil, &VarData{EpochNumber: 5}),
			wantErr: "invalid epoch number, must increment by 1, got 5 expected 6",
		},
		{
			name:    "InvalidEpochNumber_NewUnit",
			tx:      createAddVarTx(t, signer, AddVarAttributes{}, testtransaction.WithUnitID(unitID)),
			attr:    &AddVarAttributes{Var: ValidatorAssignmentRecord{EpochNumber: 1}},
			wantErr: "invalid epoch number, must be 0 for new units, got 1",
		},
		{
			name: "InvalidOwnerPredicate",
			tx: testtransaction.NewTransactionOrder(t,
				testtransaction.WithUnitID(unitID),
				testtransaction.WithSystemID(DefaultSystemIdentifier),
				testtransaction.WithPayloadType(PayloadTypeAddVAR),
				testtransaction.WithAttributes(AddVarAttributes{}),
				testtransaction.WithOwnerProof([]byte{1}),
			),
			attr:    &AddVarAttributes{},
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

func createAddVarTx(t *testing.T, signer crypto.Signer, attr AddVarAttributes, options ...testtransaction.Option) *types.TransactionOrder {
	t.Helper()
	txo := testtransaction.NewTransactionOrder(t,
		testtransaction.WithSystemID(DefaultSystemIdentifier),
		testtransaction.WithPayloadType(PayloadTypeAddVAR),
		testtransaction.WithAttributes(attr),
	)
	for _, o := range options {
		require.NoError(t, o(txo))
	}
	err := txo.SetOwnerProof(predicates.OwnerProoferForSigner(signer))
	require.NoError(t, err)
	return txo
}
