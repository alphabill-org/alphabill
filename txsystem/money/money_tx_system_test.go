package money

import (
	"bytes"
	"crypto"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	testblock "github.com/alphabill-org/alphabill/internal/testutils/block"
	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	testtb "github.com/alphabill-org/alphabill/internal/testutils/trustbase"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

const initialDustCollectorMoneyAmount uint64 = 100

type InitialBill struct {
	ID    types.UnitID
	Value uint64
	Owner types.PredicateBytes
}

var (
	initialBill = &InitialBill{
		ID:    money.NewBillID(nil, test.RandomBytes(money.UnitPartLength)),
		Value: 110,
		Owner: templates.AlwaysTrueBytes(),
	}
	fcrAmount     = uint64(1e8)
	moneySystemID = money.DefaultSystemID
)

func TestNewTxSystem(t *testing.T) {
	var (
		sdrs        = createSDRs(newBillID(3))
		txsState    = genesisStateWithUC(t, initialBill, sdrs)
		_, verifier = testsig.CreateSignerAndVerifier(t)
		trustBase   = testtb.NewTrustBase(t, verifier)
	)
	txSystem, err := NewTxSystem(
		observability.Default(t),
		WithSystemIdentifier(moneySystemID),
		WithHashAlgorithm(crypto.SHA256),
		WithSystemDescriptionRecords(sdrs),
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

	d, err := txsState.GetUnit(DustCollectorMoneySupplyID, false)
	require.NoError(t, err)
	require.NotNil(t, d)

	require.Equal(t, initialDustCollectorMoneyAmount, d.Data().SummaryValueInput())
	require.Equal(t, DustCollectorPredicate, d.Bearer())
}

func TestNewTxSystem_RecoveredState(t *testing.T) {
	sdrs := createSDRs(newBillID(2))
	s := genesisStateWithUC(t, initialBill, sdrs)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	observe := observability.Default(t)

	originalTxs, err := NewTxSystem(
		observe,
		WithSystemIdentifier(systemIdentifier),
		WithSystemDescriptionRecords(sdrs),
		WithState(s),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)

	// Create a state with some units having multiple log entries - a prunable state
	require.NoError(t, originalTxs.BeginBlock(1))
	transFC := testutils.NewTransferFC(t, signer,
		testutils.NewTransferFCAttr(t, signer,
			testutils.WithCounter(0),
			testutils.WithAmount(20),
			testutils.WithTargetRecordID(money.NewFeeCreditRecordID(nil, []byte{100})),
		),
		testtransaction.WithUnitID(initialBill.ID),
		testtransaction.WithOwnerProof(nil),
	)
	_, err = originalTxs.Execute(transFC)
	require.NoError(t, err)
	originalSummaryRound1, err := originalTxs.EndBlock()
	require.NoError(t, err)

	// Commit and serialize the state
	require.NoError(t, originalTxs.Commit(createUC(originalSummaryRound1, 1)))
	buf := &bytes.Buffer{}
	require.NoError(t, originalTxs.State().Serialize(buf, true))

	// Create a recovered state and txSystem from the serialized state
	recoveredState, err := state.NewRecoveredState(buf, money.NewUnitData, state.WithHashAlgorithm(crypto.SHA256))
	require.NoError(t, err)
	recoveredTxs, err := NewTxSystem(
		observe,
		WithSystemIdentifier(systemIdentifier),
		WithSystemDescriptionRecords(sdrs),
		WithState(recoveredState),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)

	// Original and recovered summary hashes for round 1 must match
	recoveredSummaryRound1, err := recoveredTxs.StateSummary()
	require.NoError(t, err)
	require.EqualValues(t, originalSummaryRound1.Root(), recoveredSummaryRound1.Root())

	// Calculate the summary hash of a new empty round for the original txs
	require.NoError(t, originalTxs.BeginBlock(2))
	originalSummaryRound2, err := originalTxs.EndBlock()
	require.NoError(t, err)

	// Calculate the summary hash of a new empty round for the recovered txs
	require.NoError(t, recoveredTxs.BeginBlock(2))
	recoveredSummaryRound2, err := recoveredTxs.EndBlock()
	require.NoError(t, err)
	require.EqualValues(t, originalSummaryRound2.Root(), recoveredSummaryRound2.Root())

	// Since there was pruning, summary hashes of round 1 and round 2 cannot match
	require.NotEqualValues(t, originalSummaryRound1.Root(), originalSummaryRound2.Root())
}

func TestExecute_TransferOk(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	_, data := getBill(t, rmaTree, initialBill.ID)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()

	transferOk, _ := createBillTransfer(t, initialBill.ID, fcrID, initialBill.Value, templates.AlwaysFalseBytes(), 0)
	roundNumber := uint64(10)
	err := txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	serverMetadata, err := txSystem.Execute(transferOk)
	require.NoError(t, err)

	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, serverMetadata)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, 1)))

	unit2, data2 := getBill(t, rmaTree, initialBill.ID)
	require.Equal(t, data.SummaryValueInput(), data2.SummaryValueInput())
	require.NotEqual(t, transferOk.OwnerProof, unit2.Bearer())
	require.Equal(t, roundNumber, data2.T)
	require.EqualValues(t, 1, data2.Counter)
}

