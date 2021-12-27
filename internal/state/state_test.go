package state

import (
	"crypto"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"github.com/stretchr/testify/assert"
)

func TestCreateEmptyState_SHA256_Ok(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	assert.Equal(t, uint64(0), s.maxBillID)
	assert.Equal(t, uint64(0), s.roundNumber)
	assert.Nil(t, s.root)
	assert.True(t, s.empty())
	assert.Equal(t, crypto.SHA256, s.hashAlgorithm)
}

func TestCreateEmptyState_SHA512_Ok(t *testing.T) {
	s, _ := New(crypto.SHA512, nil)
	assert.Equal(t, uint64(0), s.maxBillID)
	assert.Equal(t, uint64(0), s.roundNumber)
	assert.Nil(t, s.root)
	assert.True(t, s.empty())
	assert.Equal(t, crypto.SHA512, s.hashAlgorithm)
}

func TestCreateStateWithUnsupportedHashAlgorithm(t *testing.T) {
	s, err := New(crypto.MD5, nil)
	assert.Nil(t, s)
	assert.NotNil(t, err)
	assert.Error(t, ErrInvalidHashAlgorithm, err)
}

func TestAddBillsWithInitialContent_Ok(t *testing.T) {
	initialState := []*BillContent{newBillContent(1), newBillContent(2)}
	s, _ := New(crypto.SHA256, initialState)
	assert.EqualValues(t, 2, s.maxBillID)

	s.addBill(newBillContent(3))
	assert.EqualValues(t, 3, s.maxBillID)
}

func TestAddBills_Ok(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	total := 0
	for i := 1; i <= 10; i++ {
		s.addBill(newBillContent(uint32(i)))
		total += i
	}
	assert.Equal(t, uint64(10), s.maxBillID)
	assert.Equal(t, uint64(0), s.roundNumber)
	assert.NotNil(t, s.root)
	assert.False(t, s.empty())

	assert.Equal(t, key4, s.root.ID)
	assert.Equal(t, uint32(4), s.root.Bill.Value)

	assert.NotNil(t, s.GetRootHash())
	assert.Equal(t, uint32(total), s.root.Bill.TotalValue)
}

func TestUpdateNodeContent_Ok(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	s.addBill(newBillContent(1))
	s.addBill(newBillContent(1))

	err := s.updateBill(2, newBillContent(10))
	assert.NoError(t, err)
}

func TestUpdateNodeContent_BillNotPresent(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	s.addBill(newBillContent(1))
	s.addBill(newBillContent(1))

	err := s.updateBill(10, newBillContent(10))
	assert.Error(t, err)
	assert.Equal(t, ErrBillNotFound, err)
}

func TestGetBill_Ok(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	value1 := uint32(1)
	k1 := s.addBill(newBillContent(value1))
	s.addBill(newBillContent(2))
	s.addBill(newBillContent(3))

	bill, found := s.getBill(k1)
	assert.True(t, found)
	assert.NotNil(t, bill)
	assert.Equal(t, value1, bill.Value)
}

func TestGetBill_NotFound(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	s.addBill(newBillContent(1))
	s.addBill(newBillContent(1))
	s.addBill(newBillContent(1))

	node, found := s.getBill(key4)
	assert.False(t, found)
	assert.Nil(t, node)
}

func TestState_GetRootHash(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)

	k1 := s.addBill(newBillContent(10))
	k2 := s.addBill(newBillContent(20))
	k3 := s.addBill(newBillContent(30))

	root := s.GetRootHash()
	assert.NotNil(t, root)

	bill1, _ := getNode(s.root, k1)
	bill2, _ := getNode(s.root, k2)
	bill3, _ := getNode(s.root, k3)
	assert.False(t, bill1.recompute)
	assert.False(t, bill2.recompute)
	assert.False(t, bill3.recompute)

	var zeroHash = make([]byte, 32)
	left := calculateHash(t, bill1, zeroHash, 0, zeroHash, 0)
	right := calculateHash(t, bill3, zeroHash, 0, zeroHash, 0)
	rootHash := calculateHash(t, bill2, left, bill1.Bill.TotalValue, right, bill3.Bill.TotalValue)
	assert.Equal(t, rootHash, root)
}

