package state

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
		Backlink:          make([]byte, 32),
		PredicateArgument: make([]byte, 32),
		PayeePredicate:    nil,
	}

	var bytes bytes.Buffer
	// type
	bytes.WriteByte(0x01)
	// ID (LE)
	bytes.WriteByte(1)
	bytes.Write(make([]byte, 7))
	// amount (LE)
	bytes.WriteByte(0x0A)
	bytes.Write(make([]byte, 3))
	// backlink
	bytes.Write(make([]byte, 32))
	// predicate
	bytes.Write(make([]byte, 32))

	assert.Equal(t, bytes.Bytes(), p.Bytes())
}
