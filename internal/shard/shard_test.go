package shard

import (
	"crypto"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/shard/mocks"
)

// A type to satisfy transaction.GenericTransaction interface
type genericTx struct{}

func (g *genericTx) SystemID() []byte                 { return nil }
func (g *genericTx) UnitId() *uint256.Int             { return nil }
func (g *genericTx) IDHash() string                   { return "" }
func (g *genericTx) Timeout() uint64                  { return 0 }
func (g *genericTx) OwnerProof() []byte               { return nil }
func (g *genericTx) Hash(hashFunc crypto.Hash) []byte { return nil }

func TestProcessNew_Nil(t *testing.T) {
	s, err := New(nil)
	require.Nil(t, s)
	require.Error(t, err)
}

func TestProcess_Ok(t *testing.T) {
	sp := new(mocks.StateProcessor)
	s, err := New(sp)
	require.Nil(t, err)

	sp.On("Process", mock.Anything).Return(nil)

	err = s.Process(&genericTx{})
	require.Nil(t, err)
}

func TestProcess_Nok(t *testing.T) {
	sp := new(mocks.StateProcessor)
	s, err := New(sp)
	require.Nil(t, err)

	sp.On("Process", mock.Anything).Return(errors.New("expecting error"))

	err = s.Process(&genericTx{})
	require.Error(t, err)
}
