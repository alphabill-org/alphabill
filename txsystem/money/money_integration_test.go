package money

import (
	"bytes"
	"crypto"
	"sort"
	"testing"

	abcrypto "github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	"github.com/alphabill-org/alphabill/partition/event"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/fc/unit"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

var (
	systemIdentifier = DefaultSystemIdentifier

	pubKey1  = "0x0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f0"
	privKey1 = "0xa5e8bff9733ebc751a45ca4b8cc6ce8e76c8316a5eb556f738092df6232e78de"

	pubKey2  = "0x02d29cbdea6062c0a9d9170245188fa39a12ad3dd6cc02a78fcc026594d9bdc06c"
	privKey2 = "0xd7e5041766e8ca505ab07ffa46652e248ede22b436ec81b583a78c8c9e1aac6b"
)

func TestPartition_Ok(t *testing.T) {
	const moneyInvariant = uint64(10000 * 1e8)
	total := moneyInvariant
	initialBill := &InitialBill{
		ID:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1},
		Value: moneyInvariant,
		Owner: templates.AlwaysTrueBytes(),
	}
	sdrs := createSDRs(newBillID(2))
	s := genesisState(t, initialBill, sdrs)
	moneyPrt, err := testpartition.NewPartition(t, 3, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		s = s.Clone()
		system, err := NewTxSystem(
			logger.New(t),
			WithState(s),
			WithSystemIdentifier(systemIdentifier),
			WithHashAlgorithm(crypto.SHA256),
			WithSystemDescriptionRecords(sdrs),
			WithTrustBase(tb),
			WithFeeCalculator(fc.FixedFee(1)),
		)
		require.NoError(t, err)
		return system
	}, systemIdentifier, s)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{moneyPrt})

	require.NoError(t, err)
	require.NoError(t, abNet.Start(t))
	defer abNet.WaitClose(t)

	// create fee credit for initial bill transfer
	transferFC := testutils.NewTransferFC(t,
		testutils.NewTransferFCAttr(
			testutils.WithBacklink(nil),
			testutils.WithAmount(fcrAmount),
			testutils.WithTargetRecordID(fcrID),
		),
		testtransaction.WithUnitId(initialBill.ID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(transactions.PayloadTypeTransferFeeCredit),
	)
	require.NoError(t, moneyPrt.SubmitTx(transferFC))
	transferFCRecord, transferFCProof, err := testpartition.WaitTxProof(t, moneyPrt, transferFC)
	require.NoError(t, err, "transfer fee credit tx failed")
	unitAndProof, err := testpartition.WaitUnitProof(t, moneyPrt, initialBill.ID, transferFC)
	require.NoError(t, err)
	var billState BillData
	require.NoError(t, unitAndProof.UnmarshalUnitData(&billState))
	require.Equal(t, moneyInvariant-fcrAmount, billState.V)
	// verify proof
	ucv, err := abNet.GetValidator(systemIdentifier)
	require.NoError(t, err)
	require.NoError(t, types.VerifyUnitStateProof(unitAndProof.Proof, crypto.SHA256, unitAndProof.UnitData, ucv))
	// send addFC
	addFC := testutils.NewAddFC(t, abNet.RootPartition.Nodes[0].RootSigner,
		testutils.NewAddFCAttr(t, abNet.RootPartition.Nodes[0].RootSigner,
			testutils.WithTransferFCTx(transferFCRecord),
			testutils.WithTransferFCProof(transferFCProof),
			testutils.WithFCOwnerCondition(templates.AlwaysTrueBytes()),
		),
		testtransaction.WithUnitId(fcrID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(transactions.PayloadTypeAddFeeCredit),
	)
	require.NoError(t, moneyPrt.SubmitTx(addFC))
	// before reading state make sure that node 2 has executed the transfer
	addTxRecord, _, err := testpartition.WaitTxProof(t, moneyPrt, addFC)
	require.NoError(t, err, "add fee credit tx failed")
	unitAndProof, err = testpartition.WaitUnitProof(t, moneyPrt, fcrID, addFC)
	require.NoError(t, err)
	require.NoError(t, types.VerifyUnitStateProof(unitAndProof.Proof, crypto.SHA256, unitAndProof.UnitData, ucv))
	// verify that frc bill is created and its balance is equal to frcAmount - "transfer tx cost" - "add tx cost"
	var feeBillState unit.FeeCreditRecord
	require.NoError(t, unitAndProof.UnmarshalUnitData(&feeBillState))
	remainingFeeBalance := fcrAmount - transferFCRecord.ServerMetadata.ActualFee - addTxRecord.ServerMetadata.ActualFee
	require.Equal(t, remainingFeeBalance, feeBillState.Balance)
	// transfer initial bill to pubKey1
	transferInitialBillTx, _ := createBillTransfer(t, initialBill.ID, total-fcrAmount, templates.NewP2pkh256BytesFromKeyHash(decodeAndHashHex(pubKey1)), transferFC.Hash(crypto.SHA256))
	require.NoError(t, moneyPrt.SubmitTx(transferInitialBillTx))
	txRecord, _, err := testpartition.WaitTxProof(t, moneyPrt, transferInitialBillTx)
	require.NoError(t, err, "transfer initial bill failed")
	unitAndProof, err = testpartition.WaitUnitProof(t, moneyPrt, fcrID, transferInitialBillTx)
	require.NoError(t, err)
	require.NoError(t, types.VerifyUnitStateProof(unitAndProof.Proof, crypto.SHA256, unitAndProof.UnitData, ucv))
	require.NoError(t, unitAndProof.UnmarshalUnitData(&feeBillState))
	remainingFeeBalance = remainingFeeBalance - txRecord.ServerMetadata.GetActualFee()
	require.Equal(t, remainingFeeBalance, feeBillState.Balance)
	transferInitialBillTxRecord := txRecord
	// split initial bill from pubKey1 to pubKey2
	amountPK2 := uint64(1000)
	targetUnit := &TargetUnit{Amount: amountPK2, OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(decodeAndHashHex(pubKey2))}
	remainingValue := total - fcrAmount - amountPK2
	tx := createSplitTx(t, initialBill.ID, transferInitialBillTxRecord, []*TargetUnit{targetUnit}, remainingValue)
	require.NoError(t, moneyPrt.SubmitTx(tx))
	txRecord, _, err = testpartition.WaitTxProof(t, moneyPrt, tx)
	require.NoError(t, err, "money split tx failed")
	unitAndProof, err = testpartition.WaitUnitProof(t, moneyPrt, fcrID, tx)
	require.NoError(t, err)
	require.NoError(t, types.VerifyUnitStateProof(unitAndProof.Proof, crypto.SHA256, unitAndProof.UnitData, ucv))
	require.NoError(t, unitAndProof.UnmarshalUnitData(&feeBillState))
	remainingFeeBalance = remainingFeeBalance - txRecord.ServerMetadata.GetActualFee()
	require.EqualValues(t, remainingFeeBalance, feeBillState.Balance)

	// wrong partition tx
	tx = createSplitTx(t, initialBill.ID, transferInitialBillTxRecord, []*TargetUnit{targetUnit}, remainingValue)
	tx.Payload.SystemID = 0x01010101
	require.ErrorContains(t, moneyPrt.SubmitTx(tx), "invalid transaction system identifier")
	// and fee unit is not changed
	feeCredit, err := s.GetUnit(fcrID, true)
	require.NoError(t, err)
	require.Equal(t, remainingFeeBalance, feeCredit.Data().(*unit.FeeCreditRecord).Balance)

	for _, n := range moneyPrt.Nodes {
		testevent.NotContainsEvent(t, n.EventHandler, event.RecoveryStarted)
	}
}

func TestPartition_SwapDCOk(t *testing.T) {
	const moneyInvariant = uint64(10000 * 1e8)
	const nofDustToSwap = 3

	var (
		txsState    *state.State
		initialBill = &InitialBill{
			ID:    NewBillID(nil, []byte{1}),
			Value: moneyInvariant,
			Owner: templates.AlwaysTrueBytes(),
		}
	)
	total := moneyInvariant
	sdrs := createSDRs(newBillID(99))
	txsState = genesisState(t, initialBill, sdrs)
	moneyPrt, err := testpartition.NewPartition(t, 3, func(tb map[string]abcrypto.Verifier) txsystem.TransactionSystem {
		var err error
		// trustBase = tb
		txsState = txsState.Clone()
		system, err := NewTxSystem(
			logger.New(t),
			WithSystemIdentifier(systemIdentifier),
			WithHashAlgorithm(crypto.SHA256),
			WithSystemDescriptionRecords(sdrs),
			WithTrustBase(tb),
			WithState(txsState),
			WithFeeCalculator(fc.FixedFee(1)),
		)
		require.NoError(t, err)
		return system
	}, systemIdentifier, txsState)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{moneyPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start(t))
	defer abNet.WaitClose(t)

	// create fee credit for initial bill transfer
	transferFC := testutils.NewTransferFC(t,
		testutils.NewTransferFCAttr(
			testutils.WithBacklink(nil),
			testutils.WithAmount(fcrAmount),
			testutils.WithTargetRecordID(fcrID),
		),
		testtransaction.WithUnitId(initialBill.ID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(transactions.PayloadTypeTransferFeeCredit),
	)
	require.NoError(t, moneyPrt.SubmitTx(transferFC))
	transferFCRecord, transferFCProof, err := testpartition.WaitTxProof(t, moneyPrt, transferFC)
	require.NoError(t, err, "transfer fee credit tx failed")
	// check that frcAmount is credited from initial bill
	bill, err := txsState.GetUnit(initialBill.ID, true)
	require.NoError(t, err)
	require.Equal(t, moneyInvariant-fcrAmount, bill.Data().(*BillData).V)
	// send addFC
	addFC := testutils.NewAddFC(t, abNet.RootPartition.Nodes[0].RootSigner,
		testutils.NewAddFCAttr(t, abNet.RootPartition.Nodes[0].RootSigner,
			testutils.WithTransferFCTx(transferFCRecord),
			testutils.WithTransferFCProof(transferFCProof),
			testutils.WithFCOwnerCondition(templates.AlwaysTrueBytes()),
		),
		testtransaction.WithUnitId(fcrID),
		testtransaction.WithOwnerProof(nil),
		testtransaction.WithPayloadType(transactions.PayloadTypeAddFeeCredit),
	)
	require.NoError(t, moneyPrt.SubmitTx(addFC))
	// before reading state make sure that node 2 has executed the transfer
	addTxRecord, _, err := testpartition.WaitTxProof(t, moneyPrt, addFC)
	require.NoError(t, err, "add fee credit tx failed")
	// verify that frc bill is created and its balance is equal to frcAmount - "transfer tx cost" - "add tx cost"
	feeCredit, err := txsState.GetUnit(fcrID, true)
	require.NoError(t, err)
	require.Equal(t, fcrAmount-transferFCRecord.ServerMetadata.ActualFee-addTxRecord.ServerMetadata.ActualFee, feeCredit.Data().(*unit.FeeCreditRecord).Balance)

	// transfer initial bill to pubKey1
	transferInitialBillTx, _ := createBillTransfer(t, initialBill.ID, total-fcrAmount, templates.NewP2pkh256BytesFromKeyHash(decodeAndHashHex(pubKey1)), transferFC.Hash(crypto.SHA256))
	require.NoError(t, moneyPrt.SubmitTx(transferInitialBillTx))
	// wait for transaction to be added to block
	txRecord, _, err := testpartition.WaitTxProof(t, moneyPrt, transferInitialBillTx)
	require.NoError(t, err, "transfer initial bill failed")
	require.EqualValues(t, transferInitialBillTx, txRecord.TransactionOrder)
	feeCredit, err = txsState.GetUnit(fcrID, true)
	require.NoError(t, err)
	require.Equal(t, fcrAmount-3, feeCredit.Data().(*unit.FeeCreditRecord).Balance)

	// split initial bill using N-way split where N=nofDustToSwap
	amount := uint64(1)
	_, _, transferRecord, err := moneyPrt.GetTxProof(transferInitialBillTx)
	require.NoError(t, err)
	prev := transferRecord
	total -= fcrAmount

	var targetUnits []*TargetUnit
	for i := 0; i < nofDustToSwap; i++ {
		targetUnits = append(targetUnits, &TargetUnit{Amount: amount, OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(decodeAndHashHex(pubKey2))})
		total -= amount
		amount++
	}
	splitTx := createSplitTx(t, initialBill.ID, prev, targetUnits, total)
	require.NoError(t, moneyPrt.SubmitTx(splitTx))

	// wait for transaction to be added to block
	txRecord, _, err = testpartition.WaitTxProof(t, moneyPrt, splitTx)
	require.NoError(t, err, "money split tx failed")
	require.EqualValues(t, splitTx, txRecord.TransactionOrder)

	// create dust payments from splits
	dcBillIds := make([]types.UnitID, nofDustToSwap)
	for i := 0; i < nofDustToSwap; i++ {
		dcBillIds[i] = NewBillID(nil, unitIDFromTransaction(splitTx, util.Uint32ToBytes(uint32(i))))
	}
	// sort bill id's
	sort.Slice(dcBillIds, func(i, j int) bool {
		return bytes.Compare(dcBillIds[i], dcBillIds[j]) == -1
	})
	dcTxs, sum := createDCAndSwapTxs(t, initialBill.ID, splitTx.Hash(crypto.SHA256), dcBillIds, txsState)
	dcRecords := make([]*types.TransactionRecord, len(dcTxs))
	dcRecordsProofs := make([]*types.TxProof, len(dcTxs))
	for i, dcTx := range dcTxs {
		require.NoError(t, moneyPrt.SubmitTx(dcTx))
		dcRecords[i], dcRecordsProofs[i], err = testpartition.WaitTxProof(t, moneyPrt, dcTx)
		require.NoError(t, err, "dc tx failed")
	}

	// create swap order
	swapAttr := &SwapDCAttributes{
		OwnerCondition:   templates.NewP2pkh256BytesFromKeyHash(decodeAndHashHex(pubKey1)),
		DcTransfers:      dcRecords,
		DcTransferProofs: dcRecordsProofs,
		TargetValue:      sum,
	}
	swapBytes, err := cbor.Marshal(swapAttr)
	require.NoError(t, err)

	// create swap tx
	swapTx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   systemIdentifier,
			Type:       PayloadTypeSwapDC,
			UnitID:     initialBill.ID,
			Attributes: swapBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           20,
				MaxTransactionFee: 10,
				FeeCreditRecordID: fcrID,
			},
		},
		OwnerProof: nil,
		FeeProof:   templates.EmptyArgument(),
	}
	require.NoError(t, swapTx.SetOwnerProof(predicates.OwnerProoferSecp256K1(decodeHex(privKey1), decodeHex(pubKey1))))

	require.NoError(t, moneyPrt.SubmitTx(swapTx))
	_, _, err = testpartition.WaitTxProof(t, moneyPrt, swapTx)
	require.NoError(t, err)
	for _, n := range moneyPrt.Nodes {
		testevent.NotContainsEvent(t, n.EventHandler, event.RecoveryStarted)
	}
}