func TestExecute_Split2WayOk(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	totalValue, _, err := rmaTree.CalculateRoot()
	require.NoError(t, err)
	initBill, initBillData := getBill(t, rmaTree, initialBill.ID)
	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()
	splitOk, splitAttr := createSplit(t, initialBill.ID, fcrID, []*money.TargetUnit{{Amount: amount, OwnerCondition: templates.AlwaysTrueBytes()}}, remaining, initBillData.Counter)
	roundNumber := uint64(1)
	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	sm, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, sm)
	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	err = txSystem.Commit(createUC(stateSummary, 1))
	require.NoError(t, err)
	initBillAfterUpdate, initBillDataAfterUpdate := getBill(t, rmaTree, initialBill.ID)

	// bill value was reduced
	require.NotEqual(t, initBillData.V, initBillDataAfterUpdate.V)
	require.Equal(t, remaining, initBillDataAfterUpdate.V)

	// and bill was not locked
	require.EqualValues(t, 0, initBillDataAfterUpdate.Locked)

	// total value was not changed
	total, _, err := rmaTree.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, totalValue, total)

	// bearer of the initial bill was not changed
	require.Equal(t, initBill.Bearer(), initBillAfterUpdate.Bearer())
	require.Equal(t, roundNumber, initBillDataAfterUpdate.T)

	// counter was incremented
	require.Equal(t, initBillData.Counter+1, initBillDataAfterUpdate.Counter)

	expectedNewUnitID := money.NewBillID(nil, splitOk.HashForNewUnitID(crypto.SHA256, util.Uint32ToBytes(uint32(0))))
	newBill, bd := getBill(t, rmaTree, expectedNewUnitID)
	require.NotNil(t, newBill)
	require.NotNil(t, bd)
	require.Equal(t, amount, bd.V)
	require.EqualValues(t, 0, bd.Counter)
	require.Equal(t, types.PredicateBytes(splitAttr.TargetUnits[0].OwnerCondition), newBill.Bearer())
	require.Equal(t, roundNumber, bd.T)
	require.EqualValues(t, 0, bd.Locked)
}

func TestExecute_SplitNWayOk(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	totalValue, _, err := rmaTree.CalculateRoot()
	require.NoError(t, err)
	initBill, initBillData := getBill(t, rmaTree, initialBill.ID)
	remaining := initialBill.Value
	amount := uint64(10)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()

	var targetUnits []*money.TargetUnit
	for i := 0; i < 10; i++ {
		targetUnits = append(targetUnits, &money.TargetUnit{Amount: amount, OwnerCondition: templates.AlwaysTrueBytes()})
		remaining -= amount
	}
	splitOk, splitAttr := createSplit(t, initialBill.ID, fcrID, targetUnits, remaining, initBillData.Counter)
	roundNumber := uint64(1)
	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	sm, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, sm)
	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	err = txSystem.Commit(createUC(stateSummary, 1))
	require.NoError(t, err)
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

	// counter was incremented
	require.Equal(t, initBillData.Counter+1, initBillDataAfterUpdate.Counter)

	for i := range targetUnits {
		expectedNewUnitId := money.NewBillID(nil, splitOk.HashForNewUnitID(crypto.SHA256, util.Uint32ToBytes(uint32(i))))
		newBill, bd := getBill(t, rmaTree, expectedNewUnitId)
		require.NotNil(t, newBill)
		require.NotNil(t, bd)
		require.Equal(t, amount, bd.V)
		require.EqualValues(t, 0, bd.Counter)
		require.Equal(t, types.PredicateBytes(splitAttr.TargetUnits[0].OwnerCondition), newBill.Bearer())
		require.Equal(t, roundNumber, bd.T)
	}
}

func TestExecuteTransferDC_OK(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	_, initialBillData := getBill(t, rmaTree, initialBill.ID)
	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()
	splitOk, _ := createSplit(t, initialBill.ID, fcrID, []*money.TargetUnit{{Amount: amount, OwnerCondition: templates.AlwaysTrueBytes()}}, remaining, initialBillData.Counter)
	roundNumber := uint64(10)
	err := txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	sm, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, sm)
	billID := money.NewBillID(nil, splitOk.HashForNewUnitID(crypto.SHA256, util.Uint32ToBytes(uint32(0))))
	_, splitBillData := getBill(t, rmaTree, billID)

	transferDCOk, _ := createDCTransfer(t, billID, fcrID, splitBillData.V, splitBillData.Counter, test.RandomBytes(32), 0)
	require.NoError(t, err)

	sm, err = txSystem.Execute(transferDCOk)
	require.NoError(t, err)
	require.NotNil(t, sm)

	transferDCBill, transferDCBillData := getBill(t, rmaTree, billID)
	require.EqualValues(t, DustCollectorPredicate, transferDCBill.Bearer())
	require.EqualValues(t, 0, transferDCBillData.SummaryValueInput()) // dust transfer sets bill value to 0
	require.Equal(t, roundNumber, transferDCBillData.T)
	require.EqualValues(t, initialBillData.Counter+1, transferDCBillData.Counter)
}

