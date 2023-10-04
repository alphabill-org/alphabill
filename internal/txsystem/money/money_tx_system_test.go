package money

import (
	"bytes"
	"crypto"
	"sort"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/state"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc/unit"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

const initialDustCollectorMoneyAmount uint64 = 100

var (
	initialBill = &InitialBill{
		ID:    NewBillID(nil, test.RandomBytes(UnitPartLength)),
		Value: 110,
		Owner: script.PredicateAlwaysTrue(),
	}
	fcrID         = NewFeeCreditRecordID(nil, []byte{88})
	fcrAmount     = uint64(1e8)
	moneySystemID = DefaultSystemIdentifier
)

func TestNewMoneyTxSystem(t *testing.T) {
	var (
		txsState      = state.NewEmptyState()
		dcMoneyAmount = uint64(222)
		sdrs          = createSDRs(newBillID(3))
		_, verifier   = testsig.CreateSignerAndVerifier(t)
		trustBase     = map[string]abcrypto.Verifier{"test": verifier}
	)
	txSystem, err := NewTxSystem(
		WithSystemIdentifier(moneySystemID),
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(initialBill),
		WithSystemDescriptionRecords(sdrs),
		WithDCMoneyAmount(dcMoneyAmount),
		WithState(txsState),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)
	require.NotNil(t, txSystem)

	u, err := txsState.GetUnit(initialBill.ID, false)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, initialBill.Value, u.Data().SummaryValueInput())
	require.Equal(t, initialBill.Owner, u.Bearer())

	d, err := txsState.GetUnit(dustCollectorMoneySupplyID, false)
	require.NoError(t, err)
	require.NotNil(t, d)

	require.Equal(t, dcMoneyAmount, d.Data().SummaryValueInput())
	require.Equal(t, state.Predicate(dustCollectorPredicate), d.Bearer())
}

func TestNewMoneyTxSystem_InitialBillIsNil(t *testing.T) {
	_, err := NewTxSystem(
		WithSystemIdentifier(moneySystemID),
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(nil),
		WithSystemDescriptionRecords(createSDRs(newBillID(2))),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrInitialBillIsNil)
}

func TestNewMoneyTxSystem_InvalidInitialBillID(t *testing.T) {
	ib := &InitialBill{ID: dustCollectorMoneySupplyID, Value: 100, Owner: nil}
	_, err := NewTxSystem(
		WithSystemIdentifier(moneySystemID),
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(ib),
		WithSystemDescriptionRecords(createSDRs(newBillID(2))),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrInvalidInitialBillID)
}

func TestNewMoneyTxSystem_InvalidFeeCreditBill_Nil(t *testing.T) {
	_, err := NewTxSystem(
		WithSystemIdentifier(moneySystemID),
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(&InitialBill{ID: []byte{1}, Value: 100, Owner: nil}),
		WithSystemDescriptionRecords(nil),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrUndefinedSystemDescriptionRecords)
}

func TestNewMoneyTxSystem_InvalidFeeCreditBill_SameIDAsInitialBill(t *testing.T) {
	billID := NewBillID(nil, []byte{1})
	_, err := NewTxSystem(
		WithSystemIdentifier(moneySystemID),
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(&InitialBill{ID: billID, Value: 100, Owner: nil}),
		WithSystemDescriptionRecords(createSDRs(billID)),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrInvalidFeeCreditBillID)
}

func TestNewMoneyScheme_InvalidFeeCreditBill_SameIDAsDCBill(t *testing.T) {
	_, err := NewTxSystem(
		WithSystemIdentifier(moneySystemID),
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(&InitialBill{ID: NewBillID(nil, []byte{1}), Value: 100, Owner: nil}),
		WithSystemDescriptionRecords(createSDRs(NewBillID(nil, nil))),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrInvalidFeeCreditBillID)
}

func TestExecute_TransferOk(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	_, data := getBill(t, rmaTree, initialBill.ID)

	transferOk, _ := createBillTransfer(t, initialBill.ID, initialBill.Value, script.PredicateAlwaysFalse(), nil)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)
	serverMetadata, err := txSystem.Execute(transferOk)
	require.NoError(t, err)

	_, err = txSystem.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, serverMetadata)
	require.NoError(t, txSystem.Commit())

	unit2, data2 := getBill(t, rmaTree, initialBill.ID)
	require.Equal(t, data.SummaryValueInput(), data2.SummaryValueInput())
	require.NotEqual(t, transferOk.OwnerProof, unit2.Bearer())
	require.Equal(t, roundNumber, data2.T)
	require.EqualValues(t, transferOk.Hash(crypto.SHA256), data2.Backlink)
}