func TestState_ProcessNilPayment(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	s.addBill(newBillContent(10))

	err := s.ProcessPayment(nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentOrder, err)
}

func TestState_ProcessPaymentWithUnknownType(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	billID := s.addBill(newBillContent(10))
	b, _ := s.getBill(billID)

	payment := newTransferOrder(billID, b.Backlink, []byte{0x1})
	payment.Type = 10
	err := s.ProcessPayment(payment)

	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentType, err)
}

func TestState_ProcessTransferOrder_Ok(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	billID := s.addBill(newBillContent(10))
	s.addBill(newBillContent(20))
	s.addBill(newBillContent(30))

	b, _ := s.getBill(billID)
	err := s.ProcessPayment(newTransferOrder(billID, b.Backlink, []byte{0x1}))
	assert.NoError(t, err)

	b, _ = s.getBill(billID)
	assert.NotNil(t, b)
}

func TestState_ProcessTransferOrder_AmountPresent(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	billID := s.addBill(newBillContent(10))

	b, _ := s.getBill(billID)

	order := newTransferOrder(billID, b.Backlink, []byte{0x1})
	order.Amount = 10

	err := s.ProcessPayment(order)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentAmount, err)
}

func TestState_ProcessTransferOrder_BillNotFound(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	billID := s.addBill(newBillContent(10))

	b, _ := s.getBill(billID)

	order := newTransferOrder(2, b.Backlink, []byte{0x1})

	err := s.ProcessPayment(order)
	assert.Error(t, err)
	assert.Equal(t, ErrBillNotFound, err)
}

func TestState_ProcessTransferOrder_InvalidBacklink(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	billID := s.addBill(newBillContent(10))
	order := newTransferOrder(billID, []byte("invalid"), []byte{0x1})

	err := s.ProcessPayment(order)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentBacklink, err)
}

func TestState_ProcessSplitOrder_BillNotFound(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	billID := s.addBill(newBillContent(10))

	b, _ := s.getBill(billID)

	order := newSplitOrder(2, b.Backlink, []byte{0x1}, 1)

	err := s.ProcessPayment(order)
	assert.Error(t, err)
	assert.Equal(t, ErrBillNotFound, err)
}

func TestState_ProcessSplitOrder_InvalidBacklink(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	billID := s.addBill(newBillContent(10))
	order := newSplitOrder(billID, []byte("invalid"), []byte{0x1}, 1)

	err := s.ProcessPayment(order)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentBacklink, err)
}

func TestState_ProcessSplitOrder_AmountInvalid(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	billID := s.addBill(newBillContent(10))

	b, _ := s.getBill(billID)

	order := newSplitOrder(billID, b.Backlink, []byte{0x1}, 11)

	err := s.ProcessPayment(order)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentAmount, err)
}

func TestState_ProcessSplitOrder_Ok(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	billID := s.addBill(newBillContent(10))
	b, _ := s.getBill(billID)

	order := newSplitOrder(billID, b.Backlink, []byte{0x1}, 6)

	err := s.ProcessPayment(order)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), s.maxBillID)

	s.GetRootHash()

	b, _ = s.getBill(billID)
	assert.NotNil(t, b)

	b2, _ := s.getBill(s.maxBillID)
	assert.NotNil(t, b2)

	assert.Equal(t, uint32(10), b.TotalValue)
	assert.Equal(t, uint32(4), b.Value)
	assert.NotEqual(t, order.PayeePredicate, b.BearerPredicate)

	assert.Equal(t, uint32(6), b2.TotalValue)
	assert.Equal(t, uint32(6), b2.Value)
	assert.Equal(t, order.PayeePredicate, b2.BearerPredicate)
}

