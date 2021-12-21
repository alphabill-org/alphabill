package shard

import (
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"

	"github.com/stretchr/testify/mock"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/shard/mocks"
	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"github.com/stretchr/testify/require"
)

func TestProcessNew_Nil(t *testing.T) {
	s, err := New(nil)
	require.Nil(t, s)
	require.Error(t, err)
}

func TestProcess_Ok(t *testing.T) {
	sp := new(mocks.StateProcessor)
	s, err := New(sp)
	require.Nil(t, err)

	sp.On("ProcessPayment", mock.Anything).Return(nil)

	status, err := s.Process(test.RandomPaymentOrder(domain.PaymentTypeTransfer))
	require.Nil(t, err)
	require.Equal(t, "1", status)
}

func TestProcess_Nok(t *testing.T) {
	sp := new(mocks.StateProcessor)
	s, err := New(sp)
	require.Nil(t, err)

	sp.On("ProcessPayment", mock.Anything).Return(errors.New("expecting error"))

	status, err := s.Process(test.RandomPaymentOrder(domain.PaymentTypeTransfer))
	require.Error(t, err)
	require.Empty(t, status)
}

func TestStatus_NotImplemented(t *testing.T) {
	sp := new(mocks.StateProcessor)
	s, err := New(sp)
	require.Nil(t, err)

	status, err := s.Status("1")
	require.Nil(t, status)
	require.Error(t, err)
}
