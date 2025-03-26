package money

import (
	"bytes"
	"crypto"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	moneyid "github.com/alphabill-org/alphabill-go-base/testutils/money"
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
		ID:    append(make([]byte, 25), 5, 6, 1, 9, 0, 7, 8, money.BillUnitType),
		Value: 110,
		Owner: templates.AlwaysTrueBytes(),
	}
	moneyPartitionID = money.DefaultPartitionID
	networkID        = types.NetworkID(3)
)

func TestNewTxSystem(t *testing.T) {
	var (
		sdrs        = createPDRs(t)
		txsState    = genesisStateWithUC(t, initialBill, sdrs)
		_, verifier = testsig.CreateSignerAndVerifier(t)
		trustBase   = testtb.NewTrustBase(t, verifier)
	)
	txSystem, err := NewTxSystem(
		*sdrs[0],
		types.ShardID{},
		observability.Default(t),
		WithHashAlgorithm(crypto.SHA256),
		WithState(txsState),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)
	require.NotNil(t, txSystem)

	u, d := getBill(t, txsState, initialBill.ID)
	require.NotNil(t, u)
	require.NotNil(t, d)
	require.Equal(t, initialBill.Value, d.SummaryValueInput())
	require.EqualValues(t, initialBill.Owner, d.OwnerPredicate)

	u, d = getBill(t, txsState, DustCollectorMoneySupplyID)
	require.NotNil(t, u)
	require.NotNil(t, d)
	require.Equal(t, initialDustCollectorMoneyAmount, d.SummaryValueInput())
	require.EqualValues(t, DustCollectorPredicate, d.OwnerPredicate)
}

func TestNewTxSystem_RecoveredState(t *testing.T) {
	sdrs := createPDRs(t)
	s := genesisStateWithUC(t, initialBill, sdrs)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	observe := observability.Default(t)

	originalTxs, err := NewTxSystem(
		*sdrs[0],
		types.ShardID{},
		observe,
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
			testutils.WithTargetRecordID(moneyid.NewFeeCreditRecordID(t)),
		),
		testtransaction.WithUnitID(initialBill.ID),
		testtransaction.WithPartition(sdrs[0]),
	)
	txr, err := originalTxs.Execute(transFC)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{transFC.UnitID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	originalSummaryRound1, err := originalTxs.EndBlock()
	require.NoError(t, err)

	// Commit and serialize the state
	require.NoError(t, originalTxs.Commit(createUC(originalSummaryRound1, 1)))
	buf := &bytes.Buffer{}
	require.NoError(t, originalTxs.State().Serialize(buf, true))

	// Create a recovered state and txSystem from the serialized state
	recoveredState, err := state.NewRecoveredState(buf, func(ui types.UnitID) (types.UnitData, error) { return money.NewUnitData(ui, sdrs[0]) }, state.WithHashAlgorithm(crypto.SHA256))
	require.NoError(t, err)
	recoveredTxs, err := NewTxSystem(
		*sdrs[0],
		types.ShardID{},
		observe,
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
	pdrs := createPDRs(t)
	rmaTree, txSystem, _ := createStateAndTxSystem(t, pdrs)
	_, data := getBill(t, rmaTree, initialBill.ID)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue(t)

	transferOk, _, _ := createBillTransfer(t, initialBill.ID, fcrID, initialBill.Value, templates.AlwaysFalseBytes(), 0)
	transferOk.NetworkID = pdrs[0].NetworkID
	roundNumber := uint64(10)
	err := txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	serverMetadata, err := txSystem.Execute(transferOk)
	require.NoError(t, err)
	require.NotNil(t, serverMetadata)
	require.Equal(t, types.TxStatusSuccessful, serverMetadata.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{transferOk.UnitID, fcrID}, serverMetadata.TargetUnits())
	require.True(t, serverMetadata.ServerMetadata.ActualFee > 0)

	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, serverMetadata)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, 1)))

	_, data2 := getBill(t, rmaTree, initialBill.ID)
	require.Equal(t, data.SummaryValueInput(), data2.SummaryValueInput())
	require.EqualValues(t, 1, data2.Counter)
}

func TestExecute_Split2WayOk(t *testing.T) {
	pdrs := createPDRs(t)
	rmaTree, txSystem, _ := createStateAndTxSystem(t, pdrs)
	totalValue, _, err := rmaTree.CalculateRoot()
	require.NoError(t, err)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue(t)
	splitOk, splitAttr, _ := createSplit(t, initialBill.ID, fcrID, []*money.TargetUnit{{Amount: amount, OwnerPredicate: templates.AlwaysTrueBytes()}}, initBillData.Counter)
	splitOk.NetworkID = pdrs[0].NetworkID
	roundNumber := uint64(1)
	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	txr, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)

	expectedNewUnitID, err := pdrs[0].ComposeUnitID(types.ShardID{}, money.BillUnitType, money.PrndSh(splitOk))
	require.NoError(t, err)
	require.Equal(t, []types.UnitID{splitOk.UnitID, expectedNewUnitID, fcrID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	err = txSystem.Commit(createUC(stateSummary, 1))
	require.NoError(t, err)
	_, initBillDataAfterUpdate := getBill(t, rmaTree, initialBill.ID)

	// bill value was reduced
	require.NotEqual(t, initBillData.Value, initBillDataAfterUpdate.Value)
	require.Equal(t, remaining, initBillDataAfterUpdate.Value)

	// and bill was not locked
	require.EqualValues(t, 0, initBillDataAfterUpdate.Locked)

	// total value was not changed
	total, _, err := rmaTree.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, totalValue, total)

	// bearer of the initial bill was not changed
	require.Equal(t, initBillData.OwnerPredicate, initBillDataAfterUpdate.OwnerPredicate)

	// counter was incremented
	require.Equal(t, initBillData.Counter+1, initBillDataAfterUpdate.Counter)

	newBill, newBillData := getBill(t, rmaTree, expectedNewUnitID)
	require.NotNil(t, newBill)
	require.NotNil(t, newBillData)
	require.Equal(t, amount, newBillData.Value)
	require.EqualValues(t, 0, newBillData.Counter)
	require.EqualValues(t, splitAttr.TargetUnits[0].OwnerPredicate, newBillData.OwnerPredicate)
	require.EqualValues(t, 0, newBillData.Locked)
}

