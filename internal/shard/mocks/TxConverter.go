// Code generated by mockery v2.10.0. DO NOT EDIT.

package mocks

import (
	mock "github.com/stretchr/testify/mock"

	transaction "gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
)

// TxConverter is an autogenerated mock type for the TxConverter type
type TxConverter struct {
	mock.Mock
}

// Convert provides a mock function with given fields: tx
func (_m *TxConverter) Convert(tx *transaction.Transaction) (transaction.GenericTransaction, error) {
	ret := _m.Called(tx)

	var r0 transaction.GenericTransaction
	if rf, ok := ret.Get(0).(func(*transaction.Transaction) transaction.GenericTransaction); ok {
		r0 = rf(tx)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(transaction.GenericTransaction)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(*transaction.Transaction) error); ok {
		r1 = rf(tx)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}