func TestExecute_SplitOk(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	totalValue, _, err := rmaTree.CalculateRoot()
	require.NoError(t, err)
	initBill, initBillData := getBill(t, rmaTree, initialBill.ID)
	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	splitOk, splitAttr := createSplit(t, initialBill.ID, amount, remaining, script.PredicateAlwaysTrue(), initBillData.Backlink)
	roundNumber := uint64(1)
	txSystem.BeginBlock(roundNumber)
	sm, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, sm)

	txSystem.Commit()
	initBillAfterUpdate, initBillDataAfterUpdate := getBill(t, rmaTree, initialBill.ID)

	// bill value was reduced
	require.NotEqual(t, initBillData.V, initBillDataAfterUpdate.V)
	require.Equal(t, remaining, initBillDataAfterUpdate.V)
	// total value was not changed

	total, _, err := rmaTree.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, totalValue, total)
	// bearer of the initial bill was not changed
	require.Equal(t, initBill.Bearer(), initBillAfterUpdate.Bearer())
	require.Equal(t, roundNumber, initBillDataAfterUpdate.T)

	expectedNewUnitId := NewBillID(splitOk.UnitID(), unitIdFromTransaction(splitOk))
	newBill, bd := getBill(t, rmaTree, expectedNewUnitId)
	require.NotNil(t, newBill)
	require.NotNil(t, bd)
	require.Equal(t, amount, bd.V)
	require.EqualValues(t, splitOk.Hash(crypto.SHA256), bd.Backlink)
	require.Equal(t, state.Predicate(splitAttr.TargetBearer), newBill.Bearer())
	require.Equal(t, roundNumber, bd.T)
}

func TestExecuteTransferDC_OK(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	splitOk, _ := createSplit(t, initialBill.ID, amount, remaining, script.PredicateAlwaysTrue(), initBillData.Backlink)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)
	sm, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, sm)
	billID := NewBillID(splitOk.UnitID(), unitIdFromTransaction(splitOk))
	_, splitBillData := getBill(t, rmaTree, billID)

	transferDCOk, _ := createDCTransfer(t, billID, splitBillData.V, splitBillData.Backlink, test.RandomBytes(32), test.RandomBytes(32))
	require.NoError(t, err)

	sm, err = txSystem.Execute(transferDCOk)
	require.NoError(t, err)
	require.NotNil(t, sm)

	transferDCBill, transferDCBillData := getBill(t, rmaTree, billID)
	require.NotEqual(t, dustCollectorPredicate, transferDCBill.Bearer())
	require.EqualValues(t, 0, transferDCBillData.SummaryValueInput()) // dust transfer sets bill value to 0
	require.Equal(t, roundNumber, transferDCBillData.T)
	require.EqualValues(t, transferDCOk.Hash(crypto.SHA256), transferDCBillData.Backlink)
}

func TestExecute_SwapOk(t *testing.T) {
	rmaTree, txSystem, signer := createStateAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	var remaining uint64 = 99
	amount := initialBill.Value - remaining
	splitOk, _ := createSplit(t, initialBill.ID, amount, remaining, script.PredicateAlwaysTrue(), initBillData.Backlink)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)
	sm, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, sm)

	splitBillID := NewBillID(splitOk.UnitID(), unitIdFromTransaction(splitOk))
	targetID := initialBill.ID
	targetBacklink := splitOk.Hash(crypto.SHA256)
	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, []types.UnitID{splitBillID}, targetID, targetBacklink, rmaTree, signer)
	for _, dcTransfer := range dcTransfers {
		sm, err = txSystem.Execute(dcTransfer.TransactionOrder)
		require.NoError(t, err)
		require.NotNil(t, sm)
	}
	sm, err = txSystem.Execute(swapTx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	_, billData := getBill(t, rmaTree, swapTx.UnitID())
	require.Equal(t, initialBill.Value, billData.V) // initial bill value is the same after swap
	require.Equal(t, swapTx.Hash(crypto.SHA256), billData.Backlink)
	_, dustCollectorBill := getBill(t, rmaTree, dustCollectorMoneySupplyID)
	require.Equal(t, initialDustCollectorMoneyAmount, dustCollectorBill.V) // dust collector money supply is the same after swap
}

