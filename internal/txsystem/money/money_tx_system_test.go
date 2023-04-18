package money

import (
	"crypto"
	"sort"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/rma"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/fc"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	txutil "github.com/alphabill-org/alphabill/internal/txsystem/util"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	initialBill = &InitialBill{ID: uint256.NewInt(77), Value: 110, Owner: script.PredicateAlwaysTrue()}
	fcrID       = uint256.NewInt(88)
)

const initialDustCollectorMoneyAmount uint64 = 100

func TestNewMoneyScheme(t *testing.T) {
	var (
		state         = rma.NewWithSHA256()
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
		WithState(state),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)
	require.NotNil(t, txSystem)

	u, err := state.GetUnit(initialBill.ID)
	require.NoError(t, err)
	require.NotNil(t, u)
	require.Equal(t, rma.Uint64SummaryValue(initialBill.Value), u.Data.Value())
	require.Equal(t, initialBill.Owner, u.Bearer)

	d, err := state.GetUnit(dustCollectorMoneySupplyID)
	require.NoError(t, err)
	require.NotNil(t, d)

	require.Equal(t, rma.Uint64SummaryValue(dcMoneyAmount), d.Data.Value())
	require.Equal(t, rma.Predicate(dustCollectorPredicate), d.Bearer)
}

func TestNewMoneyScheme_InitialBillIsNil(t *testing.T) {
	_, err := NewMoneyTxSystem(
		moneySystemID,
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(nil),
		WithSystemDescriptionRecords(createSDRs(2)),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrInitialBillIsNil)
}

func TestNewMoneyScheme_InvalidInitialBillID(t *testing.T) {
	ib := &InitialBill{ID: uint256.NewInt(0), Value: 100, Owner: nil}
	_, err := NewMoneyTxSystem(
		moneySystemID,
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(ib),
		WithSystemDescriptionRecords(createSDRs(2)),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrInvalidInitialBillID)
}

func TestNewMoneyScheme_InvalidFeeCreditBill_Nil(t *testing.T) {
	_, err := NewMoneyTxSystem(
		moneySystemID,
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(&InitialBill{ID: uint256.NewInt(1), Value: 100, Owner: nil}),
		WithSystemDescriptionRecords(nil),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrUndefinedSystemDescriptionRecords)
}

func TestNewMoneyScheme_InvalidFeeCreditBill_SameIDAsInitialBill(t *testing.T) {
	_, err := NewMoneyTxSystem(
		moneySystemID,
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(&InitialBill{ID: uint256.NewInt(1), Value: 100, Owner: nil}),
		WithSystemDescriptionRecords(createSDRs(1)),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrInvalidFeeCreditBillID)
}

func TestNewMoneyScheme_InvalidFeeCreditBill_SameIDAsDCBill(t *testing.T) {
	_, err := NewMoneyTxSystem(
		moneySystemID,
		WithHashAlgorithm(crypto.SHA256),
		WithInitialBill(&InitialBill{ID: uint256.NewInt(1), Value: 100, Owner: nil}),
		WithSystemDescriptionRecords(createSDRs(0)),
		WithDCMoneyAmount(10))
	require.ErrorIs(t, err, ErrInvalidFeeCreditBillID)
}

func TestExecute_TransferOk(t *testing.T) {
	rmaTree, txSystem, _ := createRMATreeAndTxSystem(t)
	unit, data := getBill(t, rmaTree, initialBill.ID)

	transferOk, err := NewMoneyTx(systemIdentifier, createBillTransfer(initialBill.ID, initialBill.Value, script.PredicateAlwaysTrue(), nil))
	require.NoError(t, err)
	roundNumber := uint64(1)
	txSystem.BeginBlock(roundNumber)
	err = txSystem.Execute(transferOk)
	txSystem.Commit()
	require.NoError(t, err)
	unit2, data2 := getBill(t, rmaTree, initialBill.ID)
	require.Equal(t, data.Value(), data2.Value())
	require.NotEqual(t, transferOk.OwnerProof(), unit2.Bearer)
	require.NotEqual(t, unit.StateHash, unit2.StateHash)
	require.EqualValues(t, transferOk.Hash(crypto.SHA256), data2.Backlink)
	require.Equal(t, roundNumber, data2.T)
}

