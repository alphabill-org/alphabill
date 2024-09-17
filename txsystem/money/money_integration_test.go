package money

import (
	"bytes"
	"crypto"
	"sort"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	fcsdk "github.com/alphabill-org/alphabill-go-base/txsystem/fc"
	"github.com/alphabill-org/alphabill-go-base/txsystem/money"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"

	"github.com/alphabill-org/alphabill/internal/testutils/observability"
	testpartition "github.com/alphabill-org/alphabill/internal/testutils/partition"
	testevent "github.com/alphabill-org/alphabill/internal/testutils/partition/event"
	testsig "github.com/alphabill-org/alphabill/internal/testutils/sig"
	"github.com/alphabill-org/alphabill/partition/event"
	"github.com/alphabill-org/alphabill/state"
	"github.com/alphabill-org/alphabill/txsystem"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	testtransaction "github.com/alphabill-org/alphabill/txsystem/testutils/transaction"
)

var (
	systemIdentifier = money.DefaultSystemID

	pubKey1  = "0x0212911c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c107f0"
	privKey1 = "0xa5e8bff9733ebc751a45ca4b8cc6ce8e76c8316a5eb556f738092df6232e78de"

	pubKey2  = "0x02d29cbdea6062c0a9d9170245188fa39a12ad3dd6cc02a78fcc026594d9bdc06c"
	privKey2 = "0xd7e5041766e8ca505ab07ffa46652e248ede22b436ec81b583a78c8c9e1aac6b"
)

