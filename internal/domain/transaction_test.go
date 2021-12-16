package domain

import (
	"crypto"
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
)

func TestTransactionOrderBytes(t *testing.T) {
	to := &TransactionOrder{
		TransactionId: uint256.NewInt(5),
		TransactionAttributes: &BillTransfer{
			NewBearer: []byte{1},
			Backlink:  []byte{2},
		},
		Timeout:    1000,
		OwnerProof: []byte{3},
	}

	var expected []byte
	idBytes := [32]byte{}
	idBytes[31] = 5
	expected = append(expected, idBytes[:]...)          // ID bytes
	expected = append(expected, 1, 2)                   // Bill transfer
	expected = append(expected, Uint64ToBytes(1000)...) // Timeout
	expected = append(expected, 3)                      // Owner Proof

	actual := to.Bytes()

	assert.Equal(t, expected, actual)
}

func TestTransactionOrderBytes_Nils(t *testing.T) {
	to := &TransactionOrder{
		TransactionId:         nil,
		TransactionAttributes: nil,
		Timeout:               0,
		OwnerProof:            nil,
	}

	var expected []byte
	idByte := [32]byte{}
	expected = append(expected, idByte[:]...)        // ID bytes
	expected = append(expected, Uint64ToBytes(0)...) // Timeout
	actual := to.Bytes()
	assert.Equal(t, expected, actual)
}

func TestTransactionOrderHash(t *testing.T) {
	to := &TransactionOrder{
		TransactionId: uint256.NewInt(5),
		TransactionAttributes: &BillTransfer{
			NewBearer: []byte{1},
			Backlink:  []byte{2},
		},
		Timeout:    1000,
		OwnerProof: []byte{3},
	}

	hash := to.Hash(crypto.SHA256.New())
	assert.NotNil(t, hash)
}