func TestExecute_SplitOk(t *testing.T) {
	rmaTree, txSystem, _ := createRMATreeAndTxSystem(t)
	totalValue := rmaTree.TotalValue()
	initBill, initBillData := getBill(t, rmaTree, initialBill.ID)
	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	splitOk, err := NewMoneyTx(systemIdentifier, createSplit(initialBill.ID, amount, remaining, script.PredicateAlwaysTrue(), initBillData.Backlink))
	require.NoError(t, err)
	roundNumber := uint64(1)
	txSystem.BeginBlock(roundNumber)
	err = txSystem.Execute(splitOk)
	txSystem.Commit()
	require.NoError(t, err)
	initBillAfterUpdate, initBillDataAfterUpdate := getBill(t, rmaTree, initialBill.ID)

	// bill value was reduced
	require.NotEqual(t, initBillData.V, initBillDataAfterUpdate.V)
	require.Equal(t, remaining, initBillDataAfterUpdate.V)
	// total value was not changed
	require.Equal(t, totalValue, rmaTree.TotalValue())
	// bearer of the initial bill was not changed
	require.Equal(t, initBill.Bearer, initBillAfterUpdate.Bearer)
	require.Equal(t, roundNumber, initBillDataAfterUpdate.T)

	splitWrapper := splitOk.(*billSplitWrapper)
	expectedNewUnitId := txutil.SameShardID(splitOk.UnitID(), unitIdFromTransaction(splitWrapper))
	newBill, newBillData := getBill(t, rmaTree, expectedNewUnitId)
	require.NotNil(t, newBill)
	require.NotNil(t, newBillData)
	require.Equal(t, amount, newBillData.V)
	require.EqualValues(t, splitOk.Hash(crypto.SHA256), newBillData.Backlink)
	require.Equal(t, rma.Predicate(splitWrapper.billSplit.TargetBearer), newBill.Bearer)
	require.Equal(t, roundNumber, newBillData.T)
}

func TestExecute_TransferDCOk(t *testing.T) {
	rmaTree, txSystem, _ := createRMATreeAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	splitOk, err := NewMoneyTx(systemIdentifier, createSplit(initialBill.ID, amount, remaining, script.PredicateAlwaysTrue(), initBillData.Backlink))
	require.NoError(t, err)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)
	err = txSystem.Execute(splitOk)
	require.NoError(t, err)

	splitWrapper := splitOk.(*billSplitWrapper)
	billID := txutil.SameShardID(splitOk.UnitID(), unitIdFromTransaction(splitWrapper))
	_, splitBillData := getBill(t, rmaTree, billID)
	transferDCOk, err := NewMoneyTx(systemIdentifier, createDCTransfer(billID, splitBillData.V, splitBillData.Backlink, test.RandomBytes(32), script.PredicateAlwaysTrue()))
	require.NoError(t, err)

	err = txSystem.Execute(transferDCOk)
	require.NoError(t, err)

	transferDCBill, transferDCBillData := getBill(t, rmaTree, billID)
	require.NotEqual(t, dustCollectorPredicate, transferDCBill.Bearer)
	require.Equal(t, splitBillData.Value(), transferDCBillData.Value())
	require.Equal(t, roundNumber, transferDCBillData.T)
	require.EqualValues(t, transferDCOk.Hash(crypto.SHA256), transferDCBillData.Backlink)
}

