package backend

import (
	"context"
	gocrypto "crypto"
	"testing"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/internal/testutils/logger"
	"github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/predicates/templates"
	"github.com/alphabill-org/alphabill/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/txsystem/fc/transactions"
	"github.com/alphabill-org/alphabill/txsystem/money"
	"github.com/alphabill-org/alphabill/txsystem/tokens"
	"github.com/alphabill-org/alphabill/types"
	"github.com/alphabill-org/alphabill/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	sdk "github.com/alphabill-org/alphabill/wallet"
)

var moneySystemID = money.DefaultSystemIdentifier
var tokenSystemID = tokens.DefaultSystemIdentifier

func TestBlockProcessor_EachTxTypeCanBeProcessed(t *testing.T) {
	pubKeyBytes, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubKeyHash := util.Sum256(pubKeyBytes)
	fcbID := newFeeCreditRecordID(101)
	fcb := &Bill{Id: fcbID, Value: 100}
	signer, _ := crypto.NewInMemorySecp256K1Signer()
	ownerCondition := templates.NewP2pkh256BytesFromKeyHash(pubKeyHash)
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
				SystemID: moneySystemID,
				Type:     money.PayloadTypeSplit,
				UnitID:   newBillID(3),
				Attributes: splitTxAttr(1,
					&money.TargetUnit{Amount: 1, OwnerCondition: templates.NewP2pkh256BytesFromKeyHash(pubKeyHash)},
				),
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
	err := store.Do().SetFeeCreditBill(fcb, nil)
	require.NoError(t, err)
	// store existing bill for dc transfer and swap transfer
	err = store.Do().SetBill(&Bill{Id: tx2.TransactionOrder.UnitID(), OwnerPredicate: ownerCondition}, nil)
	require.NoError(t, err)
	err = store.Do().SetBill(&Bill{Id: tx4.TransactionOrder.UnitID(), OwnerPredicate: ownerCondition}, nil)
	require.NoError(t, err)
	err = store.Do().SetSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: moneySystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         money.NewBillID(nil, []byte{2}),
				OwnerPredicate: templates.AlwaysTrueBytes(),
			},
		},
	})
	require.NoError(t, err)
	blockProcessor, err := NewBlockProcessor(store, moneySystemID, logger.New(t))
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
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 96, fcb.Value)

	// process transferFC + addFC
	transferFCAttr := testutils.NewTransferFCAttr(
		testutils.WithTargetRecordID(fcbID),
		testutils.WithAmount(100), // tx1 unit value
	)
	transferFC := testutils.NewTransferFC(t, transferFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(tx1.TransactionOrder.UnitID()),
	)
	transferFCRecord := &types.TransactionRecord{
		TransactionOrder: transferFC,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}

	addFCAttr := testutils.NewAddFCAttr(t, signer,
		testutils.WithTransferFCTx(transferFCRecord),
	)
	addFC := testutils.NewAddFC(t, signer, addFCAttr,
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
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 194, fcb.Value)

	// verify txHash
	require.Equal(t, addFC.Hash(gocrypto.SHA256), fcb.TxHash)

	// verify tx1 unit is deleted (whole bill transferred to fee credit)
	unit1, err := store.Do().GetBill(tx1.TransactionOrder.UnitID())
	require.NoError(t, err)
	require.Nil(t, unit1)

	// process closeFC + reclaimFC (reclaim all credits to bill no 4)
	closeFCAttr := testutils.NewCloseFCAttr(
		testutils.WithCloseFCAmount(194),
		testutils.WithCloseFCTargetUnitID(tx4.TransactionOrder.UnitID()),
	)
	closeFC := testutils.NewCloseFC(t, closeFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(fcbID),
	)
	closeFCRecord := &types.TransactionRecord{TransactionOrder: closeFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}

	reclaimFCAttr := testutils.NewReclaimFCAttr(t, signer,
		testutils.WithReclaimFCClosureTx(closeFCRecord),
	)
	reclaimFC := testutils.NewReclaimFC(t, signer, reclaimFCAttr,
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

	// verify txHash
	require.Equal(t, closeFC.Hash(gocrypto.SHA256), fcb.TxHash)

	// verify reclaimed fee credits (194) were added to specified unit (tx4 value=100) minus 2x txfee (2)
	unit, err := store.Do().GetBill(tx4.TransactionOrder.UnitID())
	require.NoError(t, err)
	require.EqualValues(t, 292, unit.Value)

	// verify zero value bill is deleted at first block after blocknumber+DustBillDeletionTimeout
	b = &types.Block{
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: ExpiredBillDeletionTimeout + b.GetRoundNumber()}},
		Transactions:       []*types.TransactionRecord{},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	unit1, err = store.Do().GetBill(tx1.TransactionOrder.UnitID())
	require.NoError(t, err)
	require.Nil(t, unit1)
}

