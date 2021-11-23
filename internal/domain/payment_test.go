package domain

import (
	"bytes"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestPaymentOrder_Bytes(t *testing.T) {
	p := &PaymentOrder{
		Type:              PaymentTypeSplit,
		BillID:            uint64(1),
		Amount:            uint32(10),
		Backlink:          []byte{1},
		PredicateArgument: []byte{2},
		PayeePredicate:    []byte{3},
	}

	var bytes bytes.Buffer
	// type
	bytes.WriteByte(0x02)
	// ID
	bytes.Write(make([]byte, 7))
	bytes.WriteByte(1)
	// amount
	bytes.Write(make([]byte, 3))
	bytes.WriteByte(0x0A)
	// backlink
	bytes.Write([]byte{1})
	// predicate
	bytes.Write([]byte{3})

	assert.Equal(t, bytes.Bytes(), p.Bytes())
}