func TestExecute_SwapOk(t *testing.T) {
	s, txSystem, signer := createStateAndTxSystem(t)
	_, initBillData := getBill(t, s, initialBill.ID)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()

	// create new bill with swap tx so that we have something to swap
	remaining := uint64(99)
	roundNumber := uint64(10)
	amount := initialBill.Value - remaining
	counter := initBillData.Counter
	splitOk, _ := createSplit(t, initialBill.ID, fcrID, []*money.TargetUnit{{Amount: amount, OwnerCondition: templates.AlwaysTrueBytes()}}, remaining, counter)

	err := txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)

	sm, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, sm)

	splitBillID := money.NewBillID(nil, splitOk.HashForNewUnitID(crypto.SHA256, util.Uint32ToBytes(uint32(0))))

	// execute lock transaction to verify swap unlocks locked unit
	counter += 1
	lockTx, _ := createLockTx(t, initialBill.ID, fcrID, counter)
	sm, err = txSystem.Execute(lockTx)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify bill got locked
	_, billData := getBill(t, s, initialBill.ID)
	require.EqualValues(t, 1, billData.Locked)

	// and counter was updated
	counter += 1
	require.Equal(t, counter, billData.Counter)

	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, []types.UnitID{splitBillID}, fcrID, initialBill.ID, counter, s, signer)
	for _, dcTransfer := range dcTransfers {
		sm, err = txSystem.Execute(dcTransfer.TransactionOrder)
		require.NoError(t, err)
		require.NotNil(t, sm)
	}

	// calculate dust bill value + dc money supply before commit
	_, dustBillData := getBill(t, s, splitBillID)
	_, dcBillData := getBill(t, s, DustCollectorMoneySupplyID)
	beforeCommitValue := dustBillData.V + dcBillData.V

	// verify DC money supply is correctly preserved at the end of round
	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, roundNumber)))

	// calculate dust bill value + dc money supply after commit
	_, dustBillData = getBill(t, s, splitBillID)
	_, dcBillData = getBill(t, s, DustCollectorMoneySupplyID)
	afterCommitValue := dustBillData.V + dcBillData.V
	require.Equal(t, beforeCommitValue, afterCommitValue)

	require.NoError(t, txSystem.BeginBlock(roundNumber+1))
	sm, err = txSystem.Execute(swapTx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	_, billData = getBill(t, s, swapTx.UnitID())
	require.Equal(t, initialBill.Value, billData.V) // initial bill value is the same after swap
	counter += 1
	require.EqualValues(t, counter, billData.Counter)
	require.EqualValues(t, 0, billData.Locked) // verify bill got unlocked

	_, dcBillData = getBill(t, s, DustCollectorMoneySupplyID)
	require.Equal(t, initialDustCollectorMoneyAmount, dcBillData.V) // dust collector money supply is the same after swap
	require.EqualValues(t, 2, dcBillData.Counter)                   // dust collector money supply counter is incremented

	// verify DC money supply is correctly preserved at the end of round
	beforeCommitValue = dcBillData.V
	stateSummary, err = txSystem.EndBlock()
	require.NoError(t, err)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, roundNumber)))

	require.NoError(t, txSystem.BeginBlock(roundNumber+2))
	dcBill, dcBillData := getBill(t, s, DustCollectorMoneySupplyID)
	require.Equal(t, beforeCommitValue, dcBillData.V)
	// Make sure the DC bill logs are pruned
	require.Equal(t, 1, len(dcBill.Logs()))
}