func TestBillContent_CalculateStateHash_StateIsNotChanged(t *testing.T) {
	bc := newBillContent(10)
	hash := bc.calculateStateHash(nil, crypto.SHA256.New())
	assert.Equal(t, bc.StateHash, hash)
}

func TestBillContent_CalculateStateHash_TransferBill(t *testing.T) {
	bc := newBillContent(10)
	oldStateHash := bc.StateHash
	transfer := newTransferOrder(1, bc.Backlink, []byte{1})
	hash := bc.calculateStateHash(transfer, crypto.SHA256.New())

	hasher := crypto.SHA256.New()
	hasher.Write(transfer.Bytes())
	orderHash := hasher.Sum(nil)
	hasher.Reset()
	hasher.Write(oldStateHash)
	hasher.Write(orderHash)
	newBillHash := hasher.Sum(nil)

	assert.Equal(t, hash, newBillHash)
	assert.Equal(t, bc.StateHash, newBillHash)
}

func TestState_ProcessTransferOrder_InvalidPredicate(t *testing.T) {
	s, _ := New(crypto.SHA256, nil)
	bc := newBillContent(10)
	bc.BearerPredicate = []byte{script.StartByte, script.OpPushBool, script.BoolFalse}
	billID := s.addBill(bc)

	b, _ := s.getBill(billID)
	order := newTransferOrder(billID, b.Backlink, []byte{0x01})

	err := s.ProcessPayment(order)
	assert.Error(t, err)
}

func TestNewInitialBill_Empty(t *testing.T) {
	ib := NewInitialBill(0, nil)
	assert.EqualValues(t, 0, ib.Value)
	assert.Equal(t, domain.Predicate(nil), ib.BearerPredicate)
}

func TestNewInitialBill_Ok(t *testing.T) {
	ib := NewInitialBill(100, []byte{script.BoolTrue})
	assert.EqualValues(t, 100, ib.Value)
	assert.EqualValues(t, 0, ib.TotalValue)
	assert.Equal(t, domain.Predicate{script.BoolTrue}, ib.BearerPredicate)
	assert.Equal(t, []byte(nil), ib.Backlink)
	assert.Equal(t, []byte(nil), ib.StateHash)
}

func newTransferOrder(billID uint64, backlink []byte, newPredicate domain.Predicate) *domain.PaymentOrder {
	return newPaymentOrder(domain.PaymentTypeTransfer, billID, backlink, newPredicate, 0)
}

func newSplitOrder(billID uint64, backlink []byte, newPredicate domain.Predicate, amount uint32) *domain.PaymentOrder {
	return newPaymentOrder(domain.PaymentTypeSplit, billID, backlink, newPredicate, amount)
}

func newPaymentOrder(t domain.PaymentType, billID uint64, backlink []byte, payeePredicate domain.Predicate, amount uint32) *domain.PaymentOrder {
	return &domain.PaymentOrder{
		BillID:            billID,
		Type:              t,
		Amount:            amount,
		PayeePredicate:    payeePredicate,
		Backlink:          backlink,
		PredicateArgument: []byte{script.StartByte},
	}
}

func newBillContent(v uint32) *BillContent {
	return NewInitialBill(
		v,
		script.PredicateAlwaysTrue(),
	)
}

func calculateHash(t *testing.T, parent *Node, leftHash []byte, leftTotalValue uint32, rightHash []byte, rightTotalValue uint32) []byte {
	t.Helper()
	hasher := crypto.SHA256.New()
	// write bill ID
	hasher.Write(domain.Uint64ToBytes(parent.ID))
	// write bill value
	hasher.Write(domain.Uint32ToBytes(parent.Bill.Value))
	// write bill state hash
	hasher.Write(parent.Bill.StateHash)
	// write left child hash
	hasher.Write(leftHash)
	// write left child totalValue
	hasher.Write(domain.Uint32ToBytes(leftTotalValue))
	// write right child hash
	hasher.Write(rightHash)
	// write right child totalValue
	hasher.Write(domain.Uint32ToBytes(rightTotalValue))
	return hasher.Sum(nil)
}
