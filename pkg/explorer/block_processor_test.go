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
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

var moneySystemID = money.DefaultSystemIdentifier
var tokenSystemID = tokens.DefaultSystemIdentifier

func TestBlockProcessor_EachTxTypeCanBeProcessed(t *testing.T) {
	pubKeyBytes, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubKeyHash := hash.Sum256(pubKeyBytes)
	fcbID := newFeeCreditRecordID(101)
	//fcb := &Bill{Id: fcbID, Value: 100}
	signer, _ := crypto.NewInMemorySecp256K1Signer()
	ownerCondition := script.PredicatePayToPublicKeyHashDefault(pubKeyHash)
	tx1 := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       moneySystemID,
				Type:           money.PayloadTypeTransfer,
				UnitID:         newBillID(1),
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
				UnitID:         newBillID(2),
				Attributes:     dustTxAttr(),
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
				UnitID:         newBillID(3),
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
				UnitID:         newBillID(4),
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

	store := createTestBillStore(t)
	//err := store.Do().SetFeeCreditBill(fcb, nil)
	//require.NoError(t, err)
	// store existing bill for dc transfer and swap transfer
	err := store.Do().SetBill(&Bill{Id: tx2.TransactionOrder.UnitID(), OwnerPredicate: ownerCondition}, nil)
	require.NoError(t, err)
	err = store.Do().SetBill(&Bill{Id: tx4.TransactionOrder.UnitID(), OwnerPredicate: ownerCondition}, nil)
	require.NoError(t, err)
	err = store.Do().SetSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: moneySystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         money.NewBillID(nil, []byte{2}),
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
	bills, nextKey, err := store.Do().GetBills(ownerCondition, true, nil, 100)
	require.NoError(t, err)
	require.Nil(t, nextKey)
	require.Len(t, bills, 4)
	for _, bill := range bills {
		proof, err := store.Do().GetTxProof(bill.Id, bill.TxHash)
		require.NoError(t, err)
		verifyProof(t, bill, proof)
	}

	// verify tx2 is dcBill
	bill, err := store.Do().GetBill(tx2.TransactionOrder.UnitID())
	require.NoError(t, err)
	require.NotNil(t, bill.DCTargetUnitID)

	// verify fcb is reduced by 4x txFee
	//fcb, err = store.Do().GetFeeCreditBill(fcbID)
	//require.NoError(t, err)
	//require.EqualValues(t, 96, fcb.Value)

	// process transferFC + addFC
	transferFCAttr := testfc.NewTransferFCAttr(
		testfc.WithTargetRecordID(fcbID),
		testfc.WithAmount(100), // tx1 unit value
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

	// verify fee credit bill value (96) is incremented by transferFC value (100) minus transferFC and addFC txfee (1 + 1)
	//fcb, err = store.Do().GetFeeCreditBill(fcbID)
	//require.NoError(t, err)
	//require.EqualValues(t, 194, fcb.Value)

	// verify FCB LastAddFCTxHash equals to TxHash
	//lastAddFCTxHash := addFC.Hash(gocrypto.SHA256)
	//require.Equal(t, lastAddFCTxHash, fcb.TxHash)
	//require.Equal(t, lastAddFCTxHash, fcb.LastAddFCTxHash)

	// verify tx1 unit is deleted (whole bill transferred to fee credit)
	//unit1, err := store.Do().GetBill(tx1.TransactionOrder.UnitID())
	//require.NoError(t, err)
	//require.Nil(t, unit1)
	//
	//// process closeFC + reclaimFC (reclaim all credits to bill no 4)
	//closeFCAttr := testfc.NewCloseFCAttr(
	//	testfc.WithCloseFCAmount(194),
	//	testfc.WithCloseFCTargetUnitID(tx4.TransactionOrder.UnitID()),
	//)
	//closeFC := testfc.NewCloseFC(t, closeFCAttr,
	//	testtransaction.WithSystemID(moneySystemID),
	//	testtransaction.WithUnitId(fcbID),
	//)
	//closeFCRecord := &types.TransactionRecord{TransactionOrder: closeFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}
	//
	//reclaimFCAttr := testfc.NewReclaimFCAttr(t, signer,
	//	testfc.WithReclaimFCClosureTx(closeFCRecord),
	//)
	//reclaimFC := testfc.NewReclaimFC(t, signer, reclaimFCAttr,
	//	testtransaction.WithSystemID(moneySystemID),
	//	testtransaction.WithUnitId(tx4.TransactionOrder.UnitID()),
	//)
	//reclaimFCRecord := &types.TransactionRecord{
	//	TransactionOrder: reclaimFC,
	//	ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	//}
	//
	//b = &types.Block{
	//	Transactions:       []*types.TransactionRecord{closeFCRecord, reclaimFCRecord},
	//	UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 3}},
	//}
	//err = blockProcessor.ProcessBlock(context.Background(), b)
	//require.NoError(t, err)
	//
	//// verify FCB is reduced to zero
	////fcb, err = store.Do().GetFeeCreditBill(fcbID)
	////require.NoError(t, err)
	////require.EqualValues(t, 0, fcb.Value)
	//
	//// verify FCB LastAddFCTxHash is not changed
	////require.Equal(t, lastAddFCTxHash, fcb.LastAddFCTxHash)
	//
	//// verify reclaimed fee credits (194) were added to specified unit (tx4 value=100) minus 2x txfee (2)
	//unit, err := store.Do().GetBill(tx4.TransactionOrder.UnitID())
	//require.NoError(t, err)
	//require.EqualValues(t, 292, unit.Value)
	//
	//// verify zero value bill is deleted at first block after blocknumber+DustBillDeletionTimeout
	//b = &types.Block{
	//	UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: ExpiredBillDeletionTimeout + b.GetRoundNumber()}},
	//	Transactions:       []*types.TransactionRecord{},
	//}
	//err = blockProcessor.ProcessBlock(context.Background(), b)
	//require.NoError(t, err)
	//
	//unit1, err = store.Do().GetBill(tx1.TransactionOrder.UnitID())
	//require.NoError(t, err)
	//require.Nil(t, unit1)
}

func verifyProof(t *testing.T, b *Bill, txProof *sdk.Proof) {
	require.NotNil(t, b)
	require.NotNil(t, txProof)
	require.NotNil(t, txProof.TxRecord)

	p := txProof.TxProof
	require.EqualValues(t, 1, p.UnicityCertificate.GetRoundNumber())
	require.NotNil(t, p)
	require.NotNil(t, p.BlockHeaderHash)
	require.NotNil(t, p.Chain)
	require.NotNil(t, p.UnicityCertificate)
}

func newBillID(unitPart byte) []byte {
	return money.NewBillID(nil, []byte{unitPart})
}

func newFeeCreditRecordID(unitPart byte) []byte {
	return money.NewFeeCreditRecordID(nil, []byte{unitPart})
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

func dustTxAttr() []byte {
	attr := &money.TransferDCAttributes{
		Value:              100,
		Backlink:           hash.Sum256([]byte{}),
		TargetUnitID:       []byte{0},
		TargetUnitBacklink: []byte{1},
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
		OwnerCondition:   script.PredicatePayToPublicKeyHashDefault(pubKeyHash),
		DcTransfers:      []*types.TransactionRecord{},
		DcTransferProofs: []*types.TxProof{},
		TargetValue:      100,
	}
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}
