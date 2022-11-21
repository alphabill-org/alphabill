package backend

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	testtransaction "github.com/alphabill-org/alphabill/internal/testutils/transaction"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	bolt "go.etcd.io/bbolt"
)

func TestBillStore_CanBeCreated(t *testing.T) {
	bs, err := createTestBillStore(t)
	require.NoError(t, err)
	require.NotNil(t, bs)
}

func TestBlockStore_GetSetBlockNumber(t *testing.T) {
	bs, _ := createTestBillStore(t)

	// verify initial block number is 0
	blockNumber, err := bs.GetBlockNumber()
	require.NoError(t, err)
	require.EqualValues(t, 0, blockNumber)

	// set block number
	err = bs.SetBlockNumber(1)
	require.NoError(t, err)

	// verify block number
	blockNumber, err = bs.GetBlockNumber()
	require.NoError(t, err)
	require.EqualValues(t, 1, blockNumber)
}

func TestBillStore_GetSetBills(t *testing.T) {
	bs, _ := createTestBillStore(t)
	pubKey, _ := hexutil.Decode("0x000000000000000000000000000000000000000000000000000000000000000000")

	// verify non indexed pubkey returns nil bills list
	bills, err := bs.GetBills(pubKey)
	require.NoError(t, err)
	require.Nil(t, bills)

	// add bills
	err = bs.SetBills(pubKey, newBillWithValue(1))
	require.NoError(t, err)

	err = bs.SetBills(pubKey, newBillWithValue(2))
	require.NoError(t, err)

	err = bs.SetBills(pubKey, newBillWithValue(3))
	require.NoError(t, err)

	// get bills
	bills, err = bs.GetBills(pubKey)
	require.NoError(t, err)
	require.Len(t, bills, 3)

	// test order number is added to bills
	for i, b := range bills {
		require.EqualValues(t, i+1, b.OrderNumber)
	}

	// test max order number is updated
	var maxOrderNumber uint64
	err = bs.db.View(func(tx *bolt.Tx) error {
		maxOrderNumber = bs.getMaxBillOrderNumber(tx, pubKey)
		return nil
	})
	require.NoError(t, err)
	require.EqualValues(t, 3, maxOrderNumber)

	// test contains bill ok
	expectedBillId := newUnitId(1)
	f, err := bs.ContainsBill(expectedBillId)
	require.NoError(t, err)
	require.True(t, f)

	// test remove bill
	err = bs.RemoveBill(pubKey, expectedBillId)
	require.Nil(t, err)

	// test contains bill returns false after removal
	f, err = bs.ContainsBill(expectedBillId)
	require.NoError(t, err)
	require.False(t, f)
}

func TestBillStore_GetSetProofs(t *testing.T) {
	bs, _ := createTestBillStore(t)
	billId := newUnitId(1)
	pubkey := []byte{0}

	// verify nil bill
	b, err := bs.GetBill(billId)
	require.NoError(t, err)
	require.Nil(t, b)

	// add bill
	expectedBill := &Bill{
		Id: billId,
		BlockProof: &BlockProof{
			BlockNumber: 1,
			Proof:       &block.BlockProof{BlockHeaderHash: []byte{1}},
			Tx:          testtransaction.NewTransaction(t),
		},
	}
	err = bs.SetBills(pubkey, expectedBill)
	require.NoError(t, err)

	// verify get bill
	b, err = bs.GetBill(billId)
	require.NoError(t, err)
	require.Equal(t, expectedBill, b)
}

func TestBillStore_GetSetKeys(t *testing.T) {
	bs, _ := createTestBillStore(t)
	pubKeyBytes, _ := hexutil.Decode("0x000000000000000000000000000000000000000000000000000000000000000000")
	pubkey := NewPubkey(pubKeyBytes)

	// add key
	err := bs.AddKey(pubkey)
	require.NoError(t, err)

	// verify getKeys
	keys, err := bs.GetKeys()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, pubkey, keys[0])

	// verify adding same key does not overwrite existing key
	err = bs.AddKey(pubkey)
	require.ErrorIs(t, err, ErrKeyAlreadyExists)
	keys, err = bs.GetKeys()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Equal(t, pubkey, keys[0])
}

func createTestBillStore(t *testing.T) (*BoltBillStore, error) {
	dbFile := path.Join(os.TempDir(), BoltBillStoreFileName)
	t.Cleanup(func() {
		err := os.Remove(dbFile)
		if err != nil {
			fmt.Println("error deleting bills.db ", err)
		}
	})
	return NewBoltBillStore(dbFile)
}

func newBillWithValue(val uint64) *Bill {
	id := uint256.NewInt(val)
	return &Bill{
		Id:    util.Uint256ToBytes(id),
		Value: val,
	}
}