func TestPartition_Ok(t *testing.T) {
	const moneyInvariant = uint64(10000 * 1e8)
	total := moneyInvariant
	ib := &InitialBill{
		ID:    []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1},
		Value: moneyInvariant,
		Owner: templates.AlwaysTrueBytes(),
	}
	pdr := types.PartitionDescriptionRecord{
		SystemIdentifier: money.DefaultSystemID,
		TypeIdLen:        8,
		UnitIdLen:        256,
		T2Timeout:        2000 * time.Millisecond,
	}
	sdrs := createSDRs(newBillID(2))
	s := genesisState(t, ib, sdrs)
	moneyPrt, err := testpartition.NewPartition(t, 3, func(tb types.RootTrustBase) txsystem.TransactionSystem {
		s = s.Clone()
		system, err := NewTxSystem(
			observability.Default(t),
			WithState(s),
			WithSystemIdentifier(pdr.SystemIdentifier),
			WithHashAlgorithm(crypto.SHA256),
			WithPartitionDescriptionRecords(sdrs),
			WithTrustBase(tb),
		)
		require.NoError(t, err)
		return system
	}, pdr, s)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{moneyPrt})

	require.NoError(t, err)
	require.NoError(t, abNet.Start(t))
	defer abNet.WaitClose(t)

	// create fee credit for initial bill transfer
	signer, _ := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()
	transferFC := testutils.NewTransferFC(t, signer,
		testutils.NewTransferFCAttr(t, signer,
			testutils.WithCounter(0),
			testutils.WithAmount(fcrAmount),
			testutils.WithTargetRecordID(fcrID),
		),
		testtransaction.WithUnitID(ib.ID),
		testtransaction.WithPayloadType(fcsdk.PayloadTypeTransferFeeCredit),
	)
	require.NoError(t, moneyPrt.SubmitTx(transferFC))
	transferFCRecord, transferFCProof, err := testpartition.WaitTxProof(t, moneyPrt, transferFC)
	require.NoError(t, err, "transfer fee credit transaction failed")
	unitAndProof, err := testpartition.WaitUnitProof(t, moneyPrt, ib.ID, transferFC)
	require.NoError(t, err)
	var billState money.BillData
	require.NoError(t, unitAndProof.UnmarshalUnitData(&billState))
	require.Equal(t, moneyInvariant-fcrAmount, billState.V)

	// verify proof
	ucv, err := abNet.GetValidator(pdr.SystemIdentifier)
	require.NoError(t, err)
	require.NoError(t, types.VerifyUnitStateProof(unitAndProof.Proof, crypto.SHA256, unitAndProof.UnitData, ucv))

	// send addFC
	addFC := testutils.NewAddFC(t, signer,
		testutils.NewAddFCAttr(t, signer,
			testutils.WithTransferFCRecord(transferFCRecord),
			testutils.WithTransferFCProof(transferFCProof),
			testutils.WithFeeCreditOwnerPredicate(templates.AlwaysTrueBytes()),
		),
		testtransaction.WithUnitID(fcrID),
		testtransaction.WithPayloadType(fcsdk.PayloadTypeAddFeeCredit),
	)
	require.NoError(t, moneyPrt.SubmitTx(addFC))

	// before reading state make sure that node 2 has executed the transfer
	addTxRecord, _, err := testpartition.WaitTxProof(t, moneyPrt, addFC)
	require.NoError(t, err, "add fee credit transaction failed")
	unitAndProof, err = testpartition.WaitUnitProof(t, moneyPrt, fcrID, addFC)
	require.NoError(t, err)
	require.NoError(t, types.VerifyUnitStateProof(unitAndProof.Proof, crypto.SHA256, unitAndProof.UnitData, ucv))

	// verify that frc bill is created and its balance is equal to frcAmount - "transfer transaction cost" - "add transaction cost"
	var feeBillState fcsdk.FeeCreditRecord
	require.NoError(t, unitAndProof.UnmarshalUnitData(&feeBillState))
	remainingFeeBalance := fcrAmount - transferFCRecord.ServerMetadata.ActualFee - addTxRecord.ServerMetadata.ActualFee
	require.Equal(t, remainingFeeBalance, feeBillState.Balance)

	// transfer initial bill to pubKey1
	transferInitialBillTx, _, _ := createBillTransfer(t, ib.ID, fcrID, total-fcrAmount, templates.NewP2pkh256BytesFromKeyHash(decodeAndHashHex(pubKey1)), 1)
	require.NoError(t, moneyPrt.SubmitTx(transferInitialBillTx))
	txRecord, _, err := testpartition.WaitTxProof(t, moneyPrt, transferInitialBillTx)
	require.NoError(t, err, "transfer initial bill failed")
	unitAndProof, err = testpartition.WaitUnitProof(t, moneyPrt, fcrID, transferInitialBillTx)
	require.NoError(t, err)
	require.NoError(t, types.VerifyUnitStateProof(unitAndProof.Proof, crypto.SHA256, unitAndProof.UnitData, ucv))
	require.NoError(t, unitAndProof.UnmarshalUnitData(&feeBillState))
	remainingFeeBalance = remainingFeeBalance - txRecord.ServerMetadata.GetActualFee()
	require.Equal(t, remainingFeeBalance, feeBillState.Balance)

	// split initial bill from pubKey1 to pubKey2
	amountPK2 := uint64(1000)
	targetUnit := &money.TargetUnit{Amount: amountPK2, OwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(decodeAndHashHex(pubKey2))}
	tx := createSplitTx(t, ib.ID, fcrID, 2, []*money.TargetUnit{targetUnit})
	require.NoError(t, moneyPrt.SubmitTx(tx))
	txRecord, _, err = testpartition.WaitTxProof(t, moneyPrt, tx)
	require.NoError(t, err, "money split transaction failed")
	unitAndProof, err = testpartition.WaitUnitProof(t, moneyPrt, fcrID, tx)
	require.NoError(t, err)
	require.NoError(t, types.VerifyUnitStateProof(unitAndProof.Proof, crypto.SHA256, unitAndProof.UnitData, ucv))
	require.NoError(t, unitAndProof.UnmarshalUnitData(&feeBillState))
	remainingFeeBalance = remainingFeeBalance - txRecord.ServerMetadata.GetActualFee()
	require.EqualValues(t, remainingFeeBalance, feeBillState.Balance)

	// wrong partition tx
	tx = createSplitTx(t, ib.ID, fcrID, 3, []*money.TargetUnit{targetUnit})
	tx.Payload.SystemID = 0x01010101
	require.ErrorContains(t, moneyPrt.SubmitTx(tx), "invalid transaction system identifier")
	// and fee unit is not changed
	feeCredit, err := s.GetUnit(fcrID, true)
	require.NoError(t, err)
	require.Equal(t, remainingFeeBalance, feeCredit.Data().(*fcsdk.FeeCreditRecord).Balance)

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
			ID:    money.NewBillID(nil, []byte{1}),
			Value: moneyInvariant,
			Owner: templates.AlwaysTrueBytes(),
		}
	)
	pdr := types.PartitionDescriptionRecord{
		SystemIdentifier: money.DefaultSystemID,
		TypeIdLen:        8,
		UnitIdLen:        256,
		T2Timeout:        2000 * time.Millisecond,
	}
	total := moneyInvariant
	sdrs := createSDRs(newBillID(99))
	txsState = genesisState(t, initialBill, sdrs)
	moneyPrt, err := testpartition.NewPartition(t, 3, func(tb types.RootTrustBase) txsystem.TransactionSystem {
		txsState = txsState.Clone()
		system, err := NewTxSystem(
			observability.Default(t),
			WithSystemIdentifier(pdr.SystemIdentifier),
			WithHashAlgorithm(crypto.SHA256),
			WithPartitionDescriptionRecords(sdrs),
			WithTrustBase(tb),
			WithState(txsState),
		)
		require.NoError(t, err)
		return system
	}, pdr, txsState)
	require.NoError(t, err)
	abNet, err := testpartition.NewAlphabillPartition([]*testpartition.NodePartition{moneyPrt})
	require.NoError(t, err)
	require.NoError(t, abNet.Start(t))
	defer abNet.WaitClose(t)

	// create fee credit for initial bill transfer
	signer, _ := testsig.CreateSignerAndVerifier(t)
	fcrID := testutils.NewFeeCreditRecordIDAlwaysTrue()
	transferFC := testutils.NewTransferFC(t, signer,
		testutils.NewTransferFCAttr(t, signer,
			testutils.WithCounter(0),
			testutils.WithAmount(fcrAmount),
			testutils.WithTargetRecordID(fcrID),
		),
		testtransaction.WithUnitID(initialBill.ID),

		testtransaction.WithPayloadType(fcsdk.PayloadTypeTransferFeeCredit),
	)
	require.NoError(t, moneyPrt.SubmitTx(transferFC))
	transferFCRecord, transferFCProof, err := testpartition.WaitTxProof(t, moneyPrt, transferFC)
	require.NoError(t, err, "transfer fee credit transaction failed")
	// check that frcAmount is credited from initial bill
	bill, err := txsState.GetUnit(initialBill.ID, false)
	require.NoError(t, err)
	require.Equal(t, moneyInvariant-fcrAmount, bill.Data().(*money.BillData).V)
	// send addFC
	addFC := testutils.NewAddFC(t, signer,
		testutils.NewAddFCAttr(t, signer,
			testutils.WithTransferFCRecord(transferFCRecord),
			testutils.WithTransferFCProof(transferFCProof),
			testutils.WithFeeCreditOwnerPredicate(templates.AlwaysTrueBytes()),
		),
		testtransaction.WithUnitID(fcrID),
		testtransaction.WithPayloadType(fcsdk.PayloadTypeAddFeeCredit),
	)
	require.NoError(t, moneyPrt.SubmitTx(addFC))
	// before reading state make sure that node 2 has executed the transfer
	addTxRecord, _, err := testpartition.WaitTxProof(t, moneyPrt, addFC)
	require.NoError(t, err, "add fee credit transaction failed")
	// verify that frc bill is created and its balance is equal to frcAmount - "transfer transaction cost" - "add transaction cost"
	feeCredit, err := txsState.GetUnit(fcrID, false)
	require.NoError(t, err)
	require.Equal(t, fcrAmount-transferFCRecord.ServerMetadata.ActualFee-addTxRecord.ServerMetadata.ActualFee, feeCredit.Data().(*fcsdk.FeeCreditRecord).Balance)

	// transfer initial bill to pubKey1
	transferInitialBillTx, _, _ := createBillTransfer(t, initialBill.ID, fcrID, total-fcrAmount, templates.NewP2pkh256BytesFromKeyHash(decodeAndHashHex(pubKey1)), 1)
	require.NoError(t, moneyPrt.SubmitTx(transferInitialBillTx))
	// wait for transaction to be added to block
	txRecord, _, err := testpartition.WaitTxProof(t, moneyPrt, transferInitialBillTx)
	require.NoError(t, err, "transfer initial bill failed")
	require.EqualValues(t, transferInitialBillTx, txRecord.TransactionOrder)
	feeCredit, err = txsState.GetUnit(fcrID, false)
	require.NoError(t, err)
	require.Equal(t, fcrAmount-3, feeCredit.Data().(*fcsdk.FeeCreditRecord).Balance)

	// split initial bill using N-way split where N=nofDustToSwap
	amount := uint64(1)
	_, _, _, err = moneyPrt.GetTxProof(transferInitialBillTx)
	require.NoError(t, err)
	total -= fcrAmount

	var targetUnits []*money.TargetUnit
	for i := 0; i < nofDustToSwap; i++ {
		targetUnits = append(targetUnits, &money.TargetUnit{Amount: amount, OwnerPredicate: templates.NewP2pkh256BytesFromKeyHash(decodeAndHashHex(pubKey2))})
		total -= amount
		amount++
	}
	splitTx := createSplitTx(t, initialBill.ID, fcrID, 2, targetUnits)
	require.NoError(t, moneyPrt.SubmitTx(splitTx))

	// wait for transaction to be added to block
	txRecord, _, err = testpartition.WaitTxProof(t, moneyPrt, splitTx)
	require.NoError(t, err, "money split transaction failed")
	require.EqualValues(t, splitTx, txRecord.TransactionOrder)

	// create dust payments from splits
	dcBillIds := make([]types.UnitID, nofDustToSwap)
	for i := 0; i < nofDustToSwap; i++ {
		dcBillIds[i] = money.NewBillID(nil, splitTx.HashForNewUnitID(crypto.SHA256, util.Uint32ToBytes(uint32(i))))
	}
	// sort bill id's
	sort.Slice(dcBillIds, func(i, j int) bool {
		return bytes.Compare(dcBillIds[i], dcBillIds[j]) == -1
	})
	dcTxs, sum := createDustTransferTxs(t, initialBill.ID, 3, fcrID, dcBillIds, txsState)
	dcRecords := make([]*types.TransactionRecord, len(dcTxs))
	dcRecordsProofs := make([]*types.TxProof, len(dcTxs))
	for i, dcTx := range dcTxs {
		require.NoError(t, moneyPrt.SubmitTx(dcTx))
		dcRecords[i], dcRecordsProofs[i], err = testpartition.WaitTxProof(t, moneyPrt, dcTx)
		require.NoError(t, err, "dc transaction failed")
	}

	// create swap order
	swapAttr := &money.SwapDCAttributes{
		DcTransfers:      dcRecords,
		DcTransferProofs: dcRecordsProofs,
		TargetValue:      sum,
	}
	swapBytes, err := types.Cbor.Marshal(swapAttr)
	require.NoError(t, err)

	// create swap tx
	swapTx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   pdr.SystemIdentifier,
			Type:       money.PayloadTypeSwapDC,
			UnitID:     initialBill.ID,
			Attributes: swapBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           20,
				MaxTransactionFee: 10,
				FeeCreditRecordID: fcrID,
			},
		},
		FeeProof: templates.EmptyArgument(),
	}
	signer, err = abcrypto.NewInMemorySecp256K1SignerFromKey(decodeHex(privKey1))
	require.NoError(t, err)
	ownerProof := testsig.NewOwnerProof(t, swapTx, signer)
	authProof := fcsdk.AddFeeCreditAuthProof{OwnerProof: ownerProof}
	require.NoError(t, swapTx.SetAuthProof(authProof))

	require.NoError(t, moneyPrt.SubmitTx(swapTx))
	_, _, err = testpartition.WaitTxProof(t, moneyPrt, swapTx)
	require.NoError(t, err)
	for _, n := range moneyPrt.Nodes {
		testevent.NotContainsEvent(t, n.EventHandler, event.RecoveryStarted)
	}
}