// start state
// user bill: 100
//
// add 50 fee credit and reclaim
//
// end state
// user bill: 96
// money partition fee bill: 4
func TestBlockProcessor_TransferAndReclaimFeeCycle_TargetMoneyPartition(t *testing.T) {
	fcbID := newFeeCreditRecordID(101)
	signer, _ := crypto.NewInMemorySecp256K1Signer()
	store := createTestBillStore(t)
	userBillID := []byte{1}
	err := store.Do().SetBill(&Bill{
		Id:             userBillID,
		Value:          100,
		TxHash:         []byte{2},
		OwnerPredicate: templates.AlwaysTrueBytes(),
	}, nil)
	require.NoError(t, err)

	moneyPartitionFeeBillID := money.NewBillID(nil, []byte{2})
	err = store.Do().SetSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: moneySystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         moneyPartitionFeeBillID,
				OwnerPredicate: templates.AlwaysTrueBytes(),
			},
		},
	})
	require.NoError(t, err)

	err = store.Do().SetBill(&Bill{
		Id:             moneyPartitionFeeBillID,
		OwnerPredicate: templates.AlwaysTrueBytes(),
		Value:          0,
	}, nil)
	require.NoError(t, err)

	blockProcessor, err := NewBlockProcessor(store, moneySystemID, logger.New(t))
	require.NoError(t, err)

	// process transferFC of 50 billy from userBillID
	transferFCAttr := testutils.NewTransferFCAttr(
		testutils.WithTargetRecordID(fcbID),
		testutils.WithTargetSystemID(moneySystemID),
		testutils.WithAmount(50),
	)
	transferFC := testutils.NewTransferFC(t, transferFCAttr,
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

	// verify money partition fee bill value (0) is increased by transferFC amount (50)
	moneyPartitionFeeBill, err := store.Do().GetBill(moneyPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 50, moneyPartitionFeeBill.Value)

	// and user bill (100) is decreased by transferFC.amount (50)
	userBill, err := store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 50, userBill.Value)

	// process addFC
	addFCOrder := testutils.NewAddFC(t, signer,
		testutils.NewAddFCAttr(t, signer, testutils.WithTransferFCTx(transferFCRecord)),
		testtransaction.WithUnitId(fcbID),
		testtransaction.WithSystemID(moneySystemID),
	)
	addFCRecord := &types.TransactionRecord{TransactionOrder: addFCOrder, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}
	b = &types.Block{
		Transactions:       []*types.TransactionRecord{addFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 2}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify money partition fee bill does not change
	moneyPartitionFeeBill, err = store.Do().GetBill(moneyPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 50, moneyPartitionFeeBill.Value)

	// and user bill does not change
	userBill, err = store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 50, userBill.Value)

	// and user fee credit bill is created by transferFC.amount (50) minus transfer fee(1) and add fee(1) = (48)
	fcb, err := store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 48, fcb.Value)

	// process closeFC
	closeFCAttr := testutils.NewCloseFCAttr(
		testutils.WithCloseFCAmount(48),
		testutils.WithCloseFCTargetUnitID(transferFC.UnitID()),
	)
	closeFC := testutils.NewCloseFC(t, closeFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(fcbID),
	)
	closeFCRecord := &types.TransactionRecord{TransactionOrder: closeFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}
	b = &types.Block{
		Transactions:       []*types.TransactionRecord{closeFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 3}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify money partition fee bill does not change
	moneyPartitionFeeBill, err = store.Do().GetBill(moneyPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 50, moneyPartitionFeeBill.Value)

	// and user bill does not change
	userBill, err = store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 50, userBill.Value)

	// and user fee credit bill is decreased by closeFC.amount (49)
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 0, fcb.Value)

	// process reclaimFC
	reclaimFCAttr := testutils.NewReclaimFCAttr(t, signer,
		testutils.WithReclaimFCClosureTx(closeFCRecord),
	)
	reclaimFC := testutils.NewReclaimFC(t, signer, reclaimFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(transferFC.UnitID()),
	)
	reclaimFCRecord := &types.TransactionRecord{TransactionOrder: reclaimFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}

	b = &types.Block{
		Transactions:       []*types.TransactionRecord{reclaimFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify money partition fee bill value (50) is decreased by closeFC amount (49) and increased by closeFC fee amount (1) and reclaimFC fee amount(1)
	moneyPartitionFeeBill, err = store.Do().GetBill(moneyPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 4, moneyPartitionFeeBill.Value)

	// verify user bill (49) is increased closeFC amount (49) and decreased by closeFC fee amount (1) and reclaimFC fee amount(1)
	// i.e. user spent total 4 tx worth of fees
	userBill, err = store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 96, userBill.Value)

	// and user fee credit bill is not changed
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 0, fcb.Value)
}

