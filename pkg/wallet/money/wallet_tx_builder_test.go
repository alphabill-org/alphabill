package money

import (
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/hash"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	moneytx "gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem/money"
	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

func TestSplitTransactionAmount(t *testing.T) {
	receiverPubKey, _ := hexutil.Decode("0x1234511c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c12345")
	receiverPubKeyHash := hash.Sum256(receiverPubKey)
	keys, _ := wallet.NewKeys(testMnemonic)
	billId := uint256.NewInt(0)
	billIdBytes32 := billId.Bytes32()
	billIdBytes := billIdBytes32[:]
	b := &bill{
		Id:     billId,
		Value:  500,
		TxHash: []byte{1, 2, 3, 4},
	}
	amount := uint64(150)
	timeout := uint64(100)

	tx, err := createSplitTx(amount, receiverPubKey, keys.AccountKey, b, timeout)
	require.NoError(t, err)
	require.NotNil(t, tx)

	require.EqualValues(t, alphabillMoneySystemId, tx.SystemId)
	require.EqualValues(t, billIdBytes, tx.UnitId)
	require.EqualValues(t, timeout, tx.Timeout)
	require.NotNil(t, tx.OwnerProof)

	so := &moneytx.SplitOrder{}
	err = tx.TransactionAttributes.UnmarshalTo(so)
	require.NoError(t, err)
	require.Equal(t, amount, so.Amount)
	require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(receiverPubKeyHash), so.TargetBearer)
	require.EqualValues(t, 350, so.RemainingValue)
	require.EqualValues(t, b.TxHash, so.Backlink)
}