func TestExecute_SwapOk(t *testing.T) {
	rmaTree, txSystem, signer := createRMATreeAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	var remaining uint64 = 99
	amount := initialBill.Value - remaining
	splitOk, err := NewMoneyTx(systemIdentifier, createSplit(initialBill.ID, amount, remaining, script.PredicateAlwaysTrue(), initBillData.Backlink))
	require.NoError(t, err)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)
	err = txSystem.Execute(splitOk)
	require.NoError(t, err)

	splitWrapper := splitOk.(*billSplitWrapper)
	splitBillID := txutil.SameShardID(splitOk.UnitID(), unitIdFromTransaction(splitWrapper))

	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, []*uint256.Int{splitBillID}, rmaTree, signer)

	for _, dcTransfer := range dcTransfers {
		tx, err := NewMoneyTx(systemIdentifier, dcTransfer)
		require.NoError(t, err)
		err = txSystem.Execute(tx)
		require.NoError(t, err)
	}
	rmaTree.GetRootHash()
	swap, err := NewMoneyTx(systemIdentifier, swapTx)
	require.NoError(t, err)
	err = txSystem.Execute(swap)
	require.NoError(t, err)
	_, newBillData := getBill(t, rmaTree, swap.UnitID())
	require.Equal(t, amount, newBillData.V)
	_, dustBill := getBill(t, rmaTree, dustCollectorMoneySupplyID)
	require.Equal(t, amount, initialDustCollectorMoneyAmount-dustBill.V)
}

func TestBillData_Value(t *testing.T) {
	bd := &BillData{
		V:        10,
		T:        0,
		Backlink: nil,
	}

	actualSumValue := bd.Value()
	require.Equal(t, rma.Uint64SummaryValue(10), actualSumValue)
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
	bd.AddToHasher(hasher)
	actualHash := hasher.Sum(nil)
	require.Equal(t, expectedHash, actualHash)
}

func TestBillSummary_Concatenate(t *testing.T) {
	self := rma.Uint64SummaryValue(10)
	left := rma.Uint64SummaryValue(2)
	right := rma.Uint64SummaryValue(3)

	actualSum := self.Concatenate(left, right)
	require.Equal(t, rma.Uint64SummaryValue(15), actualSum)

	actualSum = self.Concatenate(nil, nil)
	require.Equal(t, rma.Uint64SummaryValue(10), actualSum)

	actualSum = self.Concatenate(left, nil)
	require.Equal(t, rma.Uint64SummaryValue(12), actualSum)

	actualSum = self.Concatenate(nil, right)
	require.Equal(t, rma.Uint64SummaryValue(13), actualSum)
}

func TestBillSummary_AddToHasher(t *testing.T) {
	bs := rma.Uint64SummaryValue(10)

	hasher := crypto.SHA256.New()
	hasher.Write(util.Uint64ToBytes(10))
	expectedHash := hasher.Sum(nil)
	hasher.Reset()

	bs.AddToHasher(hasher)
	actualHash := hasher.Sum(nil)
	require.Equal(t, expectedHash, actualHash)
}