// start state
// user bill: 100
//
// end state
// user bill: 96
// token partition fee bill: 4
func TestBlockProcessor_TransferAndReclaimFeeCycle_TargetTokenPartition(t *testing.T) {
	fcbID := newFeeCreditRecordID(101)
	signer, _ := crypto.NewInMemorySecp256K1Signer()
	store := createTestBillStore(t)

	userBillID := []byte{1}
	err := store.Do().SetBill(&Bill{
		Id:             userBillID,
		Value:          100,
		TxHash:         []byte{2},
		OwnerPredicate: templates.AlwaysTrueBytes(),
	}, nil)
	require.NoError(t, err)

	moneyPartitionFeeBillID := money.NewBillID(nil, []byte{2})
	tokenPartitionFeeBillID := money.NewBillID(nil, []byte{3})
	err = store.Do().SetSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: moneySystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         moneyPartitionFeeBillID,
				OwnerPredicate: templates.AlwaysTrueBytes(),
			},
		},
		{
			SystemIdentifier: tokenSystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         tokenPartitionFeeBillID,
				OwnerPredicate: templates.AlwaysTrueBytes(),
			},
		},
	})
	require.NoError(t, err)

	err = store.Do().SetBill(&Bill{
		Id:             moneyPartitionFeeBillID,
		OwnerPredicate: templates.AlwaysTrueBytes(),
		Value:          0,
	}, nil)
	require.NoError(t, err)
	err = store.Do().SetBill(&Bill{
		Id:             tokenPartitionFeeBillID,
		OwnerPredicate: templates.AlwaysTrueBytes(),
		Value:          0,
	}, nil)
	require.NoError(t, err)

	blockProcessor, err := NewBlockProcessor(store, moneySystemID, logger.New(t))
	require.NoError(t, err)

	// process transferFC of 20 billy from userBillID
	transferFCAttr := testutils.NewTransferFCAttr(
		testutils.WithTargetRecordID(fcbID),
		testutils.WithTargetSystemID(tokenSystemID),
		testutils.WithAmount(20),
	)
	transferFC := testutils.NewTransferFC(t, transferFCAttr,
		testtransaction.WithUnitId(userBillID),
		testtransaction.WithSystemID(tokenSystemID),
	)
	transferFCRecord := &types.TransactionRecord{TransactionOrder: transferFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}
	b := &types.Block{
		Transactions:       []*types.TransactionRecord{transferFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify token partition fee bill value (0) is increased by transferFC amount (20) - transferFC fee (1)
	tokenPartitionFeeBill, err := store.Do().GetBill(tokenPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 19, tokenPartitionFeeBill.Value)

	// and money partition fee bill value (0) is increased by tx fee amount (1)
	moneyPartitionFeeBill, err := store.Do().GetBill(moneyPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 1, moneyPartitionFeeBill.Value)

	// and user bill (100) is decreased by transferFC.amount (20)
	userBill, err := store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 80, userBill.Value)

	// process addFC
	addFCOrder := testutils.NewAddFC(t, signer,
		testutils.NewAddFCAttr(t, signer, testutils.WithTransferFCTx(transferFCRecord)),
		testtransaction.WithUnitId(fcbID),
		testtransaction.WithSystemID(tokenSystemID),
	)
	addFCRecord := &types.TransactionRecord{TransactionOrder: addFCOrder, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}
	b = &types.Block{
		Transactions:       []*types.TransactionRecord{addFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 2}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify money partition fee bill does not change
	moneyPartitionFeeBill, err = store.Do().GetBill(moneyPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 1, moneyPartitionFeeBill.Value)

	// and token partition fee bill does not change
	tokenPartitionFeeBill, err = store.Do().GetBill(tokenPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 19, tokenPartitionFeeBill.Value)

	// and user bill does not change
	userBill, err = store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 80, userBill.Value)

	// and user fee credit bill is created by transferFC.amount (20) minus transfer fee(1) and add fee (1) = (18)
	fcb, err := store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 18, fcb.Value)

	// process closeFC
	closeFCAttr := testutils.NewCloseFCAttr(
		testutils.WithCloseFCAmount(18),
		testutils.WithCloseFCTargetUnitID(transferFC.UnitID()),
	)
	closeFC := testutils.NewCloseFC(t, closeFCAttr,
		testtransaction.WithSystemID(tokenSystemID),
		testtransaction.WithUnitId(fcbID),
	)
	closeFCRecord := &types.TransactionRecord{TransactionOrder: closeFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}
	b = &types.Block{
		Transactions:       []*types.TransactionRecord{closeFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 3}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify money partition fee bill does not change
	moneyPartitionFeeBill, err = store.Do().GetBill(moneyPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 1, moneyPartitionFeeBill.Value)

	// and token partition fee bill does not change
	tokenPartitionFeeBill, err = store.Do().GetBill(tokenPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 19, tokenPartitionFeeBill.Value)

	// and user bill does not change
	userBill, err = store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 80, userBill.Value)

	// and user fee credit bill (19) is decreased by closeFC.amount (19)
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 0, fcb.Value)

	// process reclaimFC
	reclaimFCAttr := testutils.NewReclaimFCAttr(t, signer,
		testutils.WithReclaimFCClosureTx(closeFCRecord),
	)
	reclaimFC := testutils.NewReclaimFC(t, signer, reclaimFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(transferFC.UnitID()),
	)
	reclaimFCRecord := &types.TransactionRecord{TransactionOrder: reclaimFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}

	b = &types.Block{
		Transactions:       []*types.TransactionRecord{reclaimFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify money partition fee bill is increased by reclaimFC fee
	moneyPartitionFeeBill, err = store.Do().GetBill(moneyPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 2, moneyPartitionFeeBill.Value)

	// verify token partition fee bill value (19) is decreased by closeFC amount (18) and increased by closeFC fee amount (1)
	tokenPartitionFeeBill, err = store.Do().GetBill(tokenPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 2, tokenPartitionFeeBill.Value)

	// verify user bill (79) is increased by closeFC amount (19) and decreased by closeFC fee amount (1) and reclaimFC fee amount(1)
	// i.e. user spent total 4 tx worth of fees
	userBill, err = store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 96, userBill.Value)

	// and user fee credit bill is not changed
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 0, fcb.Value)
}