func TestBillData_Value(t *testing.T) {
	bd := &BillData{
		V:        10,
		T:        0,
		Backlink: nil,
	}

	actualSumValue := bd.SummaryValueInput()
	require.Equal(t, uint64(10), actualSumValue)
}

func TestBillData_AddToHasher(t *testing.T) {
	bd := &BillData{
		V:        10,
		T:        50,
		Backlink: []byte("backlink"),
	}

	hasher := crypto.SHA256.New()
	hasher.Write(util.Uint64ToBytes(bd.V))
	hasher.Write(util.Uint64ToBytes(bd.T))
	hasher.Write(bd.Backlink)
	expectedHash := hasher.Sum(nil)
	hasher.Reset()
	bd.Write(hasher)
	actualHash := hasher.Sum(nil)
	require.Equal(t, expectedHash, actualHash)
}

func TestEndBlock_DustBillsAreRemoved(t *testing.T) {
	t.Skip("TODO AB-1133 implement dust bills deletion")
	rmaTree, txSystem, signer := createStateAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	remaining := initBillData.V
	var splitBillIDs = make([]types.UnitID, 10)
	backlink := initBillData.Backlink
	for i := 0; i < 10; i++ {
		remaining--
		splitOk, _ := createSplit(t, initialBill.ID, 1, remaining, script.PredicateAlwaysTrue(), backlink)
		roundNumber := uint64(10)
		txSystem.BeginBlock(roundNumber)
		_, err := txSystem.Execute(splitOk)
		require.NoError(t, err)
		splitBillIDs[i] = NewBillID(splitOk.UnitID(), unitIdFromTransaction(splitOk))

		_, data := getBill(t, rmaTree, initialBill.ID)
		backlink = data.Backlink
	}

	sort.Slice(splitBillIDs, func(i, j int) bool {
		return bytes.Compare(splitBillIDs[i], splitBillIDs[j]) == -1
	})

	// use initial bill as target bill
	targetBillID := initialBill.ID
	_, targetBillData := getBill(t, rmaTree, initialBill.ID)

	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, splitBillIDs, targetBillID, targetBillData.Backlink, rmaTree, signer)

	for _, dcTransfer := range dcTransfers {
		_, err := txSystem.Execute(dcTransfer.TransactionOrder)
		require.NoError(t, err)
	}
	_, err := txSystem.Execute(swapTx)
	require.NoError(t, err)
	_, newBillData := getBill(t, rmaTree, swapTx.UnitID())
	require.Equal(t, uint64(10), newBillData.V)
	_, dustCollectorBill := getBill(t, rmaTree, dustCollectorMoneySupplyID)
	require.Equal(t, initialDustCollectorMoneyAmount, dustCollectorBill.V)
	_, err = txSystem.EndBlock()
	require.NoError(t, err)
	txSystem.Commit()

	txSystem.BeginBlock(defaultDustBillDeletionTimeout + 10)
	_, err = txSystem.EndBlock()
	require.NoError(t, err)
	txSystem.Commit()

	_, dustCollectorBill = getBill(t, rmaTree, dustCollectorMoneySupplyID)
	require.Equal(t, initialDustCollectorMoneyAmount, dustCollectorBill.V)
}