func TestEndBlock_DustBillsAreRemoved(t *testing.T) {
	rmaTree, txSystem, signer := createRMATreeAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	remaining := initBillData.V
	var splitBillIDs = make([]*uint256.Int, 10)
	backlink := initBillData.Backlink
	for i := 0; i < 10; i++ {
		remaining--
		splitOk, err := NewMoneyTx(systemIdentifier, createSplit(initialBill.ID, 1, remaining, script.PredicateAlwaysTrue(), backlink))

		require.NoError(t, err)
		roundNumber := uint64(10)
		txSystem.BeginBlock(roundNumber)
		err = txSystem.Execute(splitOk)
		require.NoError(t, err)
		splitWrapper := splitOk.(*billSplitWrapper)
		splitBillIDs[i] = txutil.SameShardID(splitOk.UnitID(), unitIdFromTransaction(splitWrapper))

		_, data := getBill(t, rmaTree, initialBill.ID)
		backlink = data.Backlink
	}

	sort.Slice(splitBillIDs, func(i, j int) bool {
		return splitBillIDs[i].Lt(splitBillIDs[j])
	})
	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, splitBillIDs, rmaTree, signer)

	for _, dcTransfer := range dcTransfers {
		tx, err := NewMoneyTx(systemIdentifier, dcTransfer)
		require.NoError(t, err)
		err = txSystem.Execute(tx)
		require.NoError(t, err)
	}
	rmaTree.GetRootHash()
	swap, err := NewMoneyTx(systemIdentifier, swapTx)
	require.NoError(t, err)
	err = txSystem.Execute(swap)
	require.NoError(t, err)
	_, newBillData := getBill(t, rmaTree, swap.UnitID())
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
	rmaTree, txSystem, signer := createRMATreeAndTxSystem(t)

	// process transferFC with amount 50 and fees 1
	txSystem.BeginBlock(0)
	transferFC := testfc.NewTransferFC(t,
		testfc.NewTransferFCAttr(
			testfc.WithBacklink(nil),
		),
		testtransaction.WithUnitId(util.Uint256ToBytes(initialBill.ID)),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)

	require.NoError(t, txSystem.Execute(transferFC))
	_, err := txSystem.EndBlock()
	require.NoError(t, err)
	txSystem.Commit()

	// verify that money fee credit bill is 50+1=51
	moneyFCUnitID := uint256.NewInt(2)
	moneyFCUnit, err := rmaTree.GetUnit(moneyFCUnitID)
	require.NoError(t, err)
	require.EqualValues(t, 51, moneyFCUnit.Data.Value())

	// process reclaimFC (with closeFC amount=50 and fee=1)
	txSystem.BeginBlock(0)

	transferFCHash := transferFC.Hash(crypto.SHA256)
	closeFC := testfc.NewCloseFC(t,
		testfc.NewCloseFCAttr(
			testfc.WithCloseFCAmount(50),
			testfc.WithCloseFCTargetUnitID(util.Uint256ToBytes(initialBill.ID)),
			testfc.WithCloseFCNonce(transferFCHash),
		),
	)

	closeFCPb := closeFC.Transaction
	proof := testblock.CreateProof(t, closeFC, signer, closeFCPb.UnitId)
	reclaimFC := testfc.NewReclaimFC(t, signer,
		testfc.NewReclaimFCAttr(t, signer,
			testfc.WithReclaimFCClosureTx(closeFCPb),
			testfc.WithReclaimFCClosureProof(proof),
			testfc.WithReclaimFCBacklink(transferFCHash),
		),
		testtransaction.WithUnitId(util.Uint256ToBytes(initialBill.ID)),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
	err = txSystem.Execute(reclaimFC)
	require.NoError(t, err)
	_, err = txSystem.EndBlock()
	require.NoError(t, err)
	txSystem.Commit()

	// verify that moneyFCB=51-50+1+1=3 (moneyFCB - closeAmount + closeFee + reclaimFee)
	moneyFCUnit, err = rmaTree.GetUnit(moneyFCUnitID)
	require.NoError(t, err)
	require.EqualValues(t, 3, moneyFCUnit.Data.Value())
}

func TestValidateSwap_InsufficientDcMoneySupply(t *testing.T) {
	rmaTree, txSystem, signer := createRMATreeAndTxSystem(t)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)
	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, []*uint256.Int{initialBill.ID}, rmaTree, signer)

	for _, dcTransfer := range dcTransfers {
		tx, err := NewMoneyTx(systemIdentifier, dcTransfer)
		require.NoError(t, err)
		err = txSystem.Execute(tx)
		require.NoError(t, err)
	}
	tx, err := NewMoneyTx(systemIdentifier, swapTx)
	require.NoError(t, err)
	err = txSystem.Execute(tx)
	require.ErrorIs(t, err, ErrSwapInsufficientDCMoneySupply)
}

func TestValidateSwap_SwapBillAlreadyExists(t *testing.T) {
	rmaTree, txSystem, signer := createRMATreeAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)

	var remaining uint64 = 99
	amount := initialBill.Value - remaining
	splitOk, err := NewMoneyTx(systemIdentifier, createSplit(initialBill.ID, amount, remaining, script.PredicateAlwaysTrue(), initBillData.Backlink))
	require.NoError(t, err)
	txSystem.BeginBlock(roundNumber)
	err = txSystem.Execute(splitOk)
	require.NoError(t, err)

	splitWrapper := splitOk.(*billSplitWrapper)
	splitBillID := txutil.SameShardID(splitOk.UnitID(), unitIdFromTransaction(splitWrapper))

	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, []*uint256.Int{splitBillID}, rmaTree, signer)

	err = rmaTree.AtomicUpdate(rma.AddItem(uint256.NewInt(0).SetBytes(swapTx.UnitId), script.PredicateAlwaysTrue(), &BillData{}, []byte{}))
	require.NoError(t, err)
	for _, dcTransfer := range dcTransfers {
		tx, err := NewMoneyTx(systemIdentifier, dcTransfer)
		require.NoError(t, err)
		err = txSystem.Execute(tx)
		require.NoError(t, err)
	}
	tx, err := NewMoneyTx(systemIdentifier, swapTx)
	require.NoError(t, err)
	err = txSystem.Execute(tx)
	require.ErrorIs(t, err, ErrSwapBillAlreadyExists)
}