func TestExecute_LockAndUnlockOk(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()
	lockTx, _ := createLockTx(t, initialBill.ID, fcrID, 0)

	roundNumber := uint64(10)
	err := txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	sm, err := txSystem.Execute(lockTx)
	require.NoError(t, err)
	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.EqualValues(t, 1, sm.ActualFee)
	require.Len(t, sm.TargetUnits, 2)
	require.Equal(t, lockTx.UnitID(), sm.TargetUnits[0])
	require.NoError(t, txSystem.Commit(createUC(stateSummary, 1)))

	_, bd := getBill(t, rmaTree, initialBill.ID)
	require.EqualValues(t, 1, bd.Locked)                // bill is locked
	require.EqualValues(t, roundNumber, bd.T)           // round number updated
	require.EqualValues(t, 1, bd.Counter)               // counter updated
	require.EqualValues(t, 110, bd.SummaryValueInput()) // value not changed

	unlockTx, _ := createUnlockTx(t, initialBill.ID, fcrID, 1)
	roundNumber += 1
	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	sm, err = txSystem.Execute(unlockTx)
	require.NoError(t, err)
	stateSummary, err = txSystem.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, sm)
	require.EqualValues(t, 1, sm.ActualFee)
	require.Len(t, sm.TargetUnits, 2)
	require.Equal(t, unlockTx.UnitID(), sm.TargetUnits[0])
	require.NoError(t, txSystem.Commit(createUC(stateSummary, 1)))

	_, bd = getBill(t, rmaTree, initialBill.ID)
	require.EqualValues(t, 0, bd.Locked)                // bill is unlocked
	require.EqualValues(t, roundNumber, bd.T)           // round number updated
	require.EqualValues(t, 110, bd.SummaryValueInput()) // value not changed
	require.EqualValues(t, 2, bd.Counter)               // counter updated
}

func TestBillData_Value(t *testing.T) {
	bd := &money.BillData{
		V:       10,
		T:       0,
		Counter: 0,
	}

	actualSumValue := bd.SummaryValueInput()
	require.Equal(t, uint64(10), actualSumValue)
}

func TestBillData_AddToHasher(t *testing.T) {
	bd := &money.BillData{
		V:       10,
		T:       50,
		Counter: 0,
		Locked:  1,
	}

	hasher := crypto.SHA256.New()
	res, err := types.Cbor.Marshal(bd)
	require.NoError(t, err)
	hasher.Write(res)
	expectedHash := hasher.Sum(nil)
	hasher.Reset()
	require.NoError(t, bd.Write(hasher))
	actualHash := hasher.Sum(nil)
	require.Equal(t, expectedHash, actualHash)
	// make sure all fields where serialized
	var bdFormSerialized money.BillData
	require.NoError(t, types.Cbor.Unmarshal(res, &bdFormSerialized))
	require.Equal(t, bd, &bdFormSerialized)
}

func TestEndBlock_DustBillsAreRemoved(t *testing.T) {
	t.Skip("TODO AB-1133 implement dust bills deletion")
	rmaTree, txSystem, signer := createStateAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	remaining := initBillData.V
	var splitBillIDs = make([]types.UnitID, 10)
	counter := initBillData.Counter
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()
	for i := 0; i < 10; i++ {
		remaining--
		splitOk, _ := createSplit(t, initialBill.ID, fcrID, []*money.TargetUnit{{Amount: 1, OwnerCondition: templates.AlwaysTrueBytes()}}, remaining, counter)
		roundNumber := uint64(10)
		err := txSystem.BeginBlock(roundNumber)
		require.NoError(t, err)
		_, err = txSystem.Execute(splitOk)
		require.NoError(t, err)
		splitBillIDs[i] = money.NewBillID(splitOk.UnitID(), splitOk.HashForNewUnitID(crypto.SHA256, util.Uint32ToBytes(uint32(0))))

		_, data := getBill(t, rmaTree, initialBill.ID)
		counter = data.Counter
	}

	sort.Slice(splitBillIDs, func(i, j int) bool {
		return bytes.Compare(splitBillIDs[i], splitBillIDs[j]) == -1
	})

	// use initial bill as target bill
	targetBillID := initialBill.ID
	_, targetBillData := getBill(t, rmaTree, initialBill.ID)

	dcTransfers, swapTx := createDCTransferAndSwapTxs(t, splitBillIDs, targetBillID, fcrID, targetBillData.Counter, rmaTree, signer)

	for _, dcTransfer := range dcTransfers {
		_, err := txSystem.Execute(dcTransfer.TransactionOrder)
		require.NoError(t, err)
	}
	_, err := txSystem.Execute(swapTx)
	require.NoError(t, err)
	_, newBillData := getBill(t, rmaTree, swapTx.UnitID())
	require.Equal(t, uint64(10), newBillData.V)
	_, dustCollectorBill := getBill(t, rmaTree, DustCollectorMoneySupplyID)
	require.Equal(t, initialDustCollectorMoneyAmount, dustCollectorBill.V)
	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	err = txSystem.Commit(createUC(stateSummary, 999))
	require.NoError(t, err)

	err = txSystem.BeginBlock(defaultDustBillDeletionTimeout + 10)
	require.NoError(t, err)

	stateSummary, err = txSystem.EndBlock()
	require.NoError(t, err)

	err = txSystem.Commit(createUC(stateSummary, 1))
	require.NoError(t, err)

	_, dustCollectorBill = getBill(t, rmaTree, DustCollectorMoneySupplyID)
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
	err := txSystem.BeginBlock(0)
	require.NoError(t, err)
	transferFC := testutils.NewTransferFC(t, signer,
		testutils.NewTransferFCAttr(t, signer,
			testutils.WithCounter(0),
		),
		testtransaction.WithUnitID(initialBill.ID),
		testtransaction.WithOwnerProof(nil),
	)

	_, err = txSystem.Execute(transferFC)
	require.NoError(t, err)

	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, 2)))

	// verify that money fee credit bill is 50
	moneyFeeCreditBillID := money.NewBillID(nil, []byte{2})
	moneyFeeCreditBill, err := rmaTree.GetUnit(moneyFeeCreditBillID, false)

	require.NoError(t, err)
	require.EqualValues(t, 50, moneyFeeCreditBill.Data().SummaryValueInput())

	// process reclaimFC (with closeFC amount=50 and fee=1)
	err = txSystem.BeginBlock(0)
	require.NoError(t, err)

	closeFC := testutils.NewCloseFC(t, signer,
		testutils.NewCloseFCAttr(
			testutils.WithCloseFCAmount(50),
			testutils.WithCloseFCTargetUnitID(initialBill.ID),
			testutils.WithCloseFCTargetUnitCounter(1),
		),
	)
	closeFCRecord := &types.TransactionRecord{
		TransactionOrder: closeFC,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}

	proof := testblock.CreateProof(t, closeFCRecord, signer)
	reclaimFC := testutils.NewReclaimFC(t, signer,
		testutils.NewReclaimFCAttr(t, signer,
			testutils.WithReclaimFCClosureTx(closeFCRecord),
			testutils.WithReclaimFCClosureProof(proof),
			testutils.WithReclaimFCCounter(1),
		),
		testtransaction.WithUnitID(initialBill.ID),
		testtransaction.WithPayloadType(fcsdk.PayloadTypeReclaimFeeCredit),
		testtransaction.WithOwnerProof(nil),
	)
	_, err = txSystem.Execute(reclaimFC)
	require.NoError(t, err)
	stateSummary, err = txSystem.EndBlock()
	require.NoError(t, err)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, 3)))

	// verify that moneyFCB=50-50+1+1=2 (moneyFCB - closeAmount + closeFee + reclaimFee)
	moneyFeeCreditBill, err = rmaTree.GetUnit(moneyFeeCreditBillID, false)
	require.NoError(t, err)
	require.EqualValues(t, 2, moneyFeeCreditBill.Data().SummaryValueInput())
}