func createSplitTx(t *testing.T, fromID []byte, fcrID types.UnitID, counter uint64, targetUnits []*money.TargetUnit) *types.TransactionOrder {
	tx, _, _ := createSplit(t, fromID, fcrID, targetUnits, counter)
	signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(decodeHex(privKey1))
	require.NoError(t, err)

	ownerProof := testsig.NewOwnerProof(t, tx, signer)
	authProof := money.SplitAuthProof{OwnerProof: ownerProof}
	require.NoError(t, tx.SetAuthProof(authProof))

	tx.FeeProof = templates.EmptyArgument() // default fee credit record has "always true" predicate
	return tx
}

func createDustTransferTxs(t *testing.T, targetID []byte, targetCounter uint64, fcrID types.UnitID, ids []types.UnitID, s *state.State) ([]*types.TransactionOrder, uint64) {
	t.Helper()

	// create dc transfers
	dcTransfers := make([]*types.TransactionOrder, len(ids))

	var targetValue uint64 = 0
	for i, id := range ids {
		_, billData := getBill(t, s, id)
		targetValue += billData.V
		tx, _, _ := createDCTransfer(t, id, fcrID, billData.V, billData.Counter, targetID, targetCounter)
		signer, err := abcrypto.NewInMemorySecp256K1SignerFromKey(decodeHex(privKey2))
		require.NoError(t, err)

		ownerProof := testsig.NewOwnerProof(t, tx, signer)
		authProof := money.TransferDCAuthProof{OwnerProof: ownerProof}
		require.NoError(t, tx.SetAuthProof(authProof))

		tx.FeeProof = templates.EmptyArgument() // default fee credit record has "always true" predicate
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
	return money.NewBillID(nil, []byte{unitPart})
}