func TestRegisterData_Revert(t *testing.T) {
	rmaTree, txSystem, _ := createRMATreeAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)

	vdState, err := txSystem.State()
	require.NoError(t, err)

	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	splitOk, err := NewMoneyTx(systemIdentifier, createSplit(initialBill.ID, amount, remaining, script.PredicateAlwaysTrue(), initBillData.Backlink))
	require.NoError(t, err)
	roundNumber := uint64(10)
	txSystem.BeginBlock(roundNumber)
	err = txSystem.Execute(splitOk)
	require.NoError(t, err)
	_, err = txSystem.State()
	require.ErrorIs(t, err, txsystem.ErrStateContainsUncommittedChanges)

	txSystem.Revert()
	state, err := txSystem.State()
	require.NoError(t, err)
	require.Equal(t, vdState, state)
}

// Test Transfer->Add->Close->Reclaim sequence OK
func TestExecute_FeeCreditSequence_OK(t *testing.T) {
	rmaTree, txSystem, signer := createRMATreeAndTxSystem(t)
	txFee := fc.FixedFee(1)()
	initialBillID := util.Uint256ToBytes(initialBill.ID)
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
	err := txSystem.Execute(transferFC)
	require.NoError(t, err)

	// verify unit value is reduced by 21
	ib, err := rmaTree.GetUnit(initialBill.ID)
	require.NoError(t, err)
	require.EqualValues(t, initialBill.Value-txAmount-txFee, ib.Data.Value())
	require.Equal(t, txFee, transferFC.Transaction.ServerMetadata.Fee)

	// send addFC
	transferFCPb := transferFC.Transaction
	transferFCProof := testblock.CreateProof(t, transferFC, signer, transferFCPb.UnitId)
	addFC := testfc.NewAddFC(t, signer,
		testfc.NewAddFCAttr(t, signer,
			testfc.WithTransferFCTx(transferFCPb),
			testfc.WithTransferFCProof(transferFCProof),
			testfc.WithFCOwnerCondition(script.PredicateAlwaysTrue()),
		),
		testtransaction.WithUnitId(fcrUnitID),
	)
	err = txSystem.Execute(addFC)
	require.NoError(t, err)

	// verify user fee credit is 19 (transfer 20 minus fee 1)
	remainingValue := txAmount - txFee // 19
	fcrUnit, err := rmaTree.GetUnit(uint256.NewInt(0).SetBytes(fcrUnitID))
	require.NoError(t, err)
	fcrUnitData, ok := fcrUnit.Data.(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, remainingValue, fcrUnitData.Balance)
	require.Equal(t, txFee, addFC.Transaction.ServerMetadata.Fee)

	// send closeFC
	transferFCHash := transferFC.Hash(crypto.SHA256)
	closeFC := testfc.NewCloseFC(t,
		testfc.NewCloseFCAttr(
			testfc.WithCloseFCAmount(remainingValue),
			testfc.WithCloseFCTargetUnitID(initialBillID),
			testfc.WithCloseFCNonce(transferFCHash),
		),
		testtransaction.WithUnitId(fcrUnitID),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
	err = txSystem.Execute(closeFC)
	require.NoError(t, err)
	require.Equal(t, txFee, closeFC.Transaction.ServerMetadata.Fee)

	// verify user fee credit is closed (balance 0, unit will be deleted on round completion)
	fcrUnit, err = rmaTree.GetUnit(uint256.NewInt(0).SetBytes(fcrUnitID))
	require.NoError(t, err)
	fcrUnitData, ok = fcrUnit.Data.(*fc.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 0, fcrUnitData.Balance)

	// send reclaimFC
	closeFCPb := closeFC.Transaction
	closeFCProof := testblock.CreateProof(t, closeFC, signer, closeFCPb.UnitId)
	reclaimFC := testfc.NewReclaimFC(t, signer,
		testfc.NewReclaimFCAttr(t, signer,
			testfc.WithReclaimFCClosureTx(closeFCPb),
			testfc.WithReclaimFCClosureProof(closeFCProof),
			testfc.WithReclaimFCBacklink(transferFCHash),
		),
		testtransaction.WithUnitId(initialBillID),
		testtransaction.WithOwnerProof(script.PredicateArgumentEmpty()),
	)
	err = txSystem.Execute(reclaimFC)
	require.NoError(t, err)
	require.Equal(t, txFee, reclaimFC.Transaction.ServerMetadata.Fee)

	// verify reclaimed fee is added back to initial bill (original value minus 4x txfee)
	ib, err = rmaTree.GetUnit(initialBill.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, initialBill.Value-4*txFee, ib.Data.Value())
}

func unitIdFromTransaction(tx *billSplitWrapper) []byte {
	hasher := crypto.SHA256.New()
	idBytes := tx.UnitID().Bytes32()
	hasher.Write(idBytes[:])
	tx.addAttributesToHasher(hasher)
	hasher.Write(util.Uint64ToBytes(tx.Timeout()))
	return hasher.Sum(nil)
}

func getBill(t *testing.T, rmaTree *rma.Tree, billID *uint256.Int) (*rma.Unit, *BillData) {
	t.Helper()
	ib, err := rmaTree.GetUnit(billID)
	require.NoError(t, err)
	require.IsType(t, ib.Data, &BillData{})
	return ib, ib.Data.(*BillData)
}

func createBillTransfer(fromID *uint256.Int, value uint64, bearer []byte, backlink []byte) *txsystem.Transaction {
	tx := createTx(fromID)
	bt := &TransferAttributes{
		NewBearer: bearer,
		// #nosec G404
		TargetValue: value,
		Backlink:    backlink,
	}
	// #nosec G104
	tx.TransactionAttributes.MarshalFrom(bt)
	return tx
}

func createDCTransferAndSwapTxs(
	t *testing.T,
	ids []*uint256.Int, // bills to swap
	rmaTree *rma.Tree,
	signer abcrypto.Signer) ([]*txsystem.Transaction, *txsystem.Transaction) {
	t.Helper()

	// calculate new bill ID
	hasher := crypto.SHA256.New()
	idsByteArray := make([][]byte, len(ids))
	for i, id := range ids {
		bytes32 := id.Bytes32()
		hasher.Write(bytes32[:])
		idsByteArray[i] = bytes32[:]
	}
	newBillID := hasher.Sum(nil)

	// create dc transfers
	dcTransfers := make([]*txsystem.Transaction, len(ids))
	proofs := make([]*block.BlockProof, len(ids))

	var targetValue uint64 = 0
	for i, id := range ids {
		_, billData := getBill(t, rmaTree, id)
		// NB! dc transfer nonce must be equal to swap tx unit id
		targetValue += billData.V
		dcTransfers[i] = createDCTransfer(id, billData.V, billData.Backlink, newBillID, script.PredicateAlwaysTrue())
		tx, err := NewMoneyTx(systemIdentifier, dcTransfers[i])
		require.NoError(t, err)
		proofs[i] = testblock.CreateProof(t, tx, signer, util.Uint256ToBytes(id))
	}

	tx := &txsystem.Transaction{
		SystemId:              []byte{0, 0, 0, 0},
		UnitId:                newBillID,
		TransactionAttributes: &anypb.Any{},
		OwnerProof:            script.PredicateArgumentEmpty(),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout:           20,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(fcrID),
		},
	}

	bt := &SwapDCAttributes{
		OwnerCondition:  script.PredicateAlwaysTrue(),
		BillIdentifiers: idsByteArray,
		DcTransfers:     dcTransfers,
		Proofs:          proofs,
		TargetValue:     targetValue,
	}
	// #nosec G104
	tx.TransactionAttributes.MarshalFrom(bt)
	return dcTransfers, tx
}