func TestRegisterData_RevertSplit(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()

	vdState, err := txSystem.StateSummary()
	require.NoError(t, err)

	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	splitOk, _ := createSplit(t, initialBill.ID, fcrID, []*money.TargetUnit{{Amount: amount, OwnerCondition: templates.AlwaysTrueBytes()}}, remaining, initBillData.Counter)
	require.NoError(t, err)
	roundNumber := uint64(10)
	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
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
	u, initBillData := getBill(t, rmaTree, initialBill.ID)
	unitBearer := bytes.Clone(u.Bearer())
	vdState, err := txSystem.StateSummary()
	require.NoError(t, err)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()

	transDC, _ := createDCTransfer(t, initialBill.ID, fcrID, initialBill.Value, initBillData.Counter, test.RandomBytes(32), 0)
	require.NoError(t, err)
	roundNumber := uint64(10)
	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	sm, err := txSystem.Execute(transDC)
	require.NoError(t, err)
	require.NotNil(t, sm)
	_, err = txSystem.StateSummary()
	require.ErrorIs(t, err, txsystem.ErrStateContainsUncommittedChanges)
	u, bd := getBill(t, rmaTree, initialBill.ID)
	require.EqualValues(t, DustCollectorPredicate, u.Bearer())
	require.EqualValues(t, 1, bd.Counter)

	txSystem.Revert()
	s, err := txSystem.StateSummary()
	require.NoError(t, err)
	require.Equal(t, vdState, s)
	u, bd = getBill(t, rmaTree, initialBill.ID)
	require.EqualValues(t, unitBearer, u.Bearer())
	require.NotEqualValues(t, DustCollectorPredicate, u.Bearer())
	require.EqualValues(t, 0, bd.Counter)
}

