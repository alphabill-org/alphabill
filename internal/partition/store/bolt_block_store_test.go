package store

import (
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/partition/genesis"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	moneytesttx "github.com/alphabill-org/alphabill/internal/testutils/transaction/money"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/stretchr/testify/require"
)

func TestPersistentBlockStore_CanBeCreated(t *testing.T) {
	bs, err := createTestBlockStore(t)
	require.NoError(t, err)
	require.NotNil(t, bs)
}

func TestPersistentBlockStore_AddGenesisBlock(t *testing.T) {
	bs, _ := createTestBlockStore(t)
	require.Error(t, bs.AddGenesis(newEmptyBlock(genesis.GenesisRoundNumber+1)))

	tp := newEmptyBlock(genesis.GenesisRoundNumber)

	require.Nil(t, bs.LatestUC())

	// add block
	err := bs.AddGenesis(tp)
	require.NoError(t, err)

	// verify block
	b, err := bs.Get(tp.UnicityCertificate.InputRecord.RoundNumber)
	require.NoError(t, err)
	verifyBlock(t, tp, b)

	genesisUC := bs.LatestUC()
	require.NotNil(t, genesisUC)
	require.Equal(t, tp.UnicityCertificate.InputRecord.RoundNumber, genesisUC.InputRecord.RoundNumber)
	require.Equal(t, genesis.GenesisRoundNumber, bs.LatestRoundNumber())
	bn, err := bs.BlockNumber()
	require.NoError(t, err)
	require.Equal(t, genesis.GenesisRoundNumber, bn)
}

func TestPersistentBlockStore_AddGet_EmptyBlock(t *testing.T) {
	bs, _ := createTestBlockStore(t)
	require.NoError(t, bs.AddGenesis(newEmptyBlock(genesis.GenesisRoundNumber)))

	require.Equal(t, genesis.GenesisRoundNumber, bs.LatestRoundNumber())
	genesisBlock := bs.LatestBlock()
	require.Equal(t, genesis.GenesisRoundNumber, genesisBlock.UnicityCertificate.InputRecord.RoundNumber)
	require.Equal(t, genesisBlock.UnicityCertificate, bs.LatestUC())

	// add empty block
	newBlock := newEmptyBlock(genesis.GenesisRoundNumber + 1)
	require.NoError(t, bs.Add(newBlock))

	// verify block is not persisted, but round number and UC are updated
	require.Equal(t, genesisBlock, bs.LatestBlock())
	b, err := bs.Get(newBlock.UnicityCertificate.InputRecord.RoundNumber)
	require.NoError(t, err)
	require.Nil(t, b)

	require.Equal(t, newBlock.UnicityCertificate, bs.LatestUC())
	require.Equal(t, newBlock.UnicityCertificate.InputRecord.RoundNumber, bs.LatestRoundNumber())
	require.NotEqual(t, newBlock.UnicityCertificate.InputRecord.RoundNumber, bs.LatestBlock().UnicityCertificate.InputRecord.RoundNumber)

}

func TestPersistentBlockStore_AddGet_NonEmptyBlock(t *testing.T) {
	bs, _ := createTestBlockStore(t)
	tp := newNonEmptyBlock(t, 1)

	// add block
	err := bs.Add(tp)
	require.NoError(t, err)

	// verify block
	b, err := bs.Get(tp.UnicityCertificate.InputRecord.RoundNumber)
	require.NoError(t, err)
	verifyBlock(t, tp, b)
}

func TestPersistentBlockStore_LatestBlock(t *testing.T) {
	bs, _ := createTestBlockStore(t)

	tb1 := newNonEmptyBlock(t, 1)
	tb2 := newNonEmptyBlock(t, 2)
	tb3 := newNonEmptyBlock(t, 3)

	_ = bs.Add(tb1)
	_ = bs.Add(tb2)
	_ = bs.Add(tb3)

	// verify the latest block number is returned
	number, err := bs.BlockNumber()
	require.NoError(t, err)
	require.EqualValues(t, 3, number)

	// verify latest block is the last block added
	b := bs.LatestBlock()
	verifyBlock(t, tb3, b)
}

func TestPersistentBlockStore_EmptyStore(t *testing.T) {
	bs, _ := createTestBlockStore(t)

	// verify returns 0
	number, err := bs.BlockNumber()
	require.NoError(t, err)
	require.EqualValues(t, 0, number)

	// verify latest block returns nil
	b := bs.LatestBlock()
	require.NoError(t, err)
	require.Nil(t, b)
}

func TestPersistentPendingBlockProposal(t *testing.T) {
	bs, _ := createTestBlockStore(t)
	prop := &block.PendingBlockProposal{
		RoundNumber: 1,
		PrevHash:    test.RandomBytes(32),
		StateHash:   test.RandomBytes(32),
	}
	require.NoError(t, bs.AddPendingProposal(prop))
	p, err := bs.GetPendingProposal()
	require.NoError(t, err)
	require.Equal(t, prop, p)
}

func TestPersistentGetPendingBlockProposal_NotFound(t *testing.T) {
	bs, _ := createTestBlockStore(t)
	p, err := bs.GetPendingProposal()
	require.ErrorContains(t, err, ErrStrPendingBlockProposalNotFound)
	require.Nil(t, p)
}

func TestPersistentBlockStore_InvalidBlockNo(t *testing.T) {
	bs, _ := createTestBlockStore(t)

	// try adding invalid block
	invalidBlock := newNonEmptyBlock(t, 2)
	err := bs.Add(invalidBlock)
	require.ErrorIs(t, err, errInvalidBlockNo)

	// try adding valid block twice
	validBlock := newNonEmptyBlock(t, 1)
	err = bs.Add(validBlock)
	require.NoError(t, err)
	err = bs.Add(validBlock)
	require.ErrorIs(t, err, errInvalidBlockNo)
}

func newEmptyBlock(blockNo uint64) *block.Block {
	return &block.Block{
		SystemIdentifier:   []byte{0},
		PreviousBlockHash:  []byte{2},
		Transactions:       nil,
		UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: blockNo}},
	}
}

func newNonEmptyBlock(t *testing.T, blockNo uint64) *block.Block {
	return &block.Block{
		SystemIdentifier:   []byte{0},
		PreviousBlockHash:  []byte{2},
		Transactions:       []*txsystem.Transaction{moneytesttx.RandomBillTransfer(t)},
		UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: blockNo}},
	}
}

func createTestBlockStore(t *testing.T) (*BoltBlockStore, error) {
	dbFile := path.Join(os.TempDir(), BoltBlockStoreFileName)
	t.Cleanup(func() {
		err := os.Remove(dbFile)
		if err != nil {
			fmt.Printf("error deleting blocks.db %v\n", err)
		}
	})
	return NewBoltBlockStore(dbFile)
}

func verifyBlock(t *testing.T, expected *block.Block, actual *block.Block) {
	require.EqualValues(t, expected.SystemIdentifier, actual.SystemIdentifier)
	require.EqualValues(t, expected.UnicityCertificate.InputRecord.RoundNumber, actual.UnicityCertificate.InputRecord.RoundNumber)
	require.EqualValues(t, expected.PreviousBlockHash, actual.PreviousBlockHash)
	require.EqualValues(t, expected.Transactions, actual.Transactions)
	require.EqualValues(t, expected.UnicityCertificate, actual.UnicityCertificate)
}