func createDCTransfer(fromID *uint256.Int, targetValue uint64, backlink []byte, nonce []byte, targetBearer []byte) *txsystem.Transaction {
	tx := createTx(fromID)
	bt := &TransferDCAttributes{
		Nonce:        nonce,
		TargetBearer: targetBearer,
		TargetValue:  targetValue,
		Backlink:     backlink,
	}
	// #nosec G104
	tx.TransactionAttributes.MarshalFrom(bt)
	return tx
}

func createSplit(fromID *uint256.Int, amount uint64, remainingValue uint64, targetBearer, backlink []byte) *txsystem.Transaction {
	tx := createTx(fromID)
	bt := &SplitAttributes{
		Amount:         amount,
		TargetBearer:   targetBearer,
		RemainingValue: remainingValue,
		Backlink:       backlink,
	}
	// #nosec G104
	tx.TransactionAttributes.MarshalFrom(bt)
	return tx
}

func createTx(fromID *uint256.Int) *txsystem.Transaction {
	unitId32 := fromID.Bytes32()
	tx := &txsystem.Transaction{
		SystemId:              []byte{0, 0, 0, 0},
		UnitId:                unitId32[:],
		TransactionAttributes: &anypb.Any{},
		OwnerProof:            script.PredicateArgumentEmpty(),
		FeeProof:              script.PredicateArgumentEmpty(),
		ClientMetadata: &txsystem.ClientMetadata{
			Timeout:           20,
			MaxFee:            10,
			FeeCreditRecordId: util.Uint256ToBytes(fcrID),
		},
		// add server metadata so that tx.Hash method matches bill backlink
		ServerMetadata: &txsystem.ServerMetadata{
			Fee: fc.FixedFee(1)(),
		},
	}
	return tx
}

