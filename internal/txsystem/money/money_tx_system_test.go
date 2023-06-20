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
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

const initialDustCollectorMoneyAmount uint64 = 100

var (
	initialBill   = &InitialBill{ID: []byte{0, 0, 0, 77}, Value: 110, Owner: script.PredicateAlwaysTrue()}
	fcrID         = []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 88}
	moneySystemID = []byte{0, 0, 0, 0}
)

func TestNewMoneyTxSystem(t *testing.T) {
	var (
		txsState      = state.NewEmptyState()
		dcMoneyAmount = uint64(222)
		sdrs          = createSDRs(3)
		_, verifier   = testsig.CreateSignerAndVerifier(t)
		trustBase     = map[string]abcrypto.Verifier{"test": verifier}
	)
	txSystem, err := NewMoneyTxSystem(
		moneySystemID,
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
	_, err := NewMoneyTxSystem(
		moneySystemID,
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(nil),
		WithSystemDescriptionRecords(createSDRs(2)),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrInitialBillIsNil)
}

func TestNewMoneyTxSystem_InvalidInitialBillID(t *testing.T) {
	ib := &InitialBill{ID: dustCollectorMoneySupplyID, Value: 100, Owner: nil}
	_, err := NewMoneyTxSystem(
		moneySystemID,
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(ib),
		WithSystemDescriptionRecords(createSDRs(2)),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrInvalidInitialBillID)
}

func TestNewMoneyTxSystem_InvalidFeeCreditBill_Nil(t *testing.T) {
	_, err := NewMoneyTxSystem(
		moneySystemID,
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(&InitialBill{ID: []byte{1}, Value: 100, Owner: nil}),
		WithSystemDescriptionRecords(nil),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrUndefinedSystemDescriptionRecords)
}

func TestNewMoneyTxSystem_InvalidFeeCreditBill_SameIDAsInitialBill(t *testing.T) {
	_, err := NewMoneyTxSystem(
		moneySystemID,
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(&InitialBill{ID: []byte{1}, Value: 100, Owner: nil}),
		WithSystemDescriptionRecords(createSDRs(1)),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrInvalidFeeCreditBillID)
}

func TestNewMoneyScheme_InvalidFeeCreditBill_SameIDAsDCBill(t *testing.T) {
	_, err := NewMoneyTxSystem(
		moneySystemID,
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(&InitialBill{ID: []byte{0}, Value: 100, Owner: nil}),
		WithSystemDescriptionRecords(createSDRs(0)),
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
	txSystem.Commit()

	trx := &types.TransactionRecord{
		TransactionOrder: transferOk,
		ServerMetadata:   serverMetadata,
	}

	unit2, data2 := getBill(t, rmaTree, initialBill.ID)
	require.Equal(t, data.SummaryValueInput(), data2.SummaryValueInput())
	require.NotEqual(t, transferOk.OwnerProof, unit2.Bearer())
	require.Equal(t, roundNumber, data2.T)
	require.EqualValues(t, trx.Hash(crypto.SHA256), data2.Backlink)
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

	splitOkTransacrionRecord := &types.TransactionRecord{
		TransactionOrder: splitOk,
		ServerMetadata:   sm,
	}

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

	expectedNewUnitId := txutil.SameShardID(splitOk.UnitID(), unitIdFromTransaction(splitOk))
	newBill, bd := getBill(t, rmaTree, expectedNewUnitId)
	require.NotNil(t, newBill)
	require.NotNil(t, bd)
	require.Equal(t, amount, bd.V)
	require.EqualValues(t, splitOkTransacrionRecord.Hash(crypto.SHA256), bd.Backlink)
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
	billID := txutil.SameShardID(splitOk.UnitID(), unitIdFromTransaction(splitOk))
	_, splitBillData := getBill(t, rmaTree, billID)

	transferDCOk, _ := createDCTransfer(t, billID, splitBillData.V, splitBillData.Backlink, test.RandomBytes(32), script.PredicateAlwaysTrue())
	require.NoError(t, err)

	sm, err = txSystem.Execute(transferDCOk)
	require.NoError(t, err)
	require.NotNil(t, sm)

	transferDCTransactionRecord := &types.TransactionRecord{
		TransactionOrder: transferDCOk,
		ServerMetadata:   sm,
	}

	transferDCBill, transferDCBillData := getBill(t, rmaTree, billID)
	require.NotEqual(t, dustCollectorPredicate, transferDCBill.Bearer())
	require.Equal(t, splitBillData.SummaryValueInput(), transferDCBillData.SummaryValueInput())
	require.Equal(t, roundNumber, transferDCBillData.T)
	require.EqualValues(t, transferDCTransactionRecord.Hash(crypto.SHA256), transferDCBillData.Backlink)
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

	splitBillID := txutil.SameShardID(splitOk.UnitID(), unitIdFromTransaction(splitOk))
	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, []types.UnitID{splitBillID}, rmaTree, signer)

	for _, dcTransfer := range dcTransfers {
		sm, err = txSystem.Execute(dcTransfer.TransactionOrder)
		require.NoError(t, err)
		require.NotNil(t, sm)
	}
	sm, err = txSystem.Execute(swapTx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	_, billData := getBill(t, rmaTree, swapTx.UnitID())
	require.Equal(t, amount, billData.V)
	_, dustBill := getBill(t, rmaTree, dustCollectorMoneySupplyID)
	require.Equal(t, amount, initialDustCollectorMoneyAmount-dustBill.V)
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
		splitBillIDs[i] = txutil.SameShardID(splitOk.UnitID(), unitIdFromTransaction(splitOk))

		_, data := getBill(t, rmaTree, initialBill.ID)
		backlink = data.Backlink
	}

	sort.Slice(splitBillIDs, func(i, j int) bool {
		return bytes.Compare(splitBillIDs[i], splitBillIDs[j]) == -1
	})
	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, splitBillIDs, rmaTree, signer)

	for _, dcTransfer := range dcTransfers {
		_, err := txSystem.Execute(dcTransfer.TransactionOrder)
		require.NoError(t, err)
	}
	_, err := txSystem.Execute(swapTx)
	require.NoError(t, err)
	_, newBillData := getBill(t, rmaTree, swapTx.UnitID())
	require.Equal(t, uint64(10), newBillData.V)
	_, dustBill := getBill(t, rmaTree, dustCollectorMoneySupplyID)
	require.Equal(t, uint64(10), initialDustCollectorMoneyAmount-dustBill.V)
	_, err = txSystem.EndBlock()
	require.NoError(t, err)
	txSystem.Commit()

	txSystem.BeginBlock(defaultDustBillDeletionTimeout + 10)
	_, err = txSystem.EndBlock()
	require.NoError(t, err)
	txSystem.Commit()

	_, dustBill = getBill(t, rmaTree, dustCollectorMoneySupplyID)
	require.Equal(t, initialDustCollectorMoneyAmount, dustBill.V)
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

	sm, err := txSystem.Execute(transferFC)
	transferFCRecord := &types.TransactionRecord{
		TransactionOrder: transferFC,
		ServerMetadata:   sm,
	}
	require.NoError(t, err)
	_, err = txSystem.EndBlock()
	require.NoError(t, err)
	txSystem.Commit()

	// verify that money fee credit bill is 50+1=51
	moneyFCUnitID := []byte{2}
	moneyFCUnit, err := rmaTree.GetUnit(moneyFCUnitID, false)
	require.NoError(t, err)
	require.EqualValues(t, 51, moneyFCUnit.Data().SummaryValueInput())

	// process reclaimFC (with closeFC amount=50 and fee=1)
	txSystem.BeginBlock(0)

	transferFCHash := transferFCRecord.Hash(crypto.SHA256)
	closeFC := testfc.NewCloseFC(t,
		testfc.NewCloseFCAttr(
			testfc.WithCloseFCAmount(50),
			testfc.WithCloseFCTargetUnitID(initialBill.ID),
			testfc.WithCloseFCNonce(transferFCHash),
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
	txSystem.Commit()

	// verify that moneyFCB=51-50+1+1=3 (moneyFCB - closeAmount + closeFee + reclaimFee)
	moneyFCUnit, err = rmaTree.GetUnit(moneyFCUnitID, false)
	require.NoError(t, err)
	require.EqualValues(t, 3, moneyFCUnit.Data().SummaryValueInput())
}

func TestValidateSwap_InsufficientDcMoneySupply(t *testing.T) {
	rmaTree, txSystem, signer := createStateAndTxSystem(t)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)
	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, []types.UnitID{initialBill.ID}, rmaTree, signer)

	for _, dcTransfer := range dcTransfers {
		_, err := txSystem.Execute(dcTransfer.TransactionOrder)
		require.NoError(t, err)
	}
	_, err := txSystem.Execute(swapTx)
	require.ErrorIs(t, err, ErrSwapInsufficientDCMoneySupply)
}

