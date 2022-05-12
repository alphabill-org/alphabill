package shard

import (
	"crypto"
	"fmt"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/certificates"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/partition/store"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/alphabill"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/shard/mocks"
)

// A type to satisfy transaction.GenericTransaction interface
type genericTx struct{}

func (g *genericTx) SystemID() []byte                 { return nil }
func (g *genericTx) UnitID() *uint256.Int             { return nil }
func (g *genericTx) IDHash() string                   { return "" }
func (g *genericTx) Timeout() uint64                  { return 0 }
func (g *genericTx) OwnerProof() []byte               { return nil }
func (g *genericTx) Hash(hashFunc crypto.Hash) []byte { return nil }
func (g *genericTx) SigBytes() []byte                 { return nil }

func TestProcessNew_Nil(t *testing.T) {
	s, err := New(nil, nil, nil)
	require.Nil(t, s)
	require.Error(t, err)
}

func TestProcess_Ok(t *testing.T) {
	sp := new(mocks.StateProcessor)
	tc := new(mocks.TxConverter)
	bs := store.NewInMemoryBlockStore()
	s, err := New(tc, sp, bs)
	require.NoError(t, err)

	sp.On("Process", mock.Anything).Return(nil)

	err = s.Process(&genericTx{})
	require.NoError(t, err)
}

func TestProcess_Nok(t *testing.T) {
	sp := new(mocks.StateProcessor)
	tc := new(mocks.TxConverter)
	bs := store.NewInMemoryBlockStore()
	s, err := New(tc, sp, bs)
	require.NoError(t, err)

	sp.On("Process", mock.Anything).Return(errors.New("expecting error"))

	err = s.Process(&genericTx{})
	require.Error(t, err)
}

func TestGetBlock_Ok(t *testing.T) {
	sp := new(mocks.StateProcessor)
	tc := new(mocks.TxConverter)
	bs := store.NewInMemoryBlockStore()
	s, err := New(tc, sp, bs)
	require.NoError(t, err)

	// add mock block
	_ = bs.Add(&block.Block{
		BlockNumber: 1,
		UnicityCertificate: &certificates.UnicityCertificate{
			UnicitySeal: &certificates.UnicitySeal{},
		},
	})

	b, err := s.GetBlock(&alphabill.GetBlockRequest{BlockNo: 1})
	require.NoError(t, err)
	require.EqualValues(t, 1, b.Block.BlockNumber)
}

func TestGetBlock_Nok(t *testing.T) {
	sp := new(mocks.StateProcessor)
	tc := new(mocks.TxConverter)
	bs := store.NewInMemoryBlockStore()
	s, err := New(tc, sp, bs)
	require.NoError(t, err)

	b, err := s.GetBlock(&alphabill.GetBlockRequest{BlockNo: 1})
	require.ErrorContains(t, err, fmt.Sprintf("block with number %v not found", 1))
	require.Nil(t, b)
}

func TestGetMaxBlockNo_Ok(t *testing.T) {
	sp := new(mocks.StateProcessor)
	tc := new(mocks.TxConverter)
	bs := store.NewInMemoryBlockStore()
	s, err := New(tc, sp, bs)
	require.NoError(t, err)

	// add mock block
	_ = bs.Add(&block.Block{BlockNumber: 1})

	b, err := s.GetMaxBlockNo(&alphabill.GetMaxBlockNoRequest{})
	require.NoError(t, err)
	require.EqualValues(t, 1, b.BlockNo)
}

func TestGetMaxBlockNo_Nok(t *testing.T) {
	sp := new(mocks.StateProcessor)
	tc := new(mocks.TxConverter)
	bs := store.NewInMemoryBlockStore()
	s, err := New(tc, sp, bs)
	require.NoError(t, err)

	b, err := s.GetMaxBlockNo(&alphabill.GetMaxBlockNoRequest{})
	require.NoError(t, err)
	require.EqualValues(t, 0, b.BlockNo)
}