// Test Transfer->Add->Lock->Close->Reclaim sequence OK
func TestExecute_FeeCreditSequence_OK(t *testing.T) {
	rmaTree, txSystem, signer := createStateAndTxSystem(t)
	txFee := fc.FixedFee(1)()
	initialBillID := initialBill.ID
	txAmount := uint64(20)
	fcrID := testutils.NewFeeCreditRecordID(t, signer)

	err := txSystem.BeginBlock(1)
	require.NoError(t, err)

	// transfer 20 Tema to FCB
	transferFC := testutils.NewTransferFC(t, signer,
		testutils.NewTransferFCAttr(t, signer,
			testutils.WithCounter(0),
			testutils.WithAmount(txAmount),
			testutils.WithTargetRecordID(fcrID),
		),
		testtransaction.WithUnitID(initialBillID),
		testtransaction.WithOwnerProof(nil),
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
	addFC := testutils.NewAddFC(t, signer,
		testutils.NewAddFCAttr(t, signer,
			testutils.WithTransferFCRecord(transferFCTransactionRecord),
			testutils.WithTransferFCProof(transferFCProof),
		),
		testtransaction.WithUnitID(fcrID),
	)
	addFC.OwnerProof = signTx(t, signer, addFC)
	sm, err = txSystem.Execute(addFC)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify user fee credit is 18 (transfer 20 minus fee 2 * fee)
	remainingValue := txAmount - (2 * uint64(txFee)) // 18
	fcrUnit, err := rmaTree.GetUnit(fcrID, false)
	require.NoError(t, err)
	fcrUnitData, ok := fcrUnit.Data().(*fcsdk.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, remainingValue, fcrUnitData.Balance)
	require.Equal(t, txFee, sm.ActualFee)

	// lock target unit
	lockTx, _ := createLockTx(t, initialBillID, fcrID, 1)
	lockTx.OwnerProof = nil
	lockTx.FeeProof = signTx(t, signer, lockTx)
	sm, err = txSystem.Execute(lockTx)
	require.NoError(t, err)
	require.NotNil(t, sm)
	remainingValue -= 1

	// verify target got locked
	ib, err = rmaTree.GetUnit(initialBill.ID, false)
	require.NoError(t, err)
	bd, ok := ib.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, 1, bd.Locked)

	// verify counter was incremented
	require.EqualValues(t, 2, bd.Counter)

	// send closeFC
	closeFC := testutils.NewCloseFC(t, signer,
		testutils.NewCloseFCAttr(
			testutils.WithCloseFCAmount(remainingValue),
			testutils.WithCloseFCTargetUnitID(initialBillID),
			testutils.WithCloseFCTargetUnitCounter(2),
		),
		testtransaction.WithUnitID(fcrID),
		testtransaction.WithOwnerProof(nil),
	)
	sm, err = txSystem.Execute(closeFC)
	require.NoError(t, err)
	require.Equal(t, txFee, sm.ActualFee)

	// verify user fee credit is closed (balance 0, unit will be deleted on round completion)
	fcrUnit, err = rmaTree.GetUnit(fcrID, false)
	require.NoError(t, err)
	fcrUnitData, ok = fcrUnit.Data().(*fcsdk.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 0, fcrUnitData.Balance)

	// send reclaimFC
	closeFCTransactionRecord := &types.TransactionRecord{
		TransactionOrder: closeFC,
		ServerMetadata:   sm,
	}
	closeFCProof := testblock.CreateProof(t, closeFCTransactionRecord, signer)
	reclaimFC := testutils.NewReclaimFC(t, signer,
		testutils.NewReclaimFCAttr(t, signer,
			testutils.WithReclaimFCClosureTx(closeFCTransactionRecord),
			testutils.WithReclaimFCClosureProof(closeFCProof),
			testutils.WithReclaimFCCounter(2),
		),
		testtransaction.WithUnitID(initialBillID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(fcsdk.PayloadTypeReclaimFeeCredit),
	)
	sm, err = txSystem.Execute(reclaimFC)
	require.NoError(t, err)
	require.Equal(t, txFee, sm.ActualFee)

	// verify reclaimed fee is added back to initial bill (original value minus 5x txfee)
	ib, err = rmaTree.GetUnit(initialBill.ID, false)
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, initialBill.Value-5*uint64(txFee), ib.Data().SummaryValueInput())

	// and initial bill got unlocked
	bd, ok = ib.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, 0, bd.Locked)

	// and counter was incremented
	require.EqualValues(t, 3, bd.Counter)
}

// Test LockFC -> UnlockFC
func TestExecute_AddFeeCreditWithLocking_OK(t *testing.T) {
	rmaTree, txSystem, _ := createStateAndTxSystem(t)
	err := txSystem.BeginBlock(1)
	require.NoError(t, err)
	signer, _ := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()

	// lock fee credit record
	lockFCAttr := testutils.NewLockFCAttr(testutils.WithLockFCCounter(0))
	lockFC := testutils.NewLockFC(t, signer, lockFCAttr,
		testtransaction.WithUnitID(fcrID),
		testtransaction.WithOwnerProof(nil),
	)
	sm, err := txSystem.Execute(lockFC)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify unit was locked
	u, err := rmaTree.GetUnit(fcrID, false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*fcsdk.FeeCreditRecord)
	require.True(t, ok)
	require.True(t, fcr.IsLocked())

	// unlock fee credit record
	unlockFCAttr := testutils.NewUnlockFCAttr(testutils.WithUnlockFCCounter(1))
	unlockFC := testutils.NewUnlockFC(t, signer, unlockFCAttr, testtransaction.WithUnitID(fcrID))
	sm, err = txSystem.Execute(unlockFC)
	require.NoError(t, err)
	require.NotNil(t, sm)

	// verify unit was unlocked
	fcrUnit, err := rmaTree.GetUnit(fcrID, false)
	require.NoError(t, err)
	fcr, ok = fcrUnit.Data().(*fcsdk.FeeCreditRecord)
	require.True(t, ok)
	require.False(t, fcr.IsLocked())
}

