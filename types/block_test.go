package types

import (
	"crypto"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBlock_GetBlockFees(t *testing.T) {
	t.Run("Block is nil", func(t *testing.T) {
		var b *Block = nil
		require.EqualValues(t, 0, b.GetBlockFees(), "GetBlockFees()")
	})
	t.Run("UC is nil", func(t *testing.T) {
		b := &Block{}
		require.EqualValues(t, 0, b.GetBlockFees(), "GetBlockFees()")
	})
	t.Run("InputRecord is nil", func(t *testing.T) {
		b := &Block{UnicityCertificate: &UnicityCertificate{}}
		require.EqualValues(t, 0, b.GetBlockFees(), "GetBlockFees()")
	})
	t.Run("InputRecord is nil", func(t *testing.T) {
		b := &Block{UnicityCertificate: &UnicityCertificate{
			InputRecord: &InputRecord{SumOfEarnedFees: 10},
		}}
		require.EqualValues(t, 10, b.GetBlockFees(), "GetBlockFees()")
	})
}

func TestBlock_GetProposerID(t *testing.T) {
	t.Run("Block is nil", func(t *testing.T) {
		var b *Block = nil
		require.Equal(t, "", b.GetProposerID())
	})
	t.Run("Header is nil", func(t *testing.T) {
		b := &Block{}
		require.Equal(t, "", b.GetProposerID())
	})
	t.Run("Proposer not set", func(t *testing.T) {
		b := &Block{Header: &Header{}}
		require.Equal(t, "", b.GetProposerID())
	})
	t.Run("Proposer equal", func(t *testing.T) {
		b := &Block{Header: &Header{ProposerID: "test"}}
		require.Equal(t, "test", b.GetProposerID())
	})
}

func TestBlock_GetRoundNumber(t *testing.T) {
	t.Run("block is nil", func(t *testing.T) {
		var b *Block = nil
		require.EqualValues(t, 0, b.GetRoundNumber())
	})
	t.Run("UC is nil", func(t *testing.T) {
		b := &Block{}
		require.EqualValues(t, 0, b.GetRoundNumber())
	})
	t.Run("InputRecord is nil", func(t *testing.T) {
		b := &Block{UnicityCertificate: &UnicityCertificate{}}
		require.EqualValues(t, 0, b.GetRoundNumber())
	})
	t.Run("InputRecord is nil", func(t *testing.T) {
		b := &Block{UnicityCertificate: &UnicityCertificate{
			InputRecord: &InputRecord{RoundNumber: 10},
		}}
		require.EqualValues(t, 10, b.GetRoundNumber())
	})
}

func TestBlock_SystemID(t *testing.T) {
	t.Run("Block is nil", func(t *testing.T) {
		var b *Block = nil
		require.EqualValues(t, 0, b.SystemID())
	})
	t.Run("Header is nil", func(t *testing.T) {
		b := &Block{}
		require.EqualValues(t, 0, b.SystemID())
	})
	t.Run("SystemID not set", func(t *testing.T) {
		b := &Block{Header: &Header{}}
		require.EqualValues(t, 0, b.SystemID())
	})
	t.Run("SystemID equal", func(t *testing.T) {
		b := &Block{Header: &Header{
			SystemID: SystemID(5),
		}}
		require.Equal(t, SystemID(5), b.SystemID())
	})
}

func TestBlock_IsValid(t *testing.T) {
	validFn := func(uc *UnicityCertificate) error {
		return nil
	}
	t.Run("Block is nil", func(t *testing.T) {
		var b *Block = nil
		require.EqualError(t, b.IsValid(validFn), "block is nil")
	})
	t.Run("Header is nil", func(t *testing.T) {
		b := &Block{}
		require.EqualError(t, b.IsValid(validFn), "block error: block header is nil")
	})
	t.Run("Transactions is nil", func(t *testing.T) {
		b := &Block{
			Header: &Header{
				SystemID:          SystemID(1),
				ProposerID:        "test",
				PreviousBlockHash: []byte{1, 2, 3},
			},
		}
		require.EqualError(t, b.IsValid(validFn), "transactions is nil")
	})
	t.Run("UC is nil", func(t *testing.T) {
		b := &Block{
			Header: &Header{
				SystemID:          SystemID(1),
				ProposerID:        "test",
				PreviousBlockHash: []byte{1, 2, 3},
			},
			Transactions: make([]*TransactionRecord, 0),
		}
		require.EqualError(t, b.IsValid(validFn), "unicity certificate is nil")
	})
	t.Run("UC not valid", func(t *testing.T) {
		notValidFn := func(uc *UnicityCertificate) error {
			return fmt.Errorf("some error occurred")
		}
		b := &Block{
			Header: &Header{
				SystemID:          SystemID(1),
				ProposerID:        "test",
				PreviousBlockHash: []byte{1, 2, 3},
			},
			Transactions:       make([]*TransactionRecord, 0),
			UnicityCertificate: &UnicityCertificate{},
		}
		require.EqualError(t, b.IsValid(notValidFn), "unicity certificate validation failed: some error occurred")
	})
	t.Run("valid", func(t *testing.T) {
		b := &Block{
			Header: &Header{
				SystemID:          SystemID(1),
				ProposerID:        "test",
				PreviousBlockHash: []byte{1, 2, 3},
			},
			Transactions:       make([]*TransactionRecord, 0),
			UnicityCertificate: &UnicityCertificate{},
		}
		require.NoError(t, b.IsValid(validFn))
	})
}

func TestBlock_Hash(t *testing.T) {
	t.Run("missing header", func(t *testing.T) {
		b := &Block{}
		hash, err := b.Hash(crypto.SHA256)
		require.Nil(t, hash)
		require.EqualError(t, err, "invalid block: block header is nil")
	})
	t.Run("state hash is missing", func(t *testing.T) {
		b := &Block{
			Header: &Header{
				SystemID:          SystemID(1),
				ProposerID:        "test",
				PreviousBlockHash: []byte{1, 2, 3},
			},
			Transactions:       make([]*TransactionRecord, 0),
			UnicityCertificate: &UnicityCertificate{},
		}
		hash, err := b.Hash(crypto.SHA256)
		require.Nil(t, hash)
		require.EqualError(t, err, "invalid block: state hash is nil")
	})
	t.Run("previous state hash is missing", func(t *testing.T) {
		b := &Block{
			Header: &Header{
				SystemID:          SystemID(1),
				ProposerID:        "test",
				PreviousBlockHash: []byte{1, 2, 3},
			},
			Transactions: make([]*TransactionRecord, 0),
			UnicityCertificate: &UnicityCertificate{
				InputRecord: &InputRecord{
					Hash: []byte{1, 1, 1},
				},
			},
		}
		hash, err := b.Hash(crypto.SHA256)
		require.Nil(t, hash)
		require.EqualError(t, err, "invalid block: previous state hash is nil")
	})
	t.Run("previous state hash is missing", func(t *testing.T) {
		b := &Block{
			Header: &Header{
				SystemID:          SystemID(1),
				ProposerID:        "test",
				PreviousBlockHash: []byte{1, 2, 3},
			},
			Transactions: make([]*TransactionRecord, 0),
			UnicityCertificate: &UnicityCertificate{
				InputRecord: &InputRecord{
					Hash:         []byte{1, 1, 1},
					PreviousHash: []byte{1, 1, 1},
				},
			},
		}
		hash, err := b.Hash(crypto.SHA256)
		require.NoError(t, err)
		require.Equal(t, hash, make([]byte, 32))
	})

	t.Run("hash - ok", func(t *testing.T) {
		b := &Block{
			Header: &Header{
				SystemID:          SystemID(1),
				ProposerID:        "test",
				PreviousBlockHash: []byte{1, 2, 3},
			},
			Transactions: make([]*TransactionRecord, 0),
			UnicityCertificate: &UnicityCertificate{
				InputRecord: &InputRecord{
					Hash:         []byte{1, 1, 1},
					PreviousHash: []byte{2, 2, 2},
				},
			},
		}
		hash, err := b.Hash(crypto.SHA256)
		require.NoError(t, err)
		require.NotNil(t, hash)
	})
}

func TestHeader_IsValid(t *testing.T) {
	t.Run("header is nil", func(t *testing.T) {
		var h *Header = nil
		require.EqualError(t, h.IsValid(), "block header is nil")
	})
	t.Run("system identifier is nil", func(t *testing.T) {
		h := &Header{}
		require.EqualError(t, h.IsValid(), "system identifier is unassigned")
	})
	t.Run("previous block hash is nil", func(t *testing.T) {
		h := &Header{
			SystemID: SystemID(2),
		}
		require.EqualError(t, h.IsValid(), "previous block hash is nil")
	})
	t.Run("proposer is missing", func(t *testing.T) {
		h := &Header{
			SystemID:          SystemID(2),
			PreviousBlockHash: []byte{1, 2, 3},
		}
		require.EqualError(t, h.IsValid(), "block proposer node identifier is missing")
	})
	t.Run("valid", func(t *testing.T) {
		h := &Header{
			SystemID:          SystemID(2),
			PreviousBlockHash: []byte{1, 2, 3},
			ProposerID:        "test",
		}
		require.NoError(t, h.IsValid())
	})
}

func TestHeader_Hash(t *testing.T) {
	h := &Header{
		SystemID:          SystemID(2),
		ShardID:           []byte{1, 1, 1},
		ProposerID:        "test",
		PreviousBlockHash: []byte{2, 2, 2},
	}
	headerHash := h.Hash(crypto.SHA256)
	serilized := []byte{
		0, 0, 0, 2,
		1, 1, 1,
		2, 2, 2,
		't', 'e', 's', 't',
	}
	hasher := crypto.SHA256.New()
	hasher.Write(serilized)
	require.Equal(t, headerHash, hasher.Sum(nil))
}

func TestBlock_InputRecord(t *testing.T) {
	t.Run("err: block is nil", func(t *testing.T) {
		var b *Block = nil
		got, err := b.InputRecord()
		require.ErrorIs(t, err, errBlockIsNil)
		require.Nil(t, got)
	})
	t.Run("err: UC is nil", func(t *testing.T) {
		b := &Block{}
		got, err := b.InputRecord()
		require.ErrorIs(t, err, errUCIsNil)
		require.Nil(t, got)
	})
	t.Run("err: IR is nil", func(t *testing.T) {
		b := &Block{
			UnicityCertificate: &UnicityCertificate{},
		}
		got, err := b.InputRecord()
		require.ErrorIs(t, err, ErrInputRecordIsNil)
		require.Nil(t, got)
	})
	t.Run("ok", func(t *testing.T) {
		b := &Block{
			UnicityCertificate: &UnicityCertificate{
				InputRecord: &InputRecord{},
			},
		}
		got, err := b.InputRecord()
		require.NoError(t, err)
		require.NotNil(t, got)
	})
}