func TestExecute_SplitNWayOk(t *testing.T) {
	pdrs := createPDRs(t)
	rmaTree, txSystem, _ := createStateAndTxSystem(t, pdrs)
	totalValue, _, err := rmaTree.CalculateRoot()
	require.NoError(t, err)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	remaining := initialBill.Value
	amount := uint64(10)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue(t)

	var targetUnits []*money.TargetUnit
	for i := 0; i < 10; i++ {
		targetUnits = append(targetUnits, &money.TargetUnit{Amount: amount, OwnerPredicate: templates.AlwaysTrueBytes()})
		remaining -= amount
	}
	splitOk, splitAttr, _ := createSplit(t, initialBill.ID, fcrID, targetUnits, initBillData.Counter)
	splitOk.NetworkID = pdrs[0].NetworkID
	roundNumber := uint64(1)
	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	txr, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Len(t, txr.TargetUnits(), 1+10+1)              // target, new bills and fcr
	require.Contains(t, txr.TargetUnits(), splitOk.UnitID) // target id
	require.Contains(t, txr.TargetUnits(), fcrID)          // fee credit id
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	err = txSystem.Commit(createUC(stateSummary, 1))
	require.NoError(t, err)
	_, initBillDataAfterUpdate := getBill(t, rmaTree, initialBill.ID)

	// bill value was reduced
	require.NotEqual(t, initBillData.Value, initBillDataAfterUpdate.Value)
	require.Equal(t, remaining, initBillDataAfterUpdate.Value)

	// total value was not changed
	total, _, err := rmaTree.CalculateRoot()
	require.NoError(t, err)
	require.Equal(t, totalValue, total)

	// bearer of the initial bill was not changed
	require.Equal(t, initBillData.OwnerPredicate, initBillDataAfterUpdate.OwnerPredicate)

	// counter was incremented
	require.Equal(t, initBillData.Counter+1, initBillDataAfterUpdate.Counter)

	idGen := money.PrndSh(splitOk)
	for i := 0; i < len(targetUnits); i++ {
		expectedNewUnitId, err := pdrs[0].ComposeUnitID(types.ShardID{}, money.BillUnitType, idGen)
		require.NoError(t, err)
		require.Contains(t, txr.TargetUnits(), expectedNewUnitId) // target, new bills and fcr
		newBill, newBillData := getBill(t, rmaTree, expectedNewUnitId)
		require.NotNil(t, newBill)
		require.NotNil(t, newBillData)
		require.Equal(t, amount, newBillData.Value)
		require.EqualValues(t, 0, newBillData.Counter)
		require.EqualValues(t, splitAttr.TargetUnits[0].OwnerPredicate, newBillData.OwnerPredicate)
	}
}

