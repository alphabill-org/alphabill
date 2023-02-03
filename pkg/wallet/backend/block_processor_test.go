package backend

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/hash"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/stretchr/testify/require"
)

func TestBlockProcessor_EachTxTypeCanBeProcessed(t *testing.T) {
	pubKeyBytes, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubKeyHash := hash.Sum256(pubKeyBytes)
	alphabillMoneySystemId := []byte{0, 0, 0, 0}
	tx1 := &txsystem.Transaction{
		UnitId:                test.NewUnitID(1),
		SystemId:              alphabillMoneySystemId,
		TransactionAttributes: moneytesttx.CreateBillTransferTx(pubKeyHash),
	}
	tx2 := &txsystem.Transaction{
		UnitId:                test.NewUnitID(2),
		SystemId:              alphabillMoneySystemId,
		TransactionAttributes: moneytesttx.CreateDustTransferTx(pubKeyHash),
	}
	tx3 := &txsystem.Transaction{
		UnitId:                test.NewUnitID(3),
		SystemId:              alphabillMoneySystemId,
		TransactionAttributes: moneytesttx.CreateBillSplitTx(pubKeyHash, 1, 1),
	}
	tx4 := &txsystem.Transaction{
		UnitId:                test.NewUnitID(4),
		SystemId:              alphabillMoneySystemId,
		TransactionAttributes: moneytesttx.CreateRandomSwapTransferTx(pubKeyHash),
	}
	b := &block.Block{
		Transactions:       []*txsystem.Transaction{tx1, tx2, tx3, tx4},
		UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 1}},
	}

	store, err := createTestBillStore(t)
	require.NoError(t, err)
	_ = store.AddKey(NewPubkey(pubKeyBytes))
	bp := NewBlockProcessor(alphabillMoneySystemId, store)

	// process transactions
	err = bp.ProcessBlock(b)
	require.NoError(t, err)

	// verify bills exist
	bills, err := store.GetBills(pubKeyBytes)
	require.NoError(t, err)
	require.Len(t, bills, 4)
	for _, bill := range bills {
		verifyProof(t, bill)
	}

	// verify tx2 is dcBill
	bill, _ := store.GetBill(pubKeyBytes, tx2.UnitId)
	require.True(t, bill.IsDCBill)
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