func TestBlockProcessor_NWaySplit(t *testing.T) {
	store := createTestBillStore(t)
	fcbID := newFeeCreditRecordID(101)
	fcb := &Bill{Id: fcbID, Value: 100}

	var targetUnits []*money.TargetUnit
	for i := 1; i <= 5; i++ {
		pubKeyBytes := []byte{byte(i)}
		pubKeyHash := util.Sum256(pubKeyBytes)
		ownerCondition := templates.NewP2pkh256BytesFromKeyHash(pubKeyHash)
		targetUnits = append(targetUnits, &money.TargetUnit{
			Amount:         uint64(i),
			OwnerCondition: ownerCondition,
		})
	}
	tx := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       moneySystemID,
				Type:           money.PayloadTypeSplit,
				UnitID:         newBillID(3),
				Attributes:     splitTxAttr(1, targetUnits...),
				ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
			},
		},
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}
	b := &types.Block{
		Header:             &types.Header{SystemID: moneySystemID},
		Transactions:       []*types.TransactionRecord{tx},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
	}
	err := store.Do().SetFeeCreditBill(fcb, nil)
	require.NoError(t, err)

	// store existing bill for split
	err = store.Do().SetBill(&Bill{Id: tx.TransactionOrder.UnitID(), OwnerPredicate: templates.AlwaysTrueBytes()}, nil)
	require.NoError(t, err)
	err = store.Do().SetSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: moneySystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         money.NewBillID(nil, []byte{2}),
				OwnerPredicate: templates.AlwaysTrueBytes(),
			},
		},
	})
	require.NoError(t, err)
	blockProcessor, err := NewBlockProcessor(store, moneySystemID, logger.New(t))
	require.NoError(t, err)

	// process transactions
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify target bills were added
	for _, targetUnit := range targetUnits {
		bills, nextKey, err := store.Do().GetBills(targetUnit.OwnerCondition, true, nil, 100)
		require.NoError(t, err)
		require.Nil(t, nextKey)
		require.Len(t, bills, 1)
		require.Equal(t, targetUnit.Amount, bills[0].Value)
	}
}