func TestValidateSwap_SwapBillAlreadyExists(t *testing.T) {
	rmaTree, txSystem, signer := createStateAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)

	var remaining uint64 = 99
	amount := initialBill.Value - remaining
	splitOk, _ := createSplit(t, initialBill.ID, amount, remaining, script.PredicateAlwaysTrue(), initBillData.Backlink)
	txSystem.BeginBlock(roundNumber)
	sm, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, sm)

	splitBillID := txutil.SameShardID(splitOk.UnitID(), unitIdFromTransaction(splitOk))

	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, []types.UnitID{splitBillID}, rmaTree, signer)

	err = rmaTree.Apply(state.AddUnit(swapTx.UnitID(), script.PredicateAlwaysTrue(), &BillData{}))
	require.NoError(t, err)
	for _, dcTransfer := range dcTransfers {
		_, err = txSystem.Execute(dcTransfer.TransactionOrder)
		require.NoError(t, err)
	}
	_, err = txSystem.Execute(swapTx)
	require.ErrorIs(t, err, ErrSwapBillAlreadyExists)
}

func TestRegisterData_Revert(t *testing.T) {
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

// Test Transfer->Add->Close->Reclaim sequence OK
func TestExecute_FeeCreditSequence_OK(t *testing.T) {
	rmaTree, txSystem, signer := createStateAndTxSystem(t)
	txFee := fc.FixedFee(1)()
	initialBillID := initialBill.ID
	fcrUnitID := util.Uint256ToBytes(uint256.NewInt(100))
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

	// verify unit value is reduced by 21
	ib, err := rmaTree.GetUnit(initialBill.ID, false)
	require.NoError(t, err)
	require.EqualValues(t, initialBill.Value-txAmount-txFee, ib.Data().SummaryValueInput())
	require.Equal(t, txFee, sm.ActualFee)

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

	// verify user fee credit is 19 (transfer 20 minus fee 1)
	remainingValue := txAmount - txFee // 19
	fcrUnit, err := rmaTree.GetUnit(fcrUnitID, false)
	require.NoError(t, err)
	fcrUnitData, ok := fcrUnit.Data().(*unit.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, remainingValue, fcrUnitData.Balance)
	require.Equal(t, txFee, sm.ActualFee)

	// send closeFC
	transferFCHash := transferFCTransactionRecord.Hash(crypto.SHA256)
	closeFC := testfc.NewCloseFC(t,
		testfc.NewCloseFCAttr(
			testfc.WithCloseFCAmount(remainingValue),
			testfc.WithCloseFCTargetUnitID(initialBillID),
			testfc.WithCloseFCNonce(transferFCHash),
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
	require.IsType(t, ib.Data(), &BillData{})
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
	rmaTree *state.State,
	signer abcrypto.Signer) ([]*types.TransactionRecord, *types.TransactionOrder) {

	t.Helper()
	// calculate new bill ID
	hasher := crypto.SHA256.New()
	idsByteArray := make([][]byte, len(ids))
	for i, id := range ids {
		hasher.Write(id)
		idsByteArray[i] = id
	}
	newBillID := hasher.Sum(nil)

	// create dc transfers
	dcTransfers := make([]*types.TransactionRecord, len(ids))
	proofs := make([]*types.TxProof, len(ids))

	var targetValue uint64 = 0
	for i, id := range ids {
		_, billData := getBill(t, rmaTree, id)
		// NB! dc transfer nonce must be equal to swap tx unit id
		targetValue += billData.V
		tx, _ := createDCTransfer(t, id, billData.V, billData.Backlink, newBillID, script.PredicateAlwaysTrue())
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
			UnitID:   newBillID,
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
		OwnerCondition:  script.PredicateAlwaysTrue(),
		BillIdentifiers: idsByteArray,
		DcTransfers:     dcTransfers,
		Proofs:          proofs,
		TargetValue:     targetValue,
	}
	rawBytes, err := cbor.Marshal(bt)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return dcTransfers, tx
}

func createDCTransfer(t *testing.T, fromID types.UnitID, targetValue uint64, backlink []byte, nonce []byte, targetBearer []byte) (*types.TransactionOrder, *TransferDCAttributes) {
	tx := createTx(fromID, PayloadTypeTransDC)
	bt := &TransferDCAttributes{
		Nonce:        nonce,
		TargetBearer: targetBearer,
		TargetValue:  targetValue,
		Backlink:     backlink,
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
			SystemID:   []byte{0, 0, 0, 0},
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

	mss, err := NewMoneyTxSystem(
		[]byte{0, 0, 0, 0},
		WithInitialBill(initialBill),
		WithSystemDescriptionRecords(createSDRs(2)),
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

func createSDRs(id uint8) []*genesis.SystemDescriptionRecord {
	return []*genesis.SystemDescriptionRecord{{
		SystemIdentifier: moneySystemID,
		T2Timeout:        2500,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         []byte{id},
			OwnerPredicate: script.PredicateAlwaysTrue(),
		},
	}}
}
