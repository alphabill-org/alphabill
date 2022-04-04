package partition

import (
	"fmt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"os"
	"path"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPersistentBlockStore_CanBeCreated(t *testing.T) {
	bs, err := createTestBlockStore(t)
	require.NoError(t, err)
	require.NotNil(t, bs)
}

func TestPersistentBlockStore_AddGetBlock(t *testing.T) {
	bs, _ := createTestBlockStore(t)
	tp := newDummyBlock(1)

	// add block
	err := bs.Add(tp)
	require.NoError(t, err)

	// verify block
	b, err := bs.Get(tp.TxSystemBlockNumber)
	require.NoError(t, err)
	verifyBlock(t, tp, b)
}

func TestPersistentBlockStore_LatestBlock(t *testing.T) {
	bs, _ := createTestBlockStore(t)

	tb1 := newDummyBlock(1)
	tb2 := newDummyBlock(2)
	tb3 := newDummyBlock(3)

	_ = bs.Add(tb1)
	_ = bs.Add(tb2)
	_ = bs.Add(tb3)

	// verify height returns the latest block number
	height, err := bs.Height()
	require.NoError(t, err)
	require.EqualValues(t, 3, height)

	// verify latest block is the last block added
	b, err := bs.LatestBlock()
	require.NoError(t, err)
	verifyBlock(t, tb3, b)
}

func newDummyBlock(blockNo uint64) *Block {
	return &Block{
		SystemIdentifier:         []byte{0},
		TxSystemBlockNumber:      blockNo,
		PreviousBlockHash:        []byte{2},
		Transactions:             []*transaction.Transaction{},
		UnicityCertificateRecord: &UnicityCertificateRecord{},
	}
}

func createTestBlockStore(t *testing.T) (*PersistentBlockStore, error) {
	dbFile := path.Join(os.TempDir(), blocksDbFileName)
	t.Cleanup(func() {
		err := os.Remove(dbFile)
		if err != nil {
			fmt.Println("error deleting blocks.db %w", err)
		}
	})
	return NewPersistentBlockStore(dbFile)
}

func verifyBlock(t *testing.T, expected *Block, actual *Block) {
	require.EqualValues(t, expected.SystemIdentifier, actual.SystemIdentifier)
	require.EqualValues(t, expected.TxSystemBlockNumber, actual.TxSystemBlockNumber)
	require.EqualValues(t, expected.PreviousBlockHash, actual.PreviousBlockHash)
	require.EqualValues(t, expected.Transactions, actual.Transactions)
	require.EqualValues(t, expected.UnicityCertificateRecord, actual.UnicityCertificateRecord)
}