func TestExecuteTransferDC_OK(t *testing.T) {
	pdrs := createPDRs(t)
	rmaTree, txSystem, _ := createStateAndTxSystem(t, pdrs)
	_, initialBillData := getBill(t, rmaTree, initialBill.ID)
	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue(t)
	splitOk, _, _ := createSplit(t, initialBill.ID, fcrID, []*money.TargetUnit{{Amount: amount, OwnerPredicate: templates.AlwaysTrueBytes()}}, initialBillData.Counter)
	splitOk.NetworkID = pdrs[0].NetworkID
	roundNumber := uint64(10)
	err := txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	txr, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	billID, err := pdrs[0].ComposeUnitID(types.ShardID{}, money.BillUnitType, money.PrndSh(splitOk))
	require.NoError(t, err)
	_, splitBillData := getBill(t, rmaTree, billID)

	transferDCOk, _, _ := createDCTransfer(t, billID, fcrID, splitBillData.Value, splitBillData.Counter, test.RandomBytes(32), 0)
	transferDCOk.NetworkID = pdrs[0].NetworkID
	require.NoError(t, err)

	txr, err = txSystem.Execute(transferDCOk)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{transferDCOk.UnitID, DustCollectorMoneySupplyID, fcrID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	_, transferDCBillData := getBill(t, rmaTree, billID)
	require.EqualValues(t, DustCollectorPredicate, transferDCBillData.OwnerPredicate)
	require.EqualValues(t, 0, transferDCBillData.SummaryValueInput()) // dust transfer sets bill value to 0
	require.EqualValues(t, initialBillData.Counter+1, transferDCBillData.Counter)
}

func TestExecute_SwapOk(t *testing.T) {
	pdrs := createPDRs(t)
	s, txSystem, signer := createStateAndTxSystem(t, pdrs)
	_, initBillData := getBill(t, s, initialBill.ID)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue(t)

	// create new bill with swap tx so that we have something to swap
	remaining := uint64(99)
	roundNumber := uint64(10)
	amount := initialBill.Value - remaining
	counter := initBillData.Counter
	splitOk, _, _ := createSplit(t, initialBill.ID, fcrID, []*money.TargetUnit{{Amount: amount, OwnerPredicate: templates.AlwaysTrueBytes()}}, counter)
	splitOk.NetworkID = pdrs[0].NetworkID

	err := txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	splitBillID, err := pdrs[0].ComposeUnitID(types.ShardID{}, money.BillUnitType, money.PrndSh(splitOk))
	require.NoError(t, err)
	txr, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{splitOk.UnitID, splitBillID, fcrID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	// execute lock transaction to verify swap unlocks locked unit
	counter += 1
	lockTx, _, _ := createLockTx(t, initialBill.ID, fcrID, counter)
	lockTx.NetworkID = pdrs[0].NetworkID
	txr, err = txSystem.Execute(lockTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{lockTx.UnitID, fcrID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	// verify bill got locked
	_, billData := getBill(t, s, initialBill.ID)
	require.EqualValues(t, 1, billData.Locked)

	// and counter was updated
	counter += 1
	require.Equal(t, counter, billData.Counter)

	dcTransferProofs, swapTx := createDCTransferAndSwapTxs(t, []types.UnitID{splitBillID}, fcrID, initialBill.ID, counter, s, signer)
	for _, dcTransferProof := range dcTransferProofs {
		tx, err := dcTransferProof.GetTransactionOrderV1()
		require.NoError(t, err)
		txr, err = txSystem.Execute(tx)
		require.NoError(t, err)
		require.NotNil(t, txr)
		require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
		require.Equal(t, []types.UnitID{tx.UnitID, DustCollectorMoneySupplyID, fcrID}, txr.TargetUnits())
		require.True(t, txr.ServerMetadata.ActualFee > 0)
	}

	// calculate dust bill value + dc money supply before commit
	_, dustBillData := getBill(t, s, splitBillID)
	_, dcBillData := getBill(t, s, DustCollectorMoneySupplyID)
	beforeCommitValue := dustBillData.Value + dcBillData.Value

	// verify DC money supply is correctly preserved at the end of round
	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, roundNumber)))

	// calculate dust bill value + dc money supply after commit
	_, dustBillData = getBill(t, s, splitBillID)
	_, dcBillData = getBill(t, s, DustCollectorMoneySupplyID)
	afterCommitValue := dustBillData.Value + dcBillData.Value
	require.Equal(t, beforeCommitValue, afterCommitValue)

	require.NoError(t, txSystem.BeginBlock(roundNumber+1))
	txr, err = txSystem.Execute(swapTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.EqualValues(t, []types.UnitID{swapTx.UnitID, DustCollectorMoneySupplyID, fcrID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	_, billData = getBill(t, s, swapTx.UnitID)
	require.Equal(t, initialBill.Value, billData.Value) // initial bill value is the same after swap
	counter += 1
	require.EqualValues(t, counter, billData.Counter)
	require.EqualValues(t, 0, billData.Locked) // verify bill got unlocked

	_, dcBillData = getBill(t, s, DustCollectorMoneySupplyID)
	require.Equal(t, initialDustCollectorMoneyAmount, dcBillData.Value) // dust collector money supply is the same after swap

	// verify DC money supply is correctly preserved at the end of round
	beforeCommitValue = dcBillData.Value
	stateSummary, err = txSystem.EndBlock()
	require.NoError(t, err)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, roundNumber)))

	require.NoError(t, txSystem.BeginBlock(roundNumber+2))
	dcBill, dcBillData := getBill(t, s, DustCollectorMoneySupplyID)
	require.Equal(t, beforeCommitValue, dcBillData.Value)
	// Make sure the DC bill logs are pruned
	u, err := state.ToUnitV1(dcBill)
	require.NoError(t, err)
	require.Equal(t, 1, len(u.Logs()))
}

func TestExecute_LockAndUnlockOk(t *testing.T) {
	pdrs := createPDRs(t)
	rmaTree, txSystem, _ := createStateAndTxSystem(t, pdrs)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue(t)
	lockTx, _, _ := createLockTx(t, initialBill.ID, fcrID, 0)

	roundNumber := uint64(10)
	err := txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	txr, err := txSystem.Execute(lockTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{lockTx.UnitID, fcrID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.Len(t, txr.TargetUnits(), 2)
	require.Equal(t, lockTx.UnitID, txr.ServerMetadata.TargetUnits[0])
	require.NoError(t, txSystem.Commit(createUC(stateSummary, 1)))

	_, bd := getBill(t, rmaTree, initialBill.ID)
	require.EqualValues(t, 1, bd.Locked)                // bill is locked
	require.EqualValues(t, 1, bd.Counter)               // counter updated
	require.EqualValues(t, 110, bd.SummaryValueInput()) // value not changed

	unlockTx, _, _ := createUnlockTx(t, initialBill.ID, fcrID, 1)
	roundNumber += 1
	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	txr, err = txSystem.Execute(unlockTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{unlockTx.UnitID, fcrID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	stateSummary, err = txSystem.EndBlock()
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	require.Len(t, txr.TargetUnits(), 2)
	require.Equal(t, unlockTx.UnitID, txr.ServerMetadata.TargetUnits[0])
	require.NoError(t, txSystem.Commit(createUC(stateSummary, 1)))

	_, bd = getBill(t, rmaTree, initialBill.ID)
	require.EqualValues(t, 0, bd.Locked)                // bill is unlocked
	require.EqualValues(t, 110, bd.SummaryValueInput()) // value not changed
	require.EqualValues(t, 2, bd.Counter)               // counter updated
}

func TestBillData_Value(t *testing.T) {
	bd := &money.BillData{
		Value:   10,
		Counter: 0,
	}

	actualSumValue := bd.SummaryValueInput()
	require.Equal(t, uint64(10), actualSumValue)
}

func TestBillData_AddToHasher(t *testing.T) {
	bd := &money.BillData{
		Version: 1,
		Value:   10,
		Counter: 0,
		Locked:  1,
	}

	hasher := crypto.SHA256.New()
	res, err := types.Cbor.Marshal(bd)
	require.NoError(t, err)
	hasher.Write(res)
	expectedHash := hasher.Sum(nil)
	hasher.Reset()
	abhasher := abhash.New(hasher)
	bd.Write(abhasher)
	actualHash, err := abhasher.Sum()
	require.NoError(t, err)
	require.Equal(t, expectedHash, actualHash)
	// make sure all fields where serialized
	var bdFormSerialized money.BillData
	require.NoError(t, types.Cbor.Unmarshal(res, &bdFormSerialized))
	require.Equal(t, bd, &bdFormSerialized)
}

func TestEndBlock_DustBillsAreRemoved(t *testing.T) {
	t.Skip("TODO AB-1133 implement dust bills deletion")
	pdrs := createPDRs(t)
	rmaTree, txSystem, signer := createStateAndTxSystem(t, pdrs)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	remaining := initBillData.Value
	var splitBillIDs = make([]types.UnitID, 10)
	counter := initBillData.Counter
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue(t)
	for i := 0; i < 10; i++ {
		remaining--
		splitOk, _, _ := createSplit(t, initialBill.ID, fcrID, []*money.TargetUnit{{Amount: 1, OwnerPredicate: templates.AlwaysTrueBytes()}}, counter)
		roundNumber := uint64(10)
		err := txSystem.BeginBlock(roundNumber)
		require.NoError(t, err)
		splitBillIDs[i], err = pdrs[0].ComposeUnitID(types.ShardID{}, money.BillUnitType, money.PrndSh(splitOk))
		require.NoError(t, err)
		txr, err := txSystem.Execute(splitOk)
		require.NoError(t, err)
		require.NotNil(t, txr)
		require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
		require.Equal(t, []types.UnitID{splitOk.UnitID, splitBillIDs[i], fcrID}, txr.TargetUnits())
		require.True(t, txr.ServerMetadata.ActualFee > 0)

		_, data := getBill(t, rmaTree, initialBill.ID)
		counter = data.Counter
	}

	sort.Slice(splitBillIDs, func(i, j int) bool {
		return bytes.Compare(splitBillIDs[i], splitBillIDs[j]) == -1
	})

	// use initial bill as target bill
	targetBillID := initialBill.ID
	_, targetBillData := getBill(t, rmaTree, initialBill.ID)

	dcTransferProofs, swapTx := createDCTransferAndSwapTxs(t, splitBillIDs, targetBillID, fcrID, targetBillData.Counter, rmaTree, signer)

	for _, dcTransferProof := range dcTransferProofs {
		tx, err := dcTransferProof.GetTransactionOrderV1()
		require.NoError(t, err)
		_, err = txSystem.Execute(tx)
		require.NoError(t, err)
	}
	_, err := txSystem.Execute(swapTx)
	require.NoError(t, err)
	_, newBillData := getBill(t, rmaTree, swapTx.UnitID)
	require.Equal(t, uint64(10), newBillData.Value)
	_, dustCollectorBill := getBill(t, rmaTree, DustCollectorMoneySupplyID)
	require.Equal(t, initialDustCollectorMoneyAmount, dustCollectorBill.Value)
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
	require.Equal(t, initialDustCollectorMoneyAmount, dustCollectorBill.Value)
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
	t.Skip("TODO fees consolidation will be removed")
	pdrs := createPDRs(t)
	rmaTree, txSystem, signer := createStateAndTxSystem(t, pdrs)

	// process transferFC with amount 50 and fees 1
	err := txSystem.BeginBlock(0)
	require.NoError(t, err)
	transferFC := testutils.NewTransferFC(t, signer,
		testutils.NewTransferFCAttr(t, signer,
			testutils.WithCounter(0),
		),
		testtransaction.WithPartition(pdrs[0]),
		testtransaction.WithUnitID(initialBill.ID),
	)

	txr, err := txSystem.Execute(transferFC)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{transferFC.UnitID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	stateSummary, err := txSystem.EndBlock()
	require.NoError(t, err)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, 2)))

	// verify that money fee credit bill is 50
	moneyFeeCreditBillID := moneyid.BillIDWithSuffix(t, 2, nil)
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
	closeFC.NetworkID = pdrs[0].NetworkID
	closeFCRecord := &types.TransactionRecord{
		Version:          1,
		TransactionOrder: testtransaction.TxoToBytes(t, closeFC),
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1, SuccessIndicator: types.TxStatusSuccessful},
	}
	closureTxProof := testblock.CreateTxRecordProof(t, closeFCRecord, signer)

	reclaimFC := testutils.NewReclaimFC(t, pdrs[0], signer,
		testutils.NewReclaimFCAttr(t, pdrs[0], signer,
			testutils.WithReclaimFCClosureProof(closureTxProof),
		),
		testtransaction.WithPartition(pdrs[0]),
		testtransaction.WithUnitID(initialBill.ID),
		testtransaction.WithTransactionType(fcsdk.TransactionTypeReclaimFeeCredit),
	)
	txr, err = txSystem.Execute(reclaimFC)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{reclaimFC.UnitID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	stateSummary, err = txSystem.EndBlock()
	require.NoError(t, err)
	require.NoError(t, txSystem.Commit(createUC(stateSummary, 3)))

	// verify that moneyFCB=50-50+1+1=2 (moneyFCB - closeAmount + closeFee + reclaimFee)
	moneyFeeCreditBill, err = rmaTree.GetUnit(moneyFeeCreditBillID, false)
	require.NoError(t, err)
	require.EqualValues(t, 2, moneyFeeCreditBill.Data().SummaryValueInput())
}

func TestRegisterData_RevertSplit(t *testing.T) {
	pdrs := createPDRs(t)
	rmaTree, txSystem, _ := createStateAndTxSystem(t, pdrs)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue(t)

	vdState, err := txSystem.StateSummary()
	require.NoError(t, err)

	var remaining uint64 = 10
	amount := initialBill.Value - remaining
	splitOk, _, _ := createSplit(t, initialBill.ID, fcrID, []*money.TargetUnit{{Amount: amount, OwnerPredicate: templates.AlwaysTrueBytes()}}, initBillData.Counter)
	require.NoError(t, err)
	roundNumber := uint64(10)
	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	txr, err := txSystem.Execute(splitOk)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	_, err = txSystem.StateSummary()
	require.ErrorIs(t, err, txsystem.ErrStateContainsUncommittedChanges)

	txSystem.Revert()
	s, err := txSystem.StateSummary()
	require.NoError(t, err)
	require.Equal(t, vdState, s)
}

func TestRegisterData_RevertTransDC(t *testing.T) {
	pdrs := createPDRs(t)
	rmaTree, txSystem, _ := createStateAndTxSystem(t, pdrs)
	_, initBillData := getBill(t, rmaTree, initialBill.ID)
	unitBearer := bytes.Clone(initBillData.OwnerPredicate)
	vdState, err := txSystem.StateSummary()
	require.NoError(t, err)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue(t)

	transDC, _, _ := createDCTransfer(t, initialBill.ID, fcrID, initialBill.Value, initBillData.Counter, test.RandomBytes(32), 0)
	require.NoError(t, err)
	roundNumber := uint64(10)
	err = txSystem.BeginBlock(roundNumber)
	require.NoError(t, err)
	txr, err := txSystem.Execute(transDC)
	require.NoError(t, err)
	require.NotNil(t, txr)
	_, err = txSystem.StateSummary()
	require.ErrorIs(t, err, txsystem.ErrStateContainsUncommittedChanges)
	_, bd := getBill(t, rmaTree, initialBill.ID)
	require.EqualValues(t, DustCollectorPredicate, bd.OwnerPredicate)
	require.EqualValues(t, 1, bd.Counter)

	txSystem.Revert()
	s, err := txSystem.StateSummary()
	require.NoError(t, err)
	require.Equal(t, vdState, s)
	_, bd = getBill(t, rmaTree, initialBill.ID)
	require.EqualValues(t, unitBearer, bd.OwnerPredicate)
	require.NotEqualValues(t, DustCollectorPredicate, bd.OwnerPredicate)
	require.EqualValues(t, 0, bd.Counter)
}

// Test Transfer->Add->Lock->Close->Reclaim sequence OK
func TestExecute_FeeCreditSequence_OK(t *testing.T) {
	pdrs := createPDRs(t)
	moneyPDR := pdrs[0]
	rmaTree, txSystem, signer := createStateAndTxSystem(t, pdrs)
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
		testtransaction.WithPartition(moneyPDR),
		testtransaction.WithUnitID(initialBillID),
	)
	txr, err := txSystem.Execute(transferFC)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	remainingValue := txAmount - txr.ServerMetadata.ActualFee

	// verify unit value is reduced by 20
	ib, err := rmaTree.GetUnit(initialBill.ID, false)
	require.NoError(t, err)
	require.EqualValues(t, initialBill.Value-txAmount, ib.Data().SummaryValueInput())
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	// send addFC
	transferFCTransactionRecord := &types.TransactionRecord{
		Version:          1,
		TransactionOrder: testtransaction.TxoToBytes(t, transferFC),
		ServerMetadata:   txr.ServerMetadata,
	}
	transferFCProof := testblock.CreateTxRecordProof(t, transferFCTransactionRecord, signer)
	addFC := testutils.NewAddFC(t, signer,
		testutils.NewAddFCAttr(t, signer,
			testutils.WithTransferFCProof(transferFCProof),
		),
		testtransaction.WithPartition(moneyPDR),
		testtransaction.WithUnitID(fcrID),
	)
	authProof := &fcsdk.AddFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, addFC, signer)}
	require.NoError(t, addFC.SetAuthProof(authProof))

	txr, err = txSystem.Execute(addFC)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.True(t, txr.ServerMetadata.ActualFee > 0)

	// verify user fee credit is 18 (transfer 20 minus fee 2 * fee)
	remainingValue -= txr.ServerMetadata.ActualFee // 18
	fcrUnit, err := rmaTree.GetUnit(fcrID, false)
	require.NoError(t, err)
	fcrUnitData, ok := fcrUnit.Data().(*fcsdk.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, remainingValue, fcrUnitData.Balance)

	// lock target unit
	lockTx, _, _ := createLockTx(t, initialBillID, fcrID, 1)
	lockTx.FeeProof = testsig.NewFeeProofSignature(t, lockTx, signer)

	// execute lock transaction
	txr, err = txSystem.Execute(lockTx)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	remainingValue -= txr.ServerMetadata.ActualFee

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
		testtransaction.WithPartition(moneyPDR),
	)
	closeFCAuthProof := &fcsdk.CloseFeeCreditAuthProof{OwnerProof: testsig.NewAuthProofSignature(t, closeFC, signer)}
	require.NoError(t, closeFC.SetAuthProof(closeFCAuthProof))

	txr, err = txSystem.Execute(closeFC)
	require.NoError(t, err)
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	remainingValue -= txr.ServerMetadata.ActualFee

	// verify user fee credit is closed (balance 0, unit will be deleted on round completion)
	fcrUnit, err = rmaTree.GetUnit(fcrID, false)
	require.NoError(t, err)
	fcrUnitData, ok = fcrUnit.Data().(*fcsdk.FeeCreditRecord)
	require.True(t, ok)
	require.EqualValues(t, 0, fcrUnitData.Balance)

	// send reclaimFC
	closeFCTransactionRecord := &types.TransactionRecord{
		Version:          1,
		TransactionOrder: testtransaction.TxoToBytes(t, closeFC),
		ServerMetadata:   txr.ServerMetadata,
	}
	closeFCProof := testblock.CreateTxRecordProof(t, closeFCTransactionRecord, signer)
	reclaimFC := testutils.NewReclaimFC(t, moneyPDR, signer,
		testutils.NewReclaimFCAttr(t, pdrs[0], signer,
			testutils.WithReclaimFCClosureProof(closeFCProof),
		),
		testtransaction.WithUnitID(initialBillID),
		testtransaction.WithTransactionType(fcsdk.TransactionTypeReclaimFeeCredit),
		testtransaction.WithPartition(moneyPDR),
	)
	txr, err = txSystem.Execute(reclaimFC)
	require.NoError(t, err)
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	remainingValue -= txr.ServerMetadata.ActualFee

	totalFeeCost := txAmount - remainingValue
	// verify reclaimed fee is added back to initial bill (original value minus 5x txfee)
	ib, err = rmaTree.GetUnit(initialBill.ID, false)
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, initialBill.Value-totalFeeCost, ib.Data().SummaryValueInput())

	// and initial bill got unlocked
	bd, ok = ib.Data().(*money.BillData)
	require.True(t, ok)
	require.EqualValues(t, 0, bd.Locked)

	// and counter was incremented
	require.EqualValues(t, 3, bd.Counter)
}

