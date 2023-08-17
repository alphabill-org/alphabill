package backend

import (
	"context"
	gocrypto "crypto"
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
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

var moneySystemID = []byte{0, 0, 0, 0}
var tokenSystemID = []byte{0, 0, 0, 2}

func TestBlockProcessor_EachTxTypeCanBeProcessed(t *testing.T) {
	pubKeyBytes, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubKeyHash := hash.Sum256(pubKeyBytes)
	fcbID := newUnitID(101)
	fcb := &Bill{Id: fcbID, Value: 100}
	signer, _ := crypto.NewInMemorySecp256K1Signer()
	ownerCondition := script.PredicatePayToPublicKeyHashDefault(pubKeyHash)
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
	err = store.Do().SetFeeCreditBill(fcb, nil)
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
	bills, err := store.Do().GetBills(ownerCondition)
	require.NoError(t, err)
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

	// verify FCB LastAddFCTxHash equals to TxHash
	lastAddFCTxHash := addFC.Hash(gocrypto.SHA256)
	require.Equal(t, lastAddFCTxHash, fcb.TxHash)
	require.Equal(t, lastAddFCTxHash, fcb.LastAddFCTxHash)

	// verify tx1 unit is zero value (whole bill transferred to fee credit)
	unit1, err := store.Do().GetBill(tx1.TransactionOrder.UnitID())
	require.NoError(t, err)
	require.EqualValues(t, 0, unit1.Value)

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

	// verify FCB LastAddFCTxHash is not changed
	require.Equal(t, lastAddFCTxHash, fcb.LastAddFCTxHash)

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
	fcbID := newUnitID(101)
	signer, _ := crypto.NewInMemorySecp256K1Signer()

	store, err := createTestBillStore(t)
	require.NoError(t, err)

	userBillID := []byte{1}
	err = store.Do().SetBill(&Bill{
		Id:             userBillID,
		Value:          100,
		TxHash:         []byte{2},
		OwnerPredicate: script.PredicateAlwaysTrue(),
	}, nil)
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
	}, nil)
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

	// and user bill (100) is decreased by transferFC.amount (50) and transfer fee amount (1)
	userBill, err := store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 49, userBill.Value)

	// process addFC
	addFCOrder := testfc.NewAddFC(t, signer,
		testfc.NewAddFCAttr(t, signer, testfc.WithTransferFCTx(transferFCRecord)),
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
	require.EqualValues(t, 51, moneyPartitionFeeBill.Value)

	// and user bill does not change
	userBill, err = store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 49, userBill.Value)

	// and user fee credit bill is created by transferFC.amount (50) minus fee(1) = (49)
	fcb, err := store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 49, fcb.Value)

	// process closeFC
	closeFCAttr := testfc.NewCloseFCAttr(
		testfc.WithCloseFCAmount(49),
		testfc.WithCloseFCTargetUnitID(transferFC.UnitID()),
	)
	closeFC := testfc.NewCloseFC(t, closeFCAttr,
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
	require.EqualValues(t, 51, moneyPartitionFeeBill.Value)

	// and user bill does not change
	userBill, err = store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 49, userBill.Value)

	// and user fee credit bill is decreased by closeFC.amount (49)
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 0, fcb.Value)

	// process reclaimFC
	reclaimFCAttr := testfc.NewReclaimFCAttr(t, signer,
		testfc.WithReclaimFCClosureTx(closeFCRecord),
	)
	reclaimFC := testfc.NewReclaimFC(t, signer, reclaimFCAttr,
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

	// verify money partition fee bill value (51) is decreased by closeFC amount (49) and increased by closeFC fee amount (1) and reclaimFC fee amount(1)
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
	fcbID := newUnitID(101)
	signer, _ := crypto.NewInMemorySecp256K1Signer()

	store, err := createTestBillStore(t)
	require.NoError(t, err)

	userBillID := []byte{1}
	err = store.Do().SetBill(&Bill{
		Id:             userBillID,
		Value:          100,
		TxHash:         []byte{2},
		OwnerPredicate: script.PredicateAlwaysTrue(),
	}, nil)
	require.NoError(t, err)

	moneyPartitionFeeBillID := util.Uint256ToBytes(uint256.NewInt(2))
	tokenPartitionFeeBillID := util.Uint256ToBytes(uint256.NewInt(3))
	err = store.Do().SetSystemDescriptionRecords([]*genesis.SystemDescriptionRecord{
		{
			SystemIdentifier: moneySystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         moneyPartitionFeeBillID,
				OwnerPredicate: script.PredicateAlwaysTrue(),
			},
		},
		{
			SystemIdentifier: tokenSystemID,
			T2Timeout:        2500,
			FeeCreditBill: &genesis.FeeCreditBill{
				UnitId:         tokenPartitionFeeBillID,
				OwnerPredicate: script.PredicateAlwaysTrue(),
			},
		},
	})
	require.NoError(t, err)

	err = store.Do().SetBill(&Bill{
		Id:             moneyPartitionFeeBillID,
		OwnerPredicate: script.PredicateAlwaysTrue(),
		Value:          0,
	}, nil)
	require.NoError(t, err)
	err = store.Do().SetBill(&Bill{
		Id:             tokenPartitionFeeBillID,
		OwnerPredicate: script.PredicateAlwaysTrue(),
		Value:          0,
	}, nil)
	require.NoError(t, err)

	blockProcessor, err := NewBlockProcessor(store, moneySystemID)
	require.NoError(t, err)

	// process transferFC of 20 billy from userBillID
	transferFCAttr := testfc.NewTransferFCAttr(
		testfc.WithTargetRecordID(fcbID),
		testfc.WithTargetSystemID(tokenSystemID),
		testfc.WithAmount(20),
	)
	transferFC := testfc.NewTransferFC(t, transferFCAttr,
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

	// verify token partition fee bill value (0) is increased by transferFC amount (20)
	tokenPartitionFeeBill, err := store.Do().GetBill(tokenPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 20, tokenPartitionFeeBill.Value)

	// and money partition fee bill value (0) is increased by tx fee amount (1)
	moneyPartitionFeeBill, err := store.Do().GetBill(moneyPartitionFeeBillID)
	require.NoError(t, err)
	require.EqualValues(t, 1, moneyPartitionFeeBill.Value)

	// and user bill (100) is decreased by transferFC.amount (20) and transfer fee amount (1)
	userBill, err := store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 79, userBill.Value)

	// process addFC
	addFCOrder := testfc.NewAddFC(t, signer,
		testfc.NewAddFCAttr(t, signer, testfc.WithTransferFCTx(transferFCRecord)),
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
	require.EqualValues(t, 20, tokenPartitionFeeBill.Value)

	// and user bill does not change
	userBill, err = store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 79, userBill.Value)

	// and user fee credit bill is created by transferFC.amount (20) minus fee(1) = (19)
	fcb, err := store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 19, fcb.Value)

	// process closeFC
	closeFCAttr := testfc.NewCloseFCAttr(
		testfc.WithCloseFCAmount(19),
		testfc.WithCloseFCTargetUnitID(transferFC.UnitID()),
	)
	closeFC := testfc.NewCloseFC(t, closeFCAttr,
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
	require.EqualValues(t, 20, tokenPartitionFeeBill.Value)

	// and user bill does not change
	userBill, err = store.Do().GetBill(userBillID)
	require.NoError(t, err)
	require.EqualValues(t, 79, userBill.Value)

	// and user fee credit bill (19) is decreased by closeFC.amount (19)
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 0, fcb.Value)

	// process reclaimFC
	reclaimFCAttr := testfc.NewReclaimFCAttr(t, signer,
		testfc.WithReclaimFCClosureTx(closeFCRecord),
	)
	reclaimFC := testfc.NewReclaimFC(t, signer, reclaimFCAttr,
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

	// verify token partition fee bill value (20) is decreased by closeFC amount (19) and increased by closeFC fee amount (1)
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

func TestBlockProcessor_LockedAndClosedFeeCredit_CanBeSaved(t *testing.T) {
	fcbID := newUnitID(101)
	store, err := createTestBillStore(t)
	require.NoError(t, err)

	userBillID := []byte{1}
	err = store.Do().SetBill(&Bill{
		Id:             userBillID,
		Value:          100,
		TxHash:         []byte{2},
		OwnerPredicate: script.PredicateAlwaysTrue(),
	}, nil)
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
	}, nil)
	require.NoError(t, err)

	blockProcessor, err := NewBlockProcessor(store, moneySystemID)
	require.NoError(t, err)

	// when transferFC is processed
	transferFCAttr := testfc.NewTransferFCAttr(
		testfc.WithTargetRecordID(fcbID),
		testfc.WithTargetSystemID(moneySystemID),
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

	// then locked fee credit is added
	lockedFeeCredit, err := store.Do().GetLockedFeeCredit(moneySystemID, fcbID)
	require.NoError(t, err)
	require.Equal(t, transferFCRecord, lockedFeeCredit)

	// when closeFC is processed
	closeFCAttr := testfc.NewCloseFCAttr(
		testfc.WithCloseFCTargetUnitID(transferFC.UnitID()),
	)
	closeFC := testfc.NewCloseFC(t, closeFCAttr,
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

	// then closed fee credit is added
	closedFeeCredit, err := store.Do().GetClosedFeeCredit(fcbID)
	require.NoError(t, err)
	require.Equal(t, closeFCRecord, closedFeeCredit)
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
