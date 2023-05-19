package tx_builder

import (
	"testing"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
	"github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

const testMnemonic = "dinosaur simple verify deliver bless ridge monkey design venue six problem lucky"

var (
	receiverPubKey, _ = hexutil.Decode("0x1234511c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c12345")
	accountKey, _     = account.NewKeys(testMnemonic)
)

func TestSplitTransactionAmount(t *testing.T) {
	receiverPubKey, _ := hexutil.Decode("0x1234511c7341399e876800a268855c894c43eb849a72ac5a9d26a0091041c12345")
	receiverPubKeyHash := hash.Sum256(receiverPubKey)
	keys, _ := account.NewKeys(testMnemonic)
	billId := uint256.NewInt(0)
	billIdBytes32 := billId.Bytes32()
	billIdBytes := billIdBytes32[:]
	b := &bp.Bill{
		Id:     billIdBytes,
		Value:  500,
		TxHash: []byte{1, 2, 3, 4},
	}
	amount := uint64(150)
	timeout := uint64(100)
	systemId := []byte{0, 0, 0, 0}

	tx, err := CreateSplitTx(amount, receiverPubKey, keys.AccountKey, systemId, b, timeout, nil)
	require.NoError(t, err)
	require.NotNil(t, tx)
	require.EqualValues(t, systemId, tx.SystemId)
	require.EqualValues(t, billIdBytes, tx.UnitId)
	require.EqualValues(t, timeout, tx.Timeout())
	require.NotNil(t, tx.OwnerProof)

	so := &moneytx.SplitAttributes{}
	err = tx.TransactionAttributes.UnmarshalTo(so)
	require.NoError(t, err)
	require.Equal(t, amount, so.Amount)
	require.EqualValues(t, script.PredicatePayToPublicKeyHashDefault(receiverPubKeyHash), so.TargetBearer)
	require.EqualValues(t, 350, so.RemainingValue)
	require.EqualValues(t, b.TxHash, so.Backlink)
}

func TestCreateTransactions(t *testing.T) {
	tests := []struct {
		name        string
		bills       []*bp.Bill
		amount      uint64
		txCount     int
		verify      func(t *testing.T, systemId []byte, txs []*txsystem.Transaction)
		expectedErr error
	}{
		{
			name:   "have more bills than target amount",
			bills:  []*bp.Bill{createBill(5), createBill(3), createBill(1)},
			amount: uint64(7),
			verify: func(t *testing.T, systemId []byte, txs []*txsystem.Transaction) {
				// verify tx count
				require.Len(t, txs, 2)

				// verify first tx is transfer order of bill no1
				tx, _ := moneytx.NewMoneyTx(systemId, txs[0])
				transferTx, ok := tx.(moneytx.Transfer)
				require.True(t, ok)
				require.EqualValues(t, 5, transferTx.TargetValue())

				// verify second tx is split order of bill no2
				tx, _ = moneytx.NewMoneyTx(systemId, txs[1])
				splitTx, ok := tx.(moneytx.Split)
				require.True(t, ok)
				require.EqualValues(t, 2, splitTx.Amount())
			},
		}, {
			name:   "have less bills than target amount",
			bills:  []*bp.Bill{createBill(5), createBill(1)},
			amount: uint64(7),
			verify: func(t *testing.T, systemId []byte, txs []*txsystem.Transaction) {
				require.Empty(t, txs)
			},
			expectedErr: ErrInsufficientBalance,
		}, {
			name:   "have exact amount of bills than target amount",
			bills:  []*bp.Bill{createBill(5), createBill(5)},
			amount: uint64(10),
			verify: func(t *testing.T, systemId []byte, txs []*txsystem.Transaction) {
				// verify tx count
				require.Len(t, txs, 2)

				// verify both bills are transfer orders
				for _, tx := range txs {
					mtx, _ := moneytx.NewMoneyTx(systemId, tx)
					transferTx, ok := mtx.(moneytx.Transfer)
					require.True(t, ok)
					require.EqualValues(t, 5, transferTx.TargetValue())
				}
			},
		}, {
			name:   "have exactly one bill with equal target amount",
			bills:  []*bp.Bill{createBill(5)},
			amount: uint64(5),
			verify: func(t *testing.T, systemId []byte, txs []*txsystem.Transaction) {
				// verify tx count
				require.Len(t, txs, 1)

				// verify transfer tx
				mtx, _ := moneytx.NewMoneyTx(systemId, txs[0])
				transferTx, ok := mtx.(moneytx.Transfer)
				require.True(t, ok)
				require.EqualValues(t, 5, transferTx.TargetValue())
			},
		},
	}

	systemId := []byte{0, 0, 0, 0}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txs, err := CreateTransactions(receiverPubKey, tt.amount, systemId, tt.bills, accountKey.AccountKey, 100, nil)
			if tt.expectedErr != nil {
				require.ErrorIs(t, err, tt.expectedErr)
			} else {
				require.NoError(t, err)
				tt.verify(t, systemId, txs)
			}
		})
	}
}

func createBill(value uint64) *bp.Bill {
	return &bp.Bill{
		Value:  value,
		Id:     util.Uint64ToBytes32(0),
		TxHash: []byte{},
	}
}