// Test LockFC -> UnlockFC
func TestExecute_AddFeeCreditWithLocking_OK(t *testing.T) {
	pdrs := createPDRs(t)
	rmaTree, txSystem, _ := createStateAndTxSystem(t, pdrs)
	err := txSystem.BeginBlock(1)
	require.NoError(t, err)
	signer, _ := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue(t)

	// lock fee credit record
	lockFCAttr := testutils.NewLockFCAttr(testutils.WithLockFCCounter(0))
	lockFC := testutils.NewLockFC(t, signer, lockFCAttr,
		testtransaction.WithUnitID(fcrID),
		testtransaction.WithPartition(pdrs[0]),
	)
	txr, err := txSystem.Execute(lockFC)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{lockFC.UnitID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	// verify unit was locked
	u, err := rmaTree.GetUnit(fcrID, false)
	require.NoError(t, err)
	fcr, ok := u.Data().(*fcsdk.FeeCreditRecord)
	require.True(t, ok)
	require.True(t, fcr.IsLocked())

	// unlock fee credit record
	unlockFCAttr := testutils.NewUnlockFCAttr(testutils.WithUnlockFCCounter(1))
	unlockFC := testutils.NewUnlockFC(t, signer, unlockFCAttr, testtransaction.WithUnitID(fcrID), testtransaction.WithPartition(pdrs[0]))
	txr, err = txSystem.Execute(unlockFC)
	require.NoError(t, err)
	require.NotNil(t, txr)
	require.Equal(t, types.TxStatusSuccessful, txr.ServerMetadata.SuccessIndicator)
	require.Equal(t, []types.UnitID{unlockFC.UnitID}, txr.TargetUnits())
	require.True(t, txr.ServerMetadata.ActualFee > 0)
	// verify unit was unlocked
	fcrUnit, err := rmaTree.GetUnit(fcrID, false)
	require.NoError(t, err)
	fcr, ok = fcrUnit.Data().(*fcsdk.FeeCreditRecord)
	require.True(t, ok)
	require.False(t, fcr.IsLocked())
}

func getBill(t *testing.T, s *state.State, billID types.UnitID) (state.Unit, *money.BillData) {
	t.Helper()
	ib, err := s.GetUnit(billID, false)
	require.NoError(t, err)
	require.IsType(t, &money.BillData{}, ib.Data())
	return ib, ib.Data().(*money.BillData)
}

func createBillTransfer(t *testing.T, fromID types.UnitID, fcrID types.UnitID, value uint64, bearer []byte, counter uint64) (*types.TransactionOrder, *money.TransferAttributes, *money.TransferAuthProof) {
	tx := createTx(fromID, fcrID, money.TransactionTypeTransfer)
	attr := &money.TransferAttributes{
		NewOwnerPredicate: bearer,
		// #nosec G404
		TargetValue: value,
		Counter:     counter,
	}
	require.NoError(t, tx.SetAttributes(attr))

	authProof := &money.TransferAuthProof{OwnerProof: nil}
	require.NoError(t, tx.SetAuthProof(authProof))

	return tx, attr, authProof
}

func createLockTx(t *testing.T, fromID types.UnitID, fcrID types.UnitID, counter uint64) (*types.TransactionOrder, *money.LockAttributes, *money.LockAuthProof) {
	tx := createTx(fromID, fcrID, money.TransactionTypeLock)
	attr := &money.LockAttributes{
		LockStatus: 1,
		Counter:    counter,
	}
	require.NoError(t, tx.SetAttributes(attr))

	authProof := &money.LockAuthProof{OwnerProof: nil}
	require.NoError(t, tx.SetAuthProof(authProof))

	return tx, attr, authProof
}

func createUnlockTx(t *testing.T, fromID types.UnitID, fcrID types.UnitID, counter uint64) (*types.TransactionOrder, *money.UnlockAttributes, *money.UnlockAuthProof) {
	tx := createTx(fromID, fcrID, money.TransactionTypeUnlock)
	attr := &money.UnlockAttributes{
		Counter: counter,
	}
	require.NoError(t, tx.SetAttributes(attr))

	authProof := &money.UnlockAuthProof{OwnerProof: nil}
	require.NoError(t, tx.SetAuthProof(authProof))

	return tx, attr, authProof
}

func createDCTransferAndSwapTxs(
	t *testing.T,
	ids []types.UnitID, // bills to swap
	fcrID types.UnitID,
	targetID []byte,
	targetCounter uint64,
	rmaTree *state.State,
	signer abcrypto.Signer) ([]*types.TxRecordProof, *types.TransactionOrder) {

	t.Helper()

	// create dc transfers
	dustTransferProofs := make([]*types.TxRecordProof, len(ids))

	for i, id := range ids {
		_, billData := getBill(t, rmaTree, id)
		tx, _, _ := createDCTransfer(t, id, fcrID, billData.Value, billData.Counter, targetID, targetCounter)
		txr := &types.TransactionRecord{
			Version:          1,
			TransactionOrder: testtransaction.TxoToBytes(t, tx),
			ServerMetadata:   &types.ServerMetadata{ActualFee: 1, SuccessIndicator: types.TxStatusSuccessful},
		}
		dustTransferProofs[i] = testblock.CreateTxRecordProof(t, txr, signer)
	}

	tx := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			NetworkID:   networkID,
			PartitionID: moneyPartitionID,
			UnitID:      targetID,
			Type:        money.TransactionTypeSwapDC,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           20,
				MaxTransactionFee: 10,
				FeeCreditRecordID: fcrID,
			},
		},
	}

	attr := &money.SwapDCAttributes{
		DustTransferProofs: dustTransferProofs,
	}
	require.NoError(t, tx.SetAttributes(attr))

	authProof := &money.SwapDCAuthProof{OwnerProof: nil}
	require.NoError(t, tx.SetAuthProof(authProof))

	return dustTransferProofs, tx
}

