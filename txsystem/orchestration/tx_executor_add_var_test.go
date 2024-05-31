package orchestration

import (
	"hash"
	"testing"

	"github.com/alphabill-org/alphabill-go-base/txsystem/orchestration"
	testctx "github.com/alphabill-org/alphabill/txsystem/testutils/exec_context"
	txtypes "github.com/alphabill-org/alphabill/txsystem/types"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/state"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

type TestData struct {
	_ struct{} `cbor:",toarray"`
}

func (t *TestData) Write(hasher hash.Hash) error { return nil }
func (t *TestData) SummaryValueInput() uint64 {
	return 0
}
func (t *TestData) Copy() types.UnitData { return &TestData{} }

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

	// execute addVar tx
	unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
	attr := &orchestration.AddVarAttributes{}
	txo := createAddVarTx(t, signer, attr, testtransaction.WithUnitID(unitID))
	exeCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(11))

	require.NoError(t, module.validateAddVarTx(txo, attr, exeCtx))
	serverMetadata, err := module.executeAddVarTx(txo, attr, exeCtx)
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

	// add existing unit
	unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
	err = opts.state.Apply(state.AddUnit(unitID, opts.ownerPredicate, &orchestration.VarData{EpochNumber: 0}))
	require.NoError(t, err)

	// exec addVar tx
	attr := &orchestration.AddVarAttributes{Var: orchestration.ValidatorAssignmentRecord{EpochNumber: 1}}
	txo := createAddVarTx(t, signer, attr, testtransaction.WithUnitID(unitID))
	execCtx := testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(11))
	require.NoError(t, module.validateAddVarTx(txo, attr, execCtx))
	sm, err := module.executeAddVarTx(txo, attr, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(11)))
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
	txExecutors := make(txtypes.TxExecutors)
	require.NoError(t, txExecutors.Add(module.TxHandlers()))

	// execute addVar tx with empty owner proof to simulate error
	txo := testtransaction.NewTransactionOrder(t,
		testtransaction.WithUnitID(orchestration.NewVarID(nil, test.RandomBytes(32))),
		testtransaction.WithSystemID(orchestration.DefaultSystemID),
		testtransaction.WithPayloadType(orchestration.PayloadTypeAddVAR),
		testtransaction.WithAttributes(orchestration.AddVarAttributes{}),
	)
	serverMetadata, err := txExecutors.ValidateAndExecute(txo, testctx.NewMockExecutionContext(t, testctx.WithCurrentRound(11)))
	require.ErrorContains(t, err, "'addVar' validation failed: invalid owner proof: executing predicate: failed to decode P2PKH256 signature: EOF")
	require.Nil(t, serverMetadata)
}

func TestAddVar_Validation(t *testing.T) {
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	pubKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	ownerPredicate := templates.NewP2pkh256BytesFromKey(pubKey)

	t.Run("Ok", func(t *testing.T) {
		unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
		attr := &orchestration.AddVarAttributes{}
		tx := createAddVarTx(t, signer, attr, testtransaction.WithUnitID(unitID))
		module := newTestVarModule(t, ownerPredicate)
		exeCtx := testctx.NewMockExecutionContext(t)
		require.NoError(t, module.validateAddVarTx(tx, attr, exeCtx))
	})
	t.Run("InvalidUnitIdType", func(t *testing.T) {
		unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
		attr := &orchestration.AddVarAttributes{Var: orchestration.ValidatorAssignmentRecord{EpochNumber: 1}}
		tx := createAddVarTx(t, signer, attr, testtransaction.WithUnitID([]byte{}))
		module := newTestVarModule(t, ownerPredicate, withStateUnit(unitID, templates.AlwaysTrueBytes(), &TestData{}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateAddVarTx(tx, attr, exeCtx), "invalid unit identifier: type is not VAR type")
	})
	t.Run("InvalidUnitDataType", func(t *testing.T) {
		unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
		attr := &orchestration.AddVarAttributes{Var: orchestration.ValidatorAssignmentRecord{EpochNumber: 1}}
		tx := createAddVarTx(t, signer, attr, testtransaction.WithUnitID(unitID))
		module := newTestVarModule(t, ownerPredicate, withStateUnit(unitID, templates.AlwaysTrueBytes(), &TestData{}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateAddVarTx(tx, attr, exeCtx), "invalid unit data type")
	})
	t.Run("InvalidEpochNumber_ExistingUnit", func(t *testing.T) {
		unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
		attr := &orchestration.AddVarAttributes{Var: orchestration.ValidatorAssignmentRecord{EpochNumber: 5}}
		tx := createAddVarTx(t, signer, attr, testtransaction.WithUnitID(unitID))
		module := newTestVarModule(t, ownerPredicate, withStateUnit(unitID, templates.AlwaysTrueBytes(), &orchestration.VarData{EpochNumber: 5}))
		exeCtx := testctx.NewMockExecutionContext(t)
		require.EqualError(t, module.validateAddVarTx(tx, attr, exeCtx), "invalid epoch number, must increment by 1, got 5 expected 6")
	})
	t.Run("InvalidEpochNumber_NewUnit", func(t *testing.T) {
		unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
		attr := &orchestration.AddVarAttributes{Var: orchestration.ValidatorAssignmentRecord{EpochNumber: 1}}
		tx := createAddVarTx(t, signer, attr, testtransaction.WithUnitID(unitID))
		exeCtx := testctx.NewMockExecutionContext(t)
		module := newTestVarModule(t, ownerPredicate)
		require.EqualError(t, module.validateAddVarTx(tx, attr, exeCtx), "invalid epoch number, must be 0 for new units, got 1")
	})
	t.Run("InvalidOwnerPredicate", func(t *testing.T) {
		unitID := orchestration.NewVarID(nil, test.RandomBytes(32))
		attr := &orchestration.AddVarAttributes{Var: orchestration.ValidatorAssignmentRecord{EpochNumber: 2}}
		tx := testtransaction.NewTransactionOrder(t,
			testtransaction.WithUnitID(unitID),
			testtransaction.WithSystemID(orchestration.DefaultSystemID),
			testtransaction.WithPayloadType(orchestration.PayloadTypeAddVAR),
			testtransaction.WithAttributes(attr),
			testtransaction.WithOwnerProof([]byte{1}),
		)
		exeCtx := testctx.NewMockExecutionContext(t)
		module := newTestVarModule(t, ownerPredicate, withStateUnit(unitID, templates.AlwaysTrueBytes(), &orchestration.VarData{EpochNumber: 1}))
		require.EqualError(t, module.validateAddVarTx(tx, attr, exeCtx),
			"invalid owner proof: executing predicate: failed to decode P2PKH256 signature: cbor: cannot unmarshal positive integer into Go value of type templates.P2pkh256Signature")
	})
}

func createAddVarTx(t *testing.T, signer crypto.Signer, attr *orchestration.AddVarAttributes, options ...testtransaction.Option) *types.TransactionOrder {
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

type varModuleOption func(m *Module) error

func withStateUnit(unitID []byte, bearer types.PredicateBytes, data types.UnitData) varModuleOption {
	return func(m *Module) error {
		return m.state.Apply(state.AddUnit(unitID, bearer, data))
	}
}

func newTestVarModule(t *testing.T, ownerPredicate []byte, opts ...varModuleOption) *Module {
	options, err := defaultOptions()
	require.NoError(t, err)
	options.ownerPredicate = ownerPredicate
	options.state = state.NewEmptyState()
	module, err := NewModule(options)
	require.NoError(t, err)
	for _, opt := range opts {
		require.NoError(t, opt(module))
	}
	return module
}
