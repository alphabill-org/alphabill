package test

import (
	"math/rand"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	testbytes "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/bytes"
)

func RandomPaymentOrder(pt domain.PaymentType) *domain.PaymentOrder {
	return &domain.PaymentOrder{
		Type: pt,
		// #nosec G404
		BillID: rand.Uint64(),
		// #nosec G404
		Amount:            rand.Uint32(),
		Backlink:          testbytes.RandomBytes(3),
		PayeePredicate:    testbytes.RandomBytes(3),
		PredicateArgument: testbytes.RandomBytes(3),
	}
}