func createDCTransfer(t *testing.T, fromID types.UnitID, fcrID types.UnitID, val uint64, counter uint64, targetID []byte, targetCounter uint64) (*types.TransactionOrder, *money.TransferDCAttributes, *money.TransferDCAuthProof) {
	tx := createTx(fromID, fcrID, money.TransactionTypeTransDC)
	attr := &money.TransferDCAttributes{
		Value:             val,
		TargetUnitID:      targetID,
		TargetUnitCounter: targetCounter,
		Counter:           counter,
	}
	require.NoError(t, tx.SetAttributes(attr))

	authProof := &money.TransferDCAuthProof{OwnerProof: nil}
	require.NoError(t, tx.SetAuthProof(authProof))

	return tx, attr, authProof
}

func createSplit(t *testing.T, fromID types.UnitID, fcrID types.UnitID, targetUnits []*money.TargetUnit, counter uint64) (*types.TransactionOrder, *money.SplitAttributes, *money.SplitAuthProof) {
	tx := createTx(fromID, fcrID, money.TransactionTypeSplit)
	attr := &money.SplitAttributes{
		TargetUnits: targetUnits,
		Counter:     counter,
	}
	require.NoError(t, tx.SetAttributes(attr))

	authProof := &money.SplitAuthProof{OwnerProof: nil}
	require.NoError(t, tx.SetAuthProof(authProof))

	return tx, attr, authProof
}