func getBill(t *testing.T, s *state.State, billID types.UnitID) (*state.Unit, *money.BillData) {
	t.Helper()
	ib, err := s.GetUnit(billID, false)
	require.NoError(t, err)
	require.IsType(t, &money.BillData{}, ib.Data())
	return ib, ib.Data().(*money.BillData)
}

func createBillTransfer(t *testing.T, fromID types.UnitID, fcrID types.UnitID, value uint64, bearer []byte, counter uint64) (*types.TransactionOrder, *money.TransferAttributes) {
	tx := createTx(fromID, fcrID, money.PayloadTypeTransfer)
	attr := &money.TransferAttributes{
		NewBearer: bearer,
		// #nosec G404
		TargetValue: value,
		Counter:     counter,
	}
	rawBytes, err := types.Cbor.Marshal(attr)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return tx, attr
}

func createLockTx(t *testing.T, fromID types.UnitID, fcrID types.UnitID, counter uint64) (*types.TransactionOrder, *money.LockAttributes) {
	tx := createTx(fromID, fcrID, money.PayloadTypeLock)
	lockTxAttr := &money.LockAttributes{
		LockStatus: 1,
		Counter:    counter,
	}
	rawBytes, err := types.Cbor.Marshal(lockTxAttr)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return tx, lockTxAttr
}

func createUnlockTx(t *testing.T, fromID types.UnitID, fcrID types.UnitID, counter uint64) (*types.TransactionOrder, *money.UnlockAttributes) {
	tx := createTx(fromID, fcrID, money.PayloadTypeUnlock)
	unlockTxAttr := &money.UnlockAttributes{
		Counter: counter,
	}
	rawBytes, err := types.Cbor.Marshal(unlockTxAttr)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return tx, unlockTxAttr
}

func createDCTransferAndSwapTxs(
	t *testing.T,
	ids []types.UnitID, // bills to swap
	fcrID types.UnitID,
	targetID []byte,
	targetCounter uint64,
	rmaTree *state.State,
	signer abcrypto.Signer) ([]*types.TransactionRecord, *types.TransactionOrder) {

	t.Helper()

	// create dc transfers
	dcTransfers := make([]*types.TransactionRecord, len(ids))
	proofs := make([]*types.TxProof, len(ids))

	var targetValue uint64 = 0
	for i, id := range ids {
		_, billData := getBill(t, rmaTree, id)
		targetValue += billData.V
		tx, _ := createDCTransfer(t, id, fcrID, billData.V, billData.Counter, targetID, targetCounter)
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
			Type:     money.PayloadTypeSwapDC,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           20,
				MaxTransactionFee: 10,
				FeeCreditRecordID: fcrID,
			},
		},
		OwnerProof: nil,
	}

	bt := &money.SwapDCAttributes{
		OwnerCondition:   templates.AlwaysTrueBytes(),
		DcTransfers:      dcTransfers,
		DcTransferProofs: proofs,
		TargetValue:      targetValue,
	}
	rawBytes, err := types.Cbor.Marshal(bt)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return dcTransfers, tx
}

func createDCTransfer(t *testing.T, fromID types.UnitID, fcrID types.UnitID, val uint64, counter uint64, targetID []byte, targetCounter uint64) (*types.TransactionOrder, *money.TransferDCAttributes) {
	tx := createTx(fromID, fcrID, money.PayloadTypeTransDC)
	bt := &money.TransferDCAttributes{
		Value:             val,
		TargetUnitID:      targetID,
		TargetUnitCounter: targetCounter,
		Counter:           counter,
	}
	rawBytes, err := types.Cbor.Marshal(bt)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return tx, bt
}

func createSplit(t *testing.T, fromID types.UnitID, fcrID types.UnitID, targetUnits []*money.TargetUnit, remainingValue uint64, counter uint64) (*types.TransactionOrder, *money.SplitAttributes) {
	tx := createTx(fromID, fcrID, money.PayloadTypeSplit)
	bt := &money.SplitAttributes{
		TargetUnits:    targetUnits,
		RemainingValue: remainingValue,
		Counter:        counter,
	}
	rawBytes, err := types.Cbor.Marshal(bt)
	require.NoError(t, err)
	tx.Payload.Attributes = rawBytes
	return tx, bt
}

func createTx(fromID types.UnitID, fcrID types.UnitID, payloadType string) *types.TransactionOrder {
	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   moneySystemID,
			UnitID:     fromID,
			Type:       payloadType,
			Attributes: nil,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           20,
				MaxTransactionFee: 10,
				FeeCreditRecordID: fcrID,
			},
		},
		OwnerProof: templates.EmptyArgument(), // transfers "always true" unit by default
		FeeProof:   nil,
	}
	return tx
}