// Test scenario:
// 1) begin block
// 2) process transfer FC (amount=50, fee=1)
// 3) end block (moneyFCB=50+1=51)
// commit
// 1) begin block
// 2) process reclaim FC closeFC(amount=50, fee=1)
// 3) end block (moneyFCB=51-50+1+1=3)
func TestEndBlock_FeesConsolidation(t *testing.T) {
	rmaTree, txSystem, signer := createStateAndTxSystem(t)

	// process transferFC with amount 50 and fees 1
	txSystem.BeginBlock(0)
	transferFC := testfc.NewTransferFC(t,
		testfc.NewTransferFCAttr(
			testfc.WithBacklink(nil),
		),
		testtransaction.WithUnitId(initialBill.ID),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)

	_, err := txSystem.Execute(transferFC)
	require.NoError(t, err)
	_, err = txSystem.EndBlock()
	require.NoError(t, err)
	require.NoError(t, txSystem.Commit())

	// verify that money fee credit bill is 50
	moneyFeeCreditBillID := NewBillID(nil, []byte{2})
	moneyFeeCreditBill, err := rmaTree.GetUnit(moneyFeeCreditBillID, false)

	require.NoError(t, err)
	require.EqualValues(t, 50, moneyFeeCreditBill.Data().SummaryValueInput())

	// process reclaimFC (with closeFC amount=50 and fee=1)
	txSystem.BeginBlock(0)

	transferFCHash := transferFC.Hash(crypto.SHA256)
	closeFC := testfc.NewCloseFC(t,
		testfc.NewCloseFCAttr(
			testfc.WithCloseFCAmount(50),
			testfc.WithCloseFCTargetUnitID(initialBill.ID),
			testfc.WithCloseFCTargetUnitBacklink(transferFCHash),
		),
	)
	closeFCRecord := &types.TransactionRecord{
		TransactionOrder: closeFC,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}

	proof := testblock.CreateProof(t, closeFCRecord, signer)
	reclaimFC := testfc.NewReclaimFC(t, signer,
		testfc.NewReclaimFCAttr(t, signer,
			testfc.WithReclaimFCClosureTx(closeFCRecord),
			testfc.WithReclaimFCClosureProof(proof),
			testfc.WithReclaimFCBacklink(transferFCHash),
		),
		testtransaction.WithUnitId(initialBill.ID),
		testtransaction.WithPayloadType(transactions.PayloadTypeReclaimFeeCredit),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
	_, err = txSystem.Execute(reclaimFC)
	require.NoError(t, err)
	_, err = txSystem.EndBlock()
	require.NoError(t, err)
	require.NoError(t, txSystem.Commit())

	// verify that moneyFCB=50-50+1+1=2 (moneyFCB - closeAmount + closeFee + reclaimFee)
	moneyFeeCreditBill, err = rmaTree.GetUnit(moneyFeeCreditBillID, false)
	require.NoError(t, err)
	require.EqualValues(t, 2, moneyFeeCreditBill.Data().SummaryValueInput())
}

func TestRegisterData_RevertSplit(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)

	vdState, err := txSystem.StateSummary()
	require.NoError(t, err)

	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	splitOk, _ := createSplit(t, initialBill.ID, amount, remaining, script.PredicateAlwaysTrue(), initBillData.Backlink)
	require.NoError(t, err)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)
	sm, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, sm)
	_, err = txSystem.StateSummary()
	require.ErrorIs(t, err, txsystem.ErrStateContainsUncommittedChanges)

	txSystem.Revert()
	s, err := txSystem.StateSummary()
	require.NoError(t, err)
	require.Equal(t, vdState, s)
}

func TestRegisterData_RevertTransDC(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	unit, initBillData := getBill(t, rmaTree, initialBill.ID)
	unitBearer := bytes.Clone(unit.Bearer())
	vdState, err := txSystem.StateSummary()
	require.NoError(t, err)

	transDC, _ := createDCTransfer(t, initialBill.ID, initialBill.Value, initBillData.Backlink, test.RandomBytes(32), test.RandomBytes(32))
	require.NoError(t, err)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)
	sm, err := txSystem.Execute(transDC)
	require.NoError(t, err)
	require.NotNil(t, sm)
	_, err = txSystem.StateSummary()
	require.ErrorIs(t, err, txsystem.ErrStateContainsUncommittedChanges)
	unit, _ = getBill(t, rmaTree, initialBill.ID)
	require.EqualValues(t, dustCollectorPredicate, unit.Bearer())

	txSystem.Revert()
	s, err := txSystem.StateSummary()
	require.NoError(t, err)
	require.Equal(t, vdState, s)
	unit, _ = getBill(t, rmaTree, initialBill.ID)
	require.EqualValues(t, unitBearer, unit.Bearer())
	require.NotEqualValues(t, dustCollectorPredicate, unit.Bearer())
}