func createTx(fromID types.UnitID, fcrID types.UnitID, transactionType uint16) *types.TransactionOrder {
	tx := &types.TransactionOrder{
		Version: 1,
		Payload: types.Payload{
			NetworkID:   networkID,
			PartitionID: moneyPartitionID,
			UnitID:      fromID,
			Type:        transactionType,
			Attributes:  nil,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           20,
				MaxTransactionFee: 10,
				FeeCreditRecordID: fcrID,
			},
		},
		//OwnerProof: templates.EmptyArgument(), // transfers "always true" unit by default
		FeeProof: templates.EmptyArgument(),
	}
	return tx
}

func createStateAndTxSystem(t *testing.T, pdrs []*types.PartitionDescriptionRecord) (*state.State, *txsystem.GenericTxSystem, abcrypto.Signer) {
	require.Greater(t, len(pdrs), 0, "at least one PDR must be provided")
	require.Equal(t, money.PartitionTypeID, pdrs[0].PartitionTypeID, "first PDR must be for the money partition")
	s := genesisStateWithUC(t, initialBill, pdrs)
	signer, verifier := testsig.CreateSignerAndVerifier(t)
	trustBase := testtb.NewTrustBase(t, verifier)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue(t)

	mss, err := NewTxSystem(
		*pdrs[0],
		types.ShardID{},
		observability.Default(t),
		WithState(s),
		WithTrustBase(trustBase),
	)
	require.NoError(t, err)
	summary, err := mss.StateSummary()
	require.NoError(t, err)
	require.NotNil(t, summary.Summary())
	require.NotNil(t, summary.Root())
	require.Len(t, summary.Root(), crypto.SHA256.Size())
	c, err := s.IsCommitted()
	require.NoError(t, err)
	require.True(t, c)
	// add fee credit record with empty predicate
	fcrData := &fcsdk.FeeCreditRecord{
		Balance:        100,
		MinLifetime:    100,
		OwnerPredicate: templates.AlwaysTrueBytes(),
	}
	err = s.Apply(unit.AddCredit(fcrID, fcrData))
	require.NoError(t, err)
	summary, err = mss.EndBlock()
	require.NoError(t, err)
	err = mss.Commit(createUC(summary, 2))
	require.NoError(t, err)

	return s, mss, signer
}

