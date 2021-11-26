package test

import (
	"math/rand" // #nosec 404 - using math/rand, test code

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
)

func RandomBytes(len int) []byte {
	bytes := make([]byte, len)
	_, err := rand.Read(bytes)
	if err != nil {
		panic(err)
	}
	return bytes
}

func RandomPaymentOrder(pt domain.PaymentType) *domain.PaymentOrder {
	return &domain.PaymentOrder{
		Type:              pt,
		BillID:            rand.Uint64(),
		Amount:            rand.Uint32(),
		Backlink:          RandomBytes(3),
		PayeePredicate:    RandomBytes(3),
		PredicateArgument: RandomBytes(3),
	}
}