// Test Transfer->Add->Close->Reclaim sequence OK
func TestExecute_FeeCreditSequence_OK(t *testing.T) {
	rmaTree, txSystem, signer := createStateAndTxSystem(t)
	txFee := fc.FixedFee(1)()
	initialBillID := initialBill.ID
	fcrUnitID := NewFeeCreditRecordID(nil, []byte{100})
	txAmount := uint64(20)

	txSystem.BeginBlock(1)

	// transfer 20 alphas to FCB
	transferFC := testfc.NewTransferFC(t,
		testfc.NewTransferFCAttr(
			testfc.WithBacklink(nil),
			testfc.WithAmount(txAmount),
			testfc.WithTargetRecordID(fcrUnitID),
		),
		testtransaction.WithUnitId(initialBillID),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
	sm, err := txSystem.Execute(transferFC)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify unit value is reduced by 20
	ib, err := rmaTree.GetUnit(initialBill.ID, false)
	require.NoError(t, err)
	require.EqualValues(t, initialBill.Value-txAmount, ib.Data().SummaryValueInput())
	require.EqualValues(t, txFee, sm.ActualFee)

	// send addFC
	transferFCTransactionRecord := &types.TransactionRecord{
		TransactionOrder: transferFC,
		ServerMetadata:   sm,
	}
	transferFCProof := testblock.CreateProof(t, transferFCTransactionRecord, signer)
	addFC := testfc.NewAddFC(t, signer,
		testfc.NewAddFCAttr(t, signer,
			testfc.WithTransferFCTx(transferFCTransactionRecord),
			testfc.WithTransferFCProof(transferFCProof),
			testfc.WithFCOwnerCondition(script.PredicateAlwaysTrue()),
		),
		testtransaction.WithUnitId(fcrUnitID),
	)
	sm, err = txSystem.Execute(addFC)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify user fee credit is 18 (transfer 20 minus fee 2 * fee)
	remainingValue := txAmount - (2 * txFee) // 18
	fcrUnit, err := rmaTree.GetUnit(fcrUnitID, false)
	require.NoError(t, err)
	fcrUnitData, ok := fcrUnit.Data().(*unit.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, remainingValue, fcrUnitData.Balance)
	require.Equal(t, txFee, sm.ActualFee)

	// send closeFC
	transferFCHash := transferFC.Hash(crypto.SHA256)
	closeFC := testfc.NewCloseFC(t,
		testfc.NewCloseFCAttr(
			testfc.WithCloseFCAmount(remainingValue),
			testfc.WithCloseFCTargetUnitID(initialBillID),
			testfc.WithCloseFCTargetUnitBacklink(transferFCHash),
		),
		testtransaction.WithUnitId(fcrUnitID),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
	sm, err = txSystem.Execute(closeFC)
	require.NoError(t, err)
	require.Equal(t, txFee, sm.ActualFee)

	// verify user fee credit is closed (balance 0, unit will be deleted on round completion)
	fcrUnit, err = rmaTree.GetUnit(fcrUnitID, false)
	require.NoError(t, err)
	fcrUnitData, ok = fcrUnit.Data().(*unit.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 0, fcrUnitData.Balance)

	// send reclaimFC
	closeFCTransactionRecord := &types.TransactionRecord{
		TransactionOrder: closeFC,
		ServerMetadata:   sm,
	}
	closeFCProof := testblock.CreateProof(t, closeFCTransactionRecord, signer)
	reclaimFC := testfc.NewReclaimFC(t, signer,
		testfc.NewReclaimFCAttr(t, signer,
			testfc.WithReclaimFCClosureTx(closeFCTransactionRecord),
			testfc.WithReclaimFCClosureProof(closeFCProof),
			testfc.WithReclaimFCBacklink(transferFCHash),
		),
		testtransaction.WithUnitId(initialBillID),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
		testtransaction.WithPayloadType(transactions.PayloadTypeReclaimFeeCredit),
	)
	sm, err = txSystem.Execute(reclaimFC)
	require.NoError(t, err)
	require.Equal(t, txFee, sm.ActualFee)

	// verify reclaimed fee is added back to initial bill (original value minus 4x txfee)
	ib, err = rmaTree.GetUnit(initialBill.ID, false)
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, initialBill.Value-4*txFee, ib.Data().SummaryValueInput())
}

func unitIdFromTransaction(tx *types.TransactionOrder) []byte {
	hasher := crypto.SHA256.New()
	hasher.Write(tx.UnitID())
	hasher.Write(tx.Payload.Attributes)
	hasher.Write(util.Uint64ToBytes(tx.Timeout()))
	return hasher.Sum(nil)
}

func getBill(t *testing.T, s *state.State, billID types.UnitID) (*state.Unit, *BillData) {
	t.Helper()
	ib, err := s.GetUnit(billID, false)
	require.NoError(t, err)
	require.IsType(t, &BillData{}, ib.Data())
	return ib, ib.Data().(*BillData)
}

func createBillTransfer(t *testing.T, fromID types.UnitID, value uint64, bearer []byte, backlink []byte) (*types.TransactionOrder, *TransferAttributes) {
	tx := createTx(fromID, PayloadTypeTransfer)
	bt := &TransferAttributes{
		NewBearer: bearer,
		// #nosec G404
		TargetValue: value,
		Backlink:    backlink,
	}
	rawBytes, err := cbor.Marshal(bt)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return tx, bt
}

func createDCTransferAndSwapTxs(
	t *testing.T,
	ids []types.UnitID, // bills to swap
	targetID []byte,
	targetBacklink []byte,
	rmaTree *state.State,
	signer abcrypto.Signer) ([]*types.TransactionRecord, *types.TransactionOrder) {

	t.Helper()

	// create dc transfers
	dcTransfers := make([]*types.TransactionRecord, len(ids))
	proofs := make([]*types.TxProof, len(ids))

	var targetValue uint64 = 0
	for i, id := range ids {
		_, billData := getBill(t, rmaTree, id)
		// NB! dc transfer target backlink must be equal to swap tx unit id
		targetValue += billData.V
		tx, _ := createDCTransfer(t, id, billData.V, billData.Backlink, targetID, targetBacklink)
		txr := &types.TransactionRecord{
			TransactionOrder: tx,
			ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
		}
		proofs[i] = testblock.CreateProof(t, txr, signer)
		dcTransfers[i] = txr
	}

	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID: moneySystemID,
			UnitID:   targetID,
			Type:     PayloadTypeSwapDC,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           20,
				MaxTransactionFee: 10,
				FeeCreditRecordID: fcrID,
			},
		},
		OwnerProof: script.PredicateArgumentEmpty(),
	}

	bt := &SwapDCAttributes{
		OwnerCondition:   script.PredicateAlwaysTrue(),
		DcTransfers:      dcTransfers,
		DcTransferProofs: proofs,
		TargetValue:      targetValue,
	}
	rawBytes, err := cbor.Marshal(bt)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return dcTransfers, tx
}

