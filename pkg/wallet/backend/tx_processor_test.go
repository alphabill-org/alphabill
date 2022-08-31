package backend

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/hash"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestTxProcessor(t *testing.T) {
	pubKey, _ := hexutil.Decode("0x03c30573dc0c7fd43fcb801289a6a96cb78c27f4ba398b89da91ece23e9a99aca3")
	pubKeyHash := hash.Sum256(pubKey)
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
		BlockNumber:  1,
		Transactions: []*txsystem.Transaction{tx1, tx2, tx3, tx4},
	}
	pk := &pubkey{
		pubkey: pubKey,
		pubkeyHash: &wallet.KeyHashes{
			Sha256: hash.Sum256(pubKey),
			Sha512: hash.Sum512(pubKey),
		},
	}

	store := NewInmemoryWalletBackendStore()
	txp := newTxProcessor(store)
	err := txp.processTx(tx1, b, pk)
	require.NoError(t, err)

	err = txp.processTx(tx2, b, pk)
	require.NoError(t, err)

	err = txp.processTx(tx3, b, pk)
	require.NoError(t, err)

	err = txp.processTx(tx4, b, pk)
	require.NoError(t, err)
	require.Len(t, store.GetBills(pubKey), 4)
}

func newUnitId(unitId uint64) []byte {
	bytes32 := uint256.NewInt(unitId).Bytes32()
	return bytes32[:]
}