func TestLockTx_Ok(t *testing.T) {
	store := createTestBillStore(t)
	blockProcessor, err := NewBlockProcessor(store, moneySystemID, logger.New(t))
	require.NoError(t, err)

	// store fee credit bill
	fcbID := newFeeCreditRecordID(101)
	fcb := &Bill{Id: fcbID, Value: 100}
	err = store.Do().SetFeeCreditBill(fcb, nil)
	require.NoError(t, err)

	// store existing bill for lock tx
	unitID := newBillID(3)
	err = store.Do().SetBill(&Bill{Id: unitID, OwnerPredicate: templates.AlwaysTrueBytes()}, nil)
	require.NoError(t, err)

	// store system description records
	err = store.Do().SetSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: moneySystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         money.NewBillID(nil, []byte{2}),
				OwnerPredicate: templates.AlwaysTrueBytes(),
			},
		},
	})
	require.NoError(t, err)

	// create lock tx
	tx := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       moneySystemID,
				Type:           money.PayloadTypeLock,
				UnitID:         unitID,
				Attributes:     marshalCbor(&money.LockAttributes{LockStatus: 1}),
				ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
			},
		},
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}

	// create block with lock tx
	b := &types.Block{
		Header:             &types.Header{SystemID: moneySystemID},
		Transactions:       []*types.TransactionRecord{tx},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
	}

	// process lock tx
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify bill was locked
	bill, err := store.Do().GetBill(unitID)
	require.NoError(t, err)
	require.NotNil(t, bill)
	require.EqualValues(t, 1, bill.Locked)
	require.EqualValues(t, tx.TransactionOrder.Hash(gocrypto.SHA256), bill.TxHash)

	// and fee credit bll value is reduced
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 99, fcb.Value)
}

func TestUnlockTx_Ok(t *testing.T) {
	store := createTestBillStore(t)
	blockProcessor, err := NewBlockProcessor(store, moneySystemID, logger.New(t))
	require.NoError(t, err)

	// store fee credit bill
	fcbID := newFeeCreditRecordID(101)
	fcb := &Bill{Id: fcbID, Value: 100}
	err = store.Do().SetFeeCreditBill(fcb, nil)
	require.NoError(t, err)

	// store existing locked bill for unlock tx
	unitID := newBillID(3)
	err = store.Do().SetBill(&Bill{Id: unitID, OwnerPredicate: templates.AlwaysTrueBytes(), Locked: 1}, nil)
	require.NoError(t, err)

	// create unlock tx
	tx := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       moneySystemID,
				Type:           money.PayloadTypeUnlock,
				UnitID:         unitID,
				Attributes:     marshalCbor(&money.UnlockAttributes{}),
				ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
			},
		},
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}

	// create block with unlock tx
	b := &types.Block{
		Header:             &types.Header{SystemID: moneySystemID},
		Transactions:       []*types.TransactionRecord{tx},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
	}

	// process block
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify bill was unlocked
	bill, err := store.Do().GetBill(unitID)
	require.NoError(t, err)
	require.NotNil(t, bill)
	require.EqualValues(t, 0, bill.Locked)
	require.EqualValues(t, tx.TransactionOrder.Hash(gocrypto.SHA256), bill.TxHash)

	// and fee credit bll value is reduced
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 99, fcb.Value)
}