func createNonMoneyTx() *txsystem.Transaction {
	hasher := crypto.SHA256.New()
	hasher.Write(test.RandomBytes(32))
	id := hasher.Sum(nil)
	return &txsystem.Transaction{
		SystemId:       []byte{0, 0, 0, 1},
		UnitId:         id,
		ClientMetadata: &txsystem.ClientMetadata{Timeout: 2},
	}
}

func createRMATreeAndTxSystem(t *testing.T) (*rma.Tree, *txsystem.GenericTxSystem, abcrypto.Signer) {
	rmaTree := rma.NewWithSHA256()
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := map[string]abcrypto.Verifier{"test": verifier}

	mss, err := NewMoneyTxSystem(
		[]byte{0, 0, 0, 0},
		WithInitialBill(initialBill),
		WithSystemDescriptionRecords(createSDRs(2)),
		WithDCMoneyAmount(initialDustCollectorMoneyAmount),
		WithState(rmaTree),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)
	state, err := mss.State()
	require.NoError(t, err)
	require.NotNil(t, state.Summary())
	require.NotNil(t, state.Root())
	require.Len(t, state.Root(), crypto.SHA256.Size())
	require.False(t, rmaTree.ContainsUncommittedChanges())
	// add fee credit record with empty predicate
	fcrData := &fc.FeeCreditRecord{
		Balance: 100,
		Timeout: 100,
	}
	err = rmaTree.AtomicUpdate(fc.AddCredit(fcrID, script.PredicateAlwaysTrue(), fcrData, nil))
	require.NoError(t, err)
	_, err = mss.EndBlock()
	require.NoError(t, err)
	mss.Commit()

	return rmaTree, mss, signer
}

func createSDRs(id uint64) []*genesis.SystemDescriptionRecord {
	return []*genesis.SystemDescriptionRecord{{
		SystemIdentifier: systemID,
		T2Timeout:        2500,
		FeeCreditBill: &genesis.FeeCreditBill{
			UnitId:         util.Uint256ToBytes(uint256.NewInt(id)),
			OwnerPredicate: script.PredicateAlwaysTrue(),
		},
	}}
}
