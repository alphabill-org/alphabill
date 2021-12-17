package domain

import (
	"testing"

	"github.com/holiman/uint256"

	"github.com/stretchr/testify/assert"
)

func TestBillTransfer_Normalise(t *testing.T) {
	bt := BillTransfer{
		NewBearer: []byte{1},
		Backlink:  []byte{2},
	}

	assert.Equal(t, []byte{1, 2}, bt.Normalise())
}

func TestDustTransfer_Normalise(t *testing.T) {
	dt := DustTransfer{
		NewBearer:    []byte{1},
		Backlink:     []byte{2},
		Nonce:        uint256.NewInt(3),
		TargetBearer: []byte{4},
		TargetValue:  5,
	}

	expected := []byte{1, 2}
	nonceBytes := [32]byte{}
	nonceBytes[31] = 3
	expected = append(expected, nonceBytes[:]...)
	expected = append(expected, 4)
	expected = append(expected, Uint64ToBytes(5)...)

	assert.Equal(t, expected, dt.Normalise())
}

func TestDustTransfer_Normalise_Nils(t *testing.T) {
	dt := DustTransfer{
		NewBearer:    nil,
		Backlink:     nil,
		Nonce:        nil,
		TargetBearer: nil,
		TargetValue:  0,
	}

	expected := [32 + 8]byte{}
	assert.Equal(t, expected[:], dt.Normalise())
}

func TestBillSplit_Normalise(t *testing.T) {
	bs := BillSplit{
		Amount:       1,
		TargetBearer: []byte{2},
		TargetValue:  3,
		Backlink:     []byte{4},
	}

	var expected []byte
	expected = append(expected, Uint64ToBytes(1)...)
	expected = append(expected, 2)
	expected = append(expected, Uint64ToBytes(3)...)
	expected = append(expected, 4)

	assert.Equal(t, expected, bs.Normalise())
}

func TestSwap_Normalise(t *testing.T) {
	s := Swap{
		OwnerCondition:  []byte{1},
		BillIdentifiers: []*uint256.Int{uint256.NewInt(2), uint256.NewInt(3)},
		DustTransferOrders: []*DustTransfer{{
			NewBearer:    []byte{1},
			Backlink:     []byte{2},
			Nonce:        uint256.NewInt(3),
			TargetBearer: []byte{4},
			TargetValue:  5,
		}, {
			NewBearer:    []byte{6},
			Backlink:     []byte{7},
			Nonce:        uint256.NewInt(8),
			TargetBearer: []byte{9},
			TargetValue:  10,
		}},
		Proofs: []*LedgerProof{
			{PreviousStateHash: []byte{4}},
			{PreviousStateHash: []byte{5}},
		},
		TargetValue: 6,
	}

	var expected []byte
	expected = append(expected, 1)

	idBytes := [64]byte{}
	idBytes[31] = 2
	idBytes[63] = 3
	expected = append(expected, idBytes[:]...)

	expected = append(expected, s.DustTransferOrders[0].Normalise()...)
	expected = append(expected, s.DustTransferOrders[1].Normalise()...)

	expected = append(expected, s.Proofs[0].Normalise()...)
	expected = append(expected, s.Proofs[1].Normalise()...)

	expected = append(expected, Uint64ToBytes(6)...)

	assert.Equal(t, expected, s.Normalise())
}