func TestSwapUnlocksBill(t *testing.T) {
	store := createTestBillStore(t)
	blockProcessor, err := NewBlockProcessor(store, moneySystemID, logger.New(t))
	require.NoError(t, err)

	// store fee credit bill
	fcbID := newFeeCreditRecordID(101)
	fcb := &Bill{Id: fcbID, Value: 100}
	err = store.Do().SetFeeCreditBill(fcb, nil)
	require.NoError(t, err)

	// store existing locked bill for swap tx
	unitID := newBillID(3)
	err = store.Do().SetBill(&Bill{Id: unitID, OwnerPredicate: templates.AlwaysTrueBytes(), Locked: 1}, nil)
	require.NoError(t, err)

	// create swap tx
	tx := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:       moneySystemID,
				Type:           money.PayloadTypeSwapDC,
				UnitID:         unitID,
				Attributes:     swapTxAttr([]byte{1}),
				ClientMetadata: &types.ClientMetadata{FeeCreditRecordID: fcbID},
			},
		},
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}

	// create block with the swap tx
	b := &types.Block{
		Header:             &types.Header{SystemID: moneySystemID},
		Transactions:       []*types.TransactionRecord{tx},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
	}

	// process block
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify bill was unlocked
	bill, err := store.Do().GetBill(unitID)
	require.NoError(t, err)
	require.NotNil(t, bill)
	require.EqualValues(t, 0, bill.Locked)
}

func TestReclaimUnlocksBill(t *testing.T) {
	store := createTestBillStore(t)
	signer, _ := crypto.NewInMemorySecp256K1Signer()

	// store fee credit bill
	fcbID := newFeeCreditRecordID(101)
	fcb := &Bill{Id: fcbID, Value: 100}
	err := store.Do().SetFeeCreditBill(fcb, nil)
	require.NoError(t, err)

	// store existing locked bill for reclaim tx
	unitID := newBillID(3)
	err = store.Do().SetBill(&Bill{Id: unitID, OwnerPredicate: templates.AlwaysTrueBytes(), Locked: 1}, nil)
	require.NoError(t, err)

	// store system description records
	moneyPartitionFeeBill := &genesis.FeeCreditBill{
		UnitId:         money.NewBillID(nil, []byte{2}),
		OwnerPredicate: templates.AlwaysTrueBytes(),
	}
	err = store.Do().SetSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: moneySystemID,
			T2Timeout:        2500,
			FeeCreditBill:    moneyPartitionFeeBill,
		},
	})
	require.NoError(t, err)
	err = store.Do().SetBill(&Bill{Id: moneyPartitionFeeBill.UnitId, OwnerPredicate: moneyPartitionFeeBill.OwnerPredicate, Value: 100}, nil)
	require.NoError(t, err)

	// create block processor
	blockProcessor, err := NewBlockProcessor(store, moneySystemID, logger.New(t))
	require.NoError(t, err)

	// create block with reclaim tx
	closeFCAttr := testutils.NewCloseFCAttr(
		testutils.WithCloseFCAmount(194),
		testutils.WithCloseFCTargetUnitID(unitID),
	)
	closeFC := testutils.NewCloseFC(t, closeFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(fcbID),
	)
	closeFCRecord := &types.TransactionRecord{TransactionOrder: closeFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}

	reclaimFCAttr := testutils.NewReclaimFCAttr(t, signer,
		testutils.WithReclaimFCClosureTx(closeFCRecord),
	)
	reclaimFC := testutils.NewReclaimFC(t, signer, reclaimFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(unitID),
	)
	reclaimFCRecord := &types.TransactionRecord{
		TransactionOrder: reclaimFC,
		ServerMetadata:   &types.ServerMetadata{ActualFee: 1},
	}
	b := &types.Block{
		Transactions:       []*types.TransactionRecord{reclaimFCRecord},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 3}},
	}

	// process block
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify reclaimed target bill was unlocked
	bill, err := store.Do().GetBill(unitID)
	require.NoError(t, err)
	require.NotNil(t, bill)
	require.EqualValues(t, 0, bill.Locked)
}

