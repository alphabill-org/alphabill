package backend

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/hash"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestBlockProcessor_EachTxTypeCanBeProcessed(t *testing.T) {
	pubKeyBytes, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubKeyHash := hash.Sum256(pubKeyBytes)
	tx1 := &txsystem.Transaction{
		UnitId:                newUnitId(1),
		SystemId:              alphabillMoneySystemId,
		TransactionAttributes: testtransaction.CreateBillTransferTx(pubKeyHash),
	}
	tx2 := &txsystem.Transaction{
		UnitId:                newUnitId(2),
		SystemId:              alphabillMoneySystemId,
		TransactionAttributes: testtransaction.CreateDustTransferTx(pubKeyHash),
	}
	tx3 := &txsystem.Transaction{
		UnitId:                newUnitId(3),
		SystemId:              alphabillMoneySystemId,
		TransactionAttributes: testtransaction.CreateBillSplitTx(pubKeyHash, 1, 1),
	}
	tx4 := &txsystem.Transaction{
		UnitId:                newUnitId(4),
		SystemId:              alphabillMoneySystemId,
		TransactionAttributes: testtransaction.CreateRandomSwapTransferTx(pubKeyHash),
	}
	b := &block.Block{
		BlockNumber:        1,
		Transactions:       []*txsystem.Transaction{tx1, tx2, tx3, tx4},
		UnicityCertificate: &certificates.UnicityCertificate{},
	}

	store := NewInmemoryBillStore()
	_ = store.AddKey(NewPubkey(pubKeyBytes))
	bp := NewBlockProcessor(store)

	// process transactions
	err := bp.ProcessBlock(b)
	require.NoError(t, err)

	// verify bills exist
	bills, err := store.GetBills(pubKeyBytes)
	require.NoError(t, err)
	require.Len(t, bills, 4)

	// verify proofs exist
	for _, bi := range bills {
		proof, err := store.GetBlockProof(bi.Id)
		require.NoError(t, err)
		verifyProof(t, proof, bi.Id)
	}
}

func newUnitId(unitId uint64) []byte {
	return util.Uint256ToBytes(uint256.NewInt(unitId))
}

func verifyProof(t *testing.T, proof *BlockProof, billId []byte) {
	require.NotNil(t, proof)
	require.EqualValues(t, 1, proof.BlockNumber)
	require.Equal(t, billId, proof.BillId)

	blockProof := proof.BlockProof
	require.NotNil(t, blockProof)
	require.NotNil(t, blockProof.BlockHeaderHash)
	require.NotNil(t, blockProof.TransactionsHash)
	require.NotNil(t, blockProof.HashValue)
	require.NotNil(t, blockProof.BlockTreeHashChain)
	require.Nil(t, blockProof.SecTreeHashChain)
	require.NotNil(t, blockProof.UnicityCertificate)
}