// Duplicates txsystem/money/testutils/genesis_state.go
func genesisState(t *testing.T, initialBill *InitialBill, sdrs []*types.PartitionDescriptionRecord) *state.State {
	s := state.NewEmptyState()
	randomHash := test.RandomBytes(32)

	// initial bill
	require.NoError(t, s.Apply(state.AddUnit(initialBill.ID, money.NewBillData(initialBill.Value, initialBill.Owner))))
	require.NoError(t, s.AddUnitLog(initialBill.ID, randomHash))

	// dust collector money supply
	require.NoError(t, s.Apply(state.AddUnit(DustCollectorMoneySupplyID, money.NewBillData(initialDustCollectorMoneyAmount, DustCollectorPredicate))))
	require.NoError(t, s.AddUnitLog(DustCollectorMoneySupplyID, randomHash))

	// fee credit bills
	for _, pdr := range sdrs {
		fcb := pdr.FeeCreditBill
		require.NoError(t, s.Apply(state.AddUnit(fcb.UnitID, &money.BillData{})))
		require.NoError(t, s.AddUnitLog(fcb.UnitID, randomHash))
	}

	_, _, err := s.CalculateRoot()
	require.NoError(t, err)

	return s
}

func genesisStateWithUC(t *testing.T, initialBill *InitialBill, sdrs []*types.PartitionDescriptionRecord) *state.State {
	s := genesisState(t, initialBill, sdrs)
	summaryValue, summaryHash, err := s.CalculateRoot()
	require.NoError(t, err)
	require.NoError(t, s.Commit(&types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{
		Version:      1,
		RoundNumber:  1,
		Hash:         summaryHash,
		SummaryValue: util.Uint64ToBytes(summaryValue),
	}}))
	return s
}

