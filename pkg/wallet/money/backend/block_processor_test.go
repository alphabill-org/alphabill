package backend

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var moneySystemID = []byte{0, 0, 0, 0}

func TestBlockProcessor_EachTxTypeCanBeProcessed(t *testing.T) {
	pubKeyBytes, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubKeyHash := hash.Sum256(pubKeyBytes)
	fcbID := newUnitID(101)
	fcb := &Bill{
		Id:            fcbID,
		Value:         100,
		FCBlockNumber: 1,
	}
	signer, _ := crypto.NewInMemorySecp256K1Signer()
	tx1 := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       moneySystemID,
				Type:           money.PayloadTypeTransfer,
				UnitID:         newUnitID(1),
				Attributes:     transferTxAttr(pubKeyHash),
				ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
			},
		},
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}
	tx2 := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       moneySystemID,
				Type:           money.PayloadTypeTransDC,
				UnitID:         newUnitID(2),
				Attributes:     dustTxAttr(pubKeyHash),
				ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
			},
		},
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}
	tx3 := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       moneySystemID,
				Type:           money.PayloadTypeSplit,
				UnitID:         newUnitID(3),
				Attributes:     splitTxAttr(pubKeyHash, 1, 1),
				ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
			},
		},
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}
	tx4 := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       moneySystemID,
				Type:           money.PayloadTypeSwapDC,
				UnitID:         newUnitID(4),
				Attributes:     swapTxAttr(pubKeyHash),
				ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
			},
		},
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}

	b := &types.Block{
		Header:             &types.Header{SystemID: moneySystemID},
		Transactions:       []*types.TransactionRecord{tx1, tx2, tx3, tx4},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
	}

	store, err := createTestBillStore(t)
	require.NoError(t, err)
	err = store.Do().SetFeeCreditBill(fcb)
	require.NoError(t, err)
	err = store.Do().SetSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: moneySystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         util.Uint256ToBytes(uint256.NewInt(2)),
				OwnerPredicate: script.PredicateAlwaysTrue(),
			},
		},
	})
	require.NoError(t, err)
	blockProcessor, err := NewBlockProcessor(store, moneySystemID)
	require.NoError(t, err)

	// process transactions
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify bills exist
	ownerCondition := script.PredicatePayToPublicKeyHashDefault(pubKeyHash)
	bills, err := store.Do().GetBills(ownerCondition)
	require.NoError(t, err)
	require.Len(t, bills, 4)
	for _, bill := range bills {
		verifyProof(t, bill)
	}

	// verify tx2 is dcBill
	bill, err := store.Do().GetBill(tx2.TransactionOrder.UnitID())
	require.NoError(t, err)
	require.True(t, bill.IsDCBill)

	// verify fcb is reduced by 4x txFee
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 96, fcb.Value)

	// process transferFC + addFC
	transferFCAttr := testfc.NewTransferFCAttr(
		testfc.WithTargetRecordID(fcbID),
		testfc.WithAmount(99), // tx1 unit value minus fee
	)
	transferFC := testfc.NewTransferFC(t, transferFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(tx1.TransactionOrder.UnitID()),
	)
	transferFCRecord := &types.TransactionRecord{
		TransactionOrder: transferFC,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}

	addFCAttr := testfc.NewAddFCAttr(t, signer,
		testfc.WithTransferFCTx(transferFCRecord),
	)
	addFC := testfc.NewAddFC(t, signer, addFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(fcbID),
	)
	addFCRecord := &types.TransactionRecord{
		TransactionOrder: addFC,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}

	b = &types.Block{
		Transactions:       []*types.TransactionRecord{transferFCRecord, addFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 2}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify fee credit bill value (96) is incremented by transferFC value (99) minus txfee (1)
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 194, fcb.Value)

	// verify tx1 unit is deleted (zero value bills are not allowed)
	unit1, err := store.Do().GetBill(tx1.TransactionOrder.UnitID())
	require.NoError(t, err)
	require.Nil(t, unit1)

	// process closeFC + reclaimFC (reclaim all credits to bill no 4)
	closeFCAttr := testfc.NewCloseFCAttr(
		testfc.WithCloseFCAmount(194),
		testfc.WithCloseFCTargetUnitID(tx4.TransactionOrder.UnitID()),
	)
	closeFC := testfc.NewCloseFC(t, closeFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(fcbID),
	)
	closeFCRecord := &types.TransactionRecord{TransactionOrder: closeFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}

	reclaimFCAttr := testfc.NewReclaimFCAttr(t, signer,
		testfc.WithReclaimFCClosureTx(closeFCRecord),
	)
	reclaimFC := testfc.NewReclaimFC(t, signer, reclaimFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(tx4.TransactionOrder.UnitID()),
	)
	reclaimFCRecord := &types.TransactionRecord{
		TransactionOrder: reclaimFC,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}

	b = &types.Block{
		Transactions:       []*types.TransactionRecord{closeFCRecord, reclaimFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 3}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify FCB is reduced to zero
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 0, fcb.Value)

	// verify reclaimed fee credits (194) were added to specified unit (tx4 value=100) minus 2x txfee (2)
	unit, err := store.Do().GetBill(tx4.TransactionOrder.UnitID())
	require.NoError(t, err)
	require.EqualValues(t, 292, unit.Value)
}

