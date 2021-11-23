package state

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestSigBytesSerializationOrder(t *testing.T) {
	po := PaymentOrder{
		BillID:            1,
		Type:              PaymentTypeJoin,
		JoinBillId:        2,
		Amount:            5,
		PayeePredicate:    []byte{0x01},
		Backlink:          []byte{0x02},
		PredicateArgument: []byte{script.StartByte},
	}
	// 8 bytes id + 1 type byte + 8 bytes join bill id + 4 bytes amount  + backlink byte + payee predicate byte
	expectedBytes := []byte{1, 0, 0, 0, 0, 0, 0, 0, 3, 2, 0, 0, 0, 0, 0, 0, 0, 5, 0, 0, 0, 2, 1}
	assert.Equal(t, expectedBytes, po.SigBytes())
}