func createSplitTx(t *testing.T, fromID []byte, prevTx *types.TransactionRecord, targetUnits []*TargetUnit, remaining uint64) *types.TransactionOrder {
	backlink := prevTx.TransactionOrder.Hash(crypto.SHA256)
	tx, _ := createSplit(t, fromID, targetUnits, remaining, backlink)
	require.NoError(t, tx.SetOwnerProof(predicates.OwnerProoferSecp256K1(decodeHex(privKey1), decodeHex(pubKey1))))
	return tx
}

func createDCAndSwapTxs(
	t *testing.T,
	targetID []byte,
	targetBacklink []byte,
	ids []types.UnitID, // bills to swap
	s *state.State) ([]*types.TransactionOrder, uint64) {
	t.Helper()

	// create dc transfers
	dcTransfers := make([]*types.TransactionOrder, len(ids))

	var targetValue uint64 = 0
	for i, id := range ids {
		_, billData := getBill(t, s, id)
		// NB! dc transfer target backlink must be equal to swap tx unit id
		targetValue += billData.V
		tx, _ := createDCTransfer(t, id, billData.V, billData.Backlink, targetID, targetBacklink)
		tx.SetOwnerProof(predicates.OwnerProoferSecp256K1(decodeHex(privKey2), decodeHex(pubKey2)))
		dcTransfers[i] = tx
	}

	return dcTransfers, targetValue
}

func decodeAndHashHex(hex string) []byte {
	hasher := crypto.SHA256.New()
	hasher.Write(decodeHex(hex))
	return hasher.Sum(nil)
}

func decodeHex(hex string) []byte {
	decoded, _ := hexutil.Decode(hex)
	return decoded
}

func newBillID(unitPart byte) types.UnitID {
	return NewBillID(nil, []byte{unitPart})
}