func createStateAndTxSystem(t *testing.T) (*state.State, *txsystem.GenericTxSystem, abcrypto.Signer) {
	sdrs := createSDRs(newBillID(2))
	s := genesisStateWithUC(t, initialBill, sdrs)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()

	mss, err := NewTxSystem(
		observability.Default(t),
		WithSystemIdentifier(systemIdentifier),
		WithSystemDescriptionRecords(sdrs),
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
	fcrData := &fcsdk.FeeCreditRecord{
		Balance: 100,
		Timeout: 100,
	}
	err = s.Apply(unit.AddCredit(fcrID, templates.AlwaysTrueBytes(), fcrData))
	require.NoError(t, err)
	summary, err = mss.EndBlock()
	require.NoError(t, err)
	err = mss.Commit(createUC(summary, 2))
	require.NoError(t, err)

	return s, mss, signer
}

// Duplicates txsystem/money/testutils/genesis_state.go
func genesisState(t *testing.T, initialBill *InitialBill, sdrs []*types.SystemDescriptionRecord) *state.State {
	s := state.NewEmptyState()
	zeroHash := make([]byte, crypto.SHA256.Size())

	// initial bill
	require.NoError(t, s.Apply(state.AddUnit(initialBill.ID, initialBill.Owner, &money.BillData{V: initialBill.Value})))
	require.NoError(t, s.AddUnitLog(initialBill.ID, zeroHash))

	// dust collector money supply
	require.NoError(t, s.Apply(state.AddUnit(DustCollectorMoneySupplyID, DustCollectorPredicate, &money.BillData{V: initialDustCollectorMoneyAmount})))
	require.NoError(t, s.AddUnitLog(DustCollectorMoneySupplyID, zeroHash))

	// fee credit bills
	for _, sdr := range sdrs {
		fcb := sdr.FeeCreditBill
		require.NoError(t, s.Apply(state.AddUnit(fcb.UnitID, fcb.OwnerPredicate, &money.BillData{})))
		require.NoError(t, s.AddUnitLog(fcb.UnitID, zeroHash))
	}

	_, _, err := s.CalculateRoot()
	require.NoError(t, err)

	return s
}

func genesisStateWithUC(t *testing.T, initialBill *InitialBill, sdrs []*types.SystemDescriptionRecord) *state.State {
	s := genesisState(t, initialBill, sdrs)
	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(&types.UnicityCertificate{InputRecord: &types.InputRecord{
		RoundNumber:  1,
		Hash:         summaryHash,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}}))
	return s
}

func createSDRs(fcbID types.UnitID) []*types.SystemDescriptionRecord {
	return []*types.SystemDescriptionRecord{{
		SystemIdentifier: moneySystemID,
		T2Timeout:        2500,
		FeeCreditBill: &types.FeeCreditBill{
			UnitID:         fcbID,
			OwnerPredicate: templates.AlwaysTrueBytes(),
		},
	}}
}

func createUC(s txsystem.StateSummary, roundNumber uint64) *types.UnicityCertificate {
	return &types.UnicityCertificate{InputRecord: &types.InputRecord{
		RoundNumber:  roundNumber,
		Hash:         s.Root(),
		SummaryValue: s.Summary(),
	}}
}

type moneyModuleOption func(m *Module) error

func withStateUnit(unitID []byte, bearer types.PredicateBytes, data types.UnitData) moneyModuleOption {
	return func(m *Module) error {
		return m.state.Apply(state.AddUnit(unitID, bearer, data))
	}
}

func newTestMoneyModule(t *testing.T, verifier abcrypto.Verifier, opts ...moneyModuleOption) *Module {
	module := defaultMoneyModule(t, verifier)
	for _, opt := range opts {
		require.NoError(t, opt(module))
	}
	return module
}

func defaultMoneyModule(t *testing.T, verifier abcrypto.Verifier) *Module {
	// NB! using the same pubkey for trust base and unit bearer! TODO: use different keys...
	options, err := defaultOptions()
	require.NoError(t, err)
	options.trustBase = testtb.NewTrustBase(t, verifier)
	options.state = state.NewEmptyState()
	module, err := NewMoneyModule(options)
	require.NoError(t, err)
	return module
}

func signTx(t *testing.T, signer abcrypto.Signer, txo *types.TransactionOrder) []byte {
	payloadBytes, err := txo.PayloadBytes()
	require.NoError(t, err)
	signedBytes, err := signer.SignBytes(payloadBytes)
	require.NoError(t, err)
	verifier, err := signer.Verifier()
	require.NoError(t, err)
	publicKey, err := verifier.MarshalPublicKey()
	require.NoError(t, err)
	return templates.NewP2pkh256SignatureBytes(signedBytes, publicKey)
}
