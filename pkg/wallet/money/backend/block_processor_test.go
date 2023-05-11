package backend

import (
	"context"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/script"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
	tx1 := &txsystem.Transaction{
		UnitId:                newUnitID(1),
		SystemId:              []byte{0, 0, 0, 0},
		TransactionAttributes: moneytesttx.CreateBillTransferTx(pubKeyHash),
		ClientMetadata:        &txsystem.ClientMetadata{FeeCreditRecordId: fcbID},
		ServerMetadata:        &txsystem.ServerMetadata{Fee: 1},
	}
	tx2 := &txsystem.Transaction{
		UnitId:                newUnitID(2),
		SystemId:              moneySystemID,
		TransactionAttributes: moneytesttx.CreateDustTransferTx(pubKeyHash),
		ClientMetadata:        &txsystem.ClientMetadata{FeeCreditRecordId: fcbID},
		ServerMetadata:        &txsystem.ServerMetadata{Fee: 1},
	}
	tx3 := &txsystem.Transaction{
		UnitId:                newUnitID(3),
		SystemId:              moneySystemID,
		TransactionAttributes: moneytesttx.CreateBillSplitTx(pubKeyHash, 1, 1),
		ClientMetadata:        &txsystem.ClientMetadata{FeeCreditRecordId: fcbID},
		ServerMetadata:        &txsystem.ServerMetadata{Fee: 1},
	}
	tx4 := &txsystem.Transaction{
		UnitId:                newUnitID(4),
		SystemId:              moneySystemID,
		TransactionAttributes: moneytesttx.CreateRandomSwapTransferTx(pubKeyHash),
		ClientMetadata:        &txsystem.ClientMetadata{FeeCreditRecordId: fcbID},
		ServerMetadata:        &txsystem.ServerMetadata{Fee: 1},
	}

	b := &block.Block{
		Transactions:       []*txsystem.Transaction{tx1, tx2, tx3, tx4},
		UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 1}},
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
	blockProcessor, err := NewBlockProcessor(store, NewTxConverter(moneySystemID), moneySystemID)
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
	bill, err := store.Do().GetBill(tx2.UnitId)
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
		testtransaction.WithUnitId(tx1.UnitId),
		testtransaction.WithServerMetadata(&txsystem.ServerMetadata{Fee: 1}),
	)

	addFCAttr := testfc.NewAddFCAttr(t, signer,
		testfc.WithTransferFCTx(transferFC.Transaction),
	)
	addFC := testfc.NewAddFC(t, signer, addFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(fcbID),
		testtransaction.WithServerMetadata(&txsystem.ServerMetadata{Fee: 1}),
	)

	b = &block.Block{
		Transactions:       []*txsystem.Transaction{transferFC.Transaction, addFC.Transaction},
		UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 2}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify fee credit bill value (96) is incremented by transferFC value (99) minus txfee (1)
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 194, fcb.Value)

	// verify tx1 unit is deleted (zero value bills are not allowed)
	unit1, err := store.Do().GetBill(tx1.UnitId)
	require.NoError(t, err)
	require.Nil(t, unit1)

	// process closeFC + reclaimFC (reclaim all credits to bill no 4)
	closeFCAttr := testfc.NewCloseFCAttr(
		testfc.WithCloseFCAmount(194),
		testfc.WithCloseFCTargetUnitID(tx4.UnitId),
	)
	closeFC := testfc.NewCloseFC(t, closeFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(fcbID),
		testtransaction.WithServerMetadata(&txsystem.ServerMetadata{Fee: 1}),
	)

	reclaimFCAttr := testfc.NewReclaimFCAttr(t, signer,
		testfc.WithReclaimFCClosureTx(closeFC.Transaction),
	)
	reclaimFC := testfc.NewReclaimFC(t, signer, reclaimFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(tx4.UnitId),
		testtransaction.WithServerMetadata(&txsystem.ServerMetadata{Fee: 1}),
	)

	b = &block.Block{
		Transactions:       []*txsystem.Transaction{closeFC.Transaction, reclaimFC.Transaction},
		UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 3}},
	}
	err = blockProcessor.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// verify FCB is reduced to zero
	fcb, err = store.Do().GetFeeCreditBill(fcbID)
	require.NoError(t, err)
	require.EqualValues(t, 0, fcb.Value)

	// verify reclaimed fee credits (194) were added to specified unit (tx4 value=100) minus 2x txfee (2)
	unit, err := store.Do().GetBill(tx4.UnitId)
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

	blockProcessor, err := NewBlockProcessor(store, NewTxConverter(moneySystemID), moneySystemID)
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
		testtransaction.WithServerMetadata(&txsystem.ServerMetadata{Fee: 1}),
	)
	b := &block.Block{
		Transactions:       []*txsystem.Transaction{transferFC.Transaction},
		UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 1}},
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
		testfc.WithCloseFCTargetUnitID(transferFC.Transaction.UnitId),
	)
	closeFC := testfc.NewCloseFC(t, closeFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(fcbID),
		testtransaction.WithServerMetadata(&txsystem.ServerMetadata{Fee: 1}),
	)

	reclaimFCAttr := testfc.NewReclaimFCAttr(t, signer,
		testfc.WithReclaimFCClosureTx(closeFC.Transaction),
	)
	reclaimFC := testfc.NewReclaimFC(t, signer, reclaimFCAttr,
		testtransaction.WithSystemID(moneySystemID),
		testtransaction.WithUnitId(transferFC.Transaction.UnitId),
		testtransaction.WithServerMetadata(&txsystem.ServerMetadata{Fee: 1}),
	)

	b = &block.Block{
		Transactions:       []*txsystem.Transaction{closeFC.Transaction, reclaimFC.Transaction},
		UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 3}},
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
	blockProof := b.TxProof
	require.NotNil(t, blockProof)
	require.EqualValues(t, 1, blockProof.BlockNumber)
	require.NotNil(t, blockProof.Tx)

	p := blockProof.Proof
	require.NotNil(t, p)
	require.NotNil(t, p.BlockHeaderHash)
	require.NotNil(t, p.TransactionsHash)
	require.NotNil(t, p.HashValue)
	require.NotNil(t, p.BlockTreeHashChain)
	require.Nil(t, p.SecTreeHashChain)
	require.NotNil(t, p.UnicityCertificate)
}

func newUnitID(unitID uint64) []byte {
	return util.Uint256ToBytes(uint256.NewInt(unitID))
}