func createDCTransfer(t *testing.T, fromID types.UnitID, val uint64, backlink []byte, targetID []byte, targetBacklink []byte) (*types.TransactionOrder, *TransferDCAttributes) {
	tx := createTx(fromID, PayloadTypeTransDC)
	bt := &TransferDCAttributes{
		Value:              val,
		TargetUnitID:       targetID,
		TargetUnitBacklink: targetBacklink,
		Backlink:           backlink,
	}
	rawBytes, err := cbor.Marshal(bt)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return tx, bt
}

func createSplit(t *testing.T, fromID types.UnitID, amount, remainingValue uint64, targetBearer, backlink []byte) (*types.TransactionOrder, *SplitAttributes) {
	tx := createTx(fromID, PayloadTypeSplit)
	bt := &SplitAttributes{
		Amount:         amount,
		TargetBearer:   targetBearer,
		RemainingValue: remainingValue,
		Backlink:       backlink,
	}
	rawBytes, err := cbor.Marshal(bt)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return tx, bt
}

func createTx(fromID types.UnitID, payloadType string) *types.TransactionOrder {
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   DefaultSystemIdentifier,
			UnitID:     fromID,
			Type:       payloadType,
			Attributes: nil,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           20,
				MaxTransactionFee: 10,
				FeeCreditRecordID: fcrID,
			},
		},

		OwnerProof: script.PredicateArgumentEmpty(),
		FeeProof:   script.PredicateArgumentEmpty(),
	}
	return tx
}

func createStateAndTxSystem(t *testing.T) (*state.State, *txsystem.GenericTxSystem, abcrypto.Signer) {
	s := state.NewEmptyState()
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}

	mss, err := NewTxSystem(
		WithSystemIdentifier(systemIdentifier),
		WithInitialBill(initialBill),
		WithSystemDescriptionRecords(createSDRs(newBillID(2))),
		WithDCMoneyAmount(initialDustCollectorMoneyAmount),
		WithState(s),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)
	summary, err := mss.StateSummary()
	require.NoError(t, err)
	require.NotNil(t, summary.Summary())
	require.NotNil(t, summary.Root())
	require.Len(t, summary.Root(), crypto.SHA256.Size())
	require.True(t, s.IsCommitted())
	// add fee credit record with empty predicate
	fcrData := &unit.FeeCreditRecord{
		Balance: 100,
		Timeout: 100,
	}
	err = s.Apply(unit.AddCredit(fcrID, script.PredicateAlwaysTrue(), fcrData))
	require.NoError(t, err)
	_, err = mss.EndBlock()
	require.NoError(t, err)
	mss.Commit()

	return s, mss, signer
}

func createSDRs(fcbID types.UnitID) []*genesis.SystemDescriptionRecord {
	return []*genesis.SystemDescriptionRecord{{
		SystemIdentifier: moneySystemID,
		T2Timeout:        2500,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         fcbID,
			OwnerPredicate: script.PredicateAlwaysTrue(),
		},
	}}
}