func TestBlockProcessor_TransferFCAndReclaimFC(t *testing.T) {
	fcbID := newUnitID(101)
	fcb := &Bill{
		Id:            fcbID,
		Value:         50,
		FCBlockNumber: 1,
	}
	signer, _ := crypto.NewInMemorySecp256K1Signer()

	store, err := createTestBillStore(t)
	require.NoError(t, err)

	err = store.Do().SetFeeCreditBill(fcb)
	require.NoError(t, err)

	userBillID := []byte{1}
	err = store.Do().SetBill(&Bill{
		Id:             userBillID,
		Value:          100,
		TxHash:         []byte{2},
		OwnerPredicate: script.PredicateAlwaysTrue(),
	})
	require.NoError(t, err)

	moneyPartitionFeeBillID := util.Uint256ToBytes(uint256.NewInt(2))
	err = store.Do().SetSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: moneySystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         moneyPartitionFeeBillID,
				OwnerPredicate: script.PredicateAlwaysTrue(),
			},
		},
	})
	require.NoError(t, err)

	err = store.Do().SetBill(&Bill{
		Id:             moneyPartitionFeeBillID,
		OwnerPredicate: script.PredicateAlwaysTrue(),
		Value:          0,
	})
	require.NoError(t, err)

	blockProcessor, err := NewBlockProcessor(store, moneySystemID)
	require.NoError(t, err)

	// process transferFC of 50 billy from userBillID
	transferFCAttr := testfc.NewTransferFCAttr(
		testfc.WithTargetRecordID(fcbID),
		testfc.WithTargetSystemID(moneySystemID),
		testfc.WithAmount(50),
	)
	transferFC := testfc.NewTransferFC(t, transferFCAttr,
		testtransaction.WithUnitId(userBillID),
		testtransaction.WithSystemID(moneySystemID),
	)
	transferFCRecord := &types.TransactionRecord{TransactionOrder: transferFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}
	b := &types.Block{
		Transactions:       []*types.TransactionRecord{transferFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify money partition fee bill value (0) is increased by transferFC amount (50) + transfer fee amount (1)
	moneyPartitionFeeBill, err := store.Do().GetBill(moneyPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 51, moneyPartitionFeeBill.Value)

	// process closeFC + reclaimFC (reclaim all credits)
	closeFCAttr := testfc.NewCloseFCAttr(
		testfc.WithCloseFCAmount(30),
		testfc.WithCloseFCTargetUnitID(transferFC.UnitID()),
	)
	closeFC := testfc.NewCloseFC(t, closeFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(fcbID),
	)
	closeFCRecord := &types.TransactionRecord{TransactionOrder: closeFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}

	reclaimFCAttr := testfc.NewReclaimFCAttr(t, signer,
		testfc.WithReclaimFCClosureTx(closeFCRecord),
	)
	reclaimFC := testfc.NewReclaimFC(t, signer, reclaimFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(transferFC.UnitID()),
	)
	reclaimFCRecord := &types.TransactionRecord{TransactionOrder: reclaimFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}

	b = &types.Block{
		Transactions:       []*types.TransactionRecord{closeFCRecord, reclaimFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 3}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify money partition fee bill value (51) is decreased by closeFC amount (30) and increased by closeFC fee amount (1) and reclaimFC fee amount(1)
	moneyPartitionFeeBill, err = store.Do().GetBill(moneyPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 23, moneyPartitionFeeBill.Value)
}

func verifyProof(t *testing.T, b *Bill) {
	require.NotNil(t, b)
	txProof := b.TxProof
	require.NotNil(t, txProof)
	require.NotNil(t, txProof.TxRecord)

	p := txProof.TxProof
	require.EqualValues(t, 1, p.UnicityCertificate.GetRoundNumber())
	require.NotNil(t, p)
	require.NotNil(t, p.BlockHeaderHash)
	require.NotNil(t, p.Chain)
	require.NotNil(t, p.UnicityCertificate)
}

func newUnitID(unitID uint64) []byte {
	return util.Uint256ToBytes(uint256.NewInt(unitID))
}

func transferTxAttr(pubKeyHash []byte) []byte {
	attr := &money.TransferAttributes{
		TargetValue: 100,
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		Backlink:    hash.Sum256([]byte{}),
	}
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}

func dustTxAttr(pubKeyHash []byte) []byte {
	attr := &money.TransferDCAttributes{
		TargetValue:  100,
		TargetBearer: script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		Backlink:     hash.Sum256([]byte{}),
	}
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}

func splitTxAttr(pubKeyHash []byte, amount uint64, remainingValue uint64) []byte {
	attr := &money.SplitAttributes{
		Amount:         amount,
		TargetBearer:   script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		RemainingValue: remainingValue,
		Backlink:       hash.Sum256([]byte{}),
	}
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}

func swapTxAttr(pubKeyHash []byte) []byte {
	attr := &money.SwapDCAttributes{
		OwnerCondition:  script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		BillIdentifiers: [][]byte{},
		DcTransfers:     []*types.TransactionRecord{},
		Proofs:          []*types.TxProof{},
		TargetValue:     100,
	}
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}