func createPDRs(t *testing.T) []*types.PartitionDescriptionRecord {
	return []*types.PartitionDescriptionRecord{{
		Version:         1,
		PartitionTypeID: money.PartitionTypeID,
		NetworkID:       types.NetworkLocal,
		PartitionID:     money.DefaultPartitionID,
		TypeIDLen:       8,
		UnitIDLen:       256,
		T2Timeout:       2500 * time.Millisecond,
		FeeCreditBill: &types.FeeCreditBill{
			UnitID:         moneyid.BillIDWithSuffix(t, 2, nil),
			OwnerPredicate: templates.AlwaysTrueBytes(),
		},
	}}
}

func createUC(s txsystem.StateSummary, roundNumber uint64) *types.UnicityCertificate {
	return &types.UnicityCertificate{Version: 1, InputRecord: &types.InputRecord{
		Version:      1,
		RoundNumber:  roundNumber,
		Hash:         s.Root(),
		SummaryValue: s.Summary(),
	}}
}

type moneyModuleOption func(m *Module) error

func withStateUnit(unitID []byte, data types.UnitData) moneyModuleOption {
	return func(m *Module) error {
		return m.state.Apply(state.AddUnit(unitID, data))
	}
}

func newTestMoneyModule(t *testing.T, verifier abcrypto.Verifier, opts ...moneyModuleOption) *Module {
	module := defaultMoneyModule(t, moneyid.PDR(), verifier)
	for _, opt := range opts {
		require.NoError(t, opt(module))
	}
	return module
}

func defaultMoneyModule(t *testing.T, pdr types.PartitionDescriptionRecord, verifier abcrypto.Verifier) *Module {
	// NB! using the same pubkey for trust base and unit bearer! TODO: use different keys...
	options, err := defaultOptions(observability.Default(t))
	require.NoError(t, err)
	options.trustBase = testtb.NewTrustBase(t, verifier)
	options.state = state.NewEmptyState()
	module, err := NewMoneyModule(pdr, options)
	require.NoError(t, err)
	return module
}