func TestLockFC_Ok(t *testing.T) {
	store := createTestBillStore(t)

	// store existing fee credit bill for lockFC tx
	unitID := newBillID(3)
	err := store.Do().SetFeeCreditBill(&Bill{Id: unitID, Value: 100}, nil)
	require.NoError(t, err)

	// store system description records
	err = store.Do().SetSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: moneySystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         money.NewBillID(nil, []byte{2}),
				OwnerPredicate: templates.AlwaysTrueBytes(),
			},
		},
	})
	require.NoError(t, err)

	// create block processor
	blockProcessor, err := NewBlockProcessor(store, moneySystemID, logger.New(t))
	require.NoError(t, err)

	// create lockFC tx
	tx := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:   moneySystemID,
				Type:       transactions.PayloadTypeLockFeeCredit,
				UnitID:     unitID,
				Attributes: marshalCbor(&transactions.LockFeeCreditAttributes{LockStatus: 1}),
			},
		},
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}

	// create block with lockFC tx
	b := &types.Block{
		Header:             &types.Header{SystemID: moneySystemID},
		Transactions:       []*types.TransactionRecord{tx},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
	}

	// process lockFC tx
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify fee credit bill was locked
	fcb, err := store.Do().GetFeeCreditBill(unitID)
	require.NoError(t, err)
	require.NotNil(t, fcb)
	require.EqualValues(t, 1, fcb.Locked)
	require.EqualValues(t, tx.TransactionOrder.Hash(gocrypto.SHA256), fcb.TxHash)
}

func TestUnlockFC_Ok(t *testing.T) {
	store := createTestBillStore(t)

	// store existing locked fee credit bill for unlockFC tx
	unitID := newBillID(3)
	err := store.Do().SetFeeCreditBill(&Bill{Id: unitID, Locked: 1, Value: 100}, nil)
	require.NoError(t, err)

	// create unlockFC tx
	tx := &types.TransactionRecord{
		TransactionOrder: &types.TransactionOrder{
			Payload: &types.Payload{
				SystemID:   moneySystemID,
				Type:       transactions.PayloadTypeUnlockFeeCredit,
				UnitID:     unitID,
				Attributes: marshalCbor(&transactions.UnlockFeeCreditAttributes{}),
			},
		},
		ServerMetadata: &types.ServerMetadata{ActualFee: 1},
	}

	// create block with unlockFC tx
	b := &types.Block{
		Header:             &types.Header{SystemID: moneySystemID},
		Transactions:       []*types.TransactionRecord{tx},
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 1}},
	}

	// create block processor
	blockProcessor, err := NewBlockProcessor(store, moneySystemID, logger.New(t))
	require.NoError(t, err)

	// process block
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify fee credit bill was unlocked
	fcb, err := store.Do().GetFeeCreditBill(unitID)
	require.NoError(t, err)
	require.NotNil(t, fcb)
	require.EqualValues(t, 0, fcb.Locked)
	require.EqualValues(t, 99, fcb.Value)
	require.EqualValues(t, tx.TransactionOrder.Hash(gocrypto.SHA256), fcb.TxHash)
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
		NewBearer:   templates.NewP2pkh256BytesFromKeyHash(pubKeyHash),
		Backlink:    util.Sum256([]byte{}),
	}
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}

func dustTxAttr() []byte {
	attr := &money.TransferDCAttributes{
		Value:              100,
		Backlink:           util.Sum256([]byte{}),
		TargetUnitID:       []byte{0},
		TargetUnitBacklink: []byte{1},
	}
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}

func splitTxAttr(remainingValue uint64, targetUnits ...*money.TargetUnit) []byte {
	attr := &money.SplitAttributes{
		TargetUnits:    targetUnits,
		RemainingValue: remainingValue,
		Backlink:       util.Sum256([]byte{}),
	}
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}

func swapTxAttr(pubKeyHash []byte) []byte {
	attr := &money.SwapDCAttributes{
		OwnerCondition:   templates.NewP2pkh256BytesFromKeyHash(pubKeyHash),
		DcTransfers:      []*types.TransactionRecord{},
		DcTransferProofs: []*types.TxProof{},
		TargetValue:      100,
	}
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}

func marshalCbor(attr interface{}) []byte {
	attrBytes, _ := cbor.Marshal(attr)
	return attrBytes
}
