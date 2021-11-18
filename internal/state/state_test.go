package state

import (
	"crypto"
	"github.com/stretchr/testify/assert"
	"testing"
)

const (
	key1  = uint64(1)
	key2  = uint64(2)
	key3  = uint64(3)
	key4  = uint64(4)
	key10 = uint64(10)
	key12 = uint64(12)
	key15 = uint64(15)
	key20 = uint64(20)
	key24 = uint64(24)
	key25 = uint64(25)
	key30 = uint64(30)
	key31 = uint64(31)
)

func TestCreateEmptyState_SHA256_Ok(t *testing.T) {
	s, _ := New(crypto.SHA256)
	assert.Equal(t, uint64(0), s.maxBillID)
	assert.Equal(t, uint64(0), s.roundNumber)
	assert.Nil(t, s.root)
	assert.True(t, s.empty())
	assert.Equal(t, crypto.SHA256, s.hashAlgorithm)
}

func TestCreateEmptyState_SHA512_Ok(t *testing.T) {
	s, _ := New(crypto.SHA512)
	assert.Equal(t, uint64(0), s.maxBillID)
	assert.Equal(t, uint64(0), s.roundNumber)
	assert.Nil(t, s.root)
	assert.True(t, s.empty())
	assert.Equal(t, crypto.SHA512, s.hashAlgorithm)
}

func TestCreateStateWithUnsupportedHashAlgorithm(t *testing.T) {
	s, err := New(crypto.MD5)
	assert.Nil(t, s)
	assert.NotNil(t, err)
	assert.Error(t, ErrInvalidHashAlgorithm, err)
}

func TestAddBills_Ok(t *testing.T) {
	s, _ := New(crypto.SHA256)
	total := 0
	for i := 1; i <= 10; i++ {
		s.addBill(newBillContent(uint32(i)))
		total += i
	}
	assert.Equal(t, uint64(10), s.maxBillID)
	assert.Equal(t, uint64(0), s.roundNumber)
	assert.NotNil(t, s.root)
	assert.False(t, s.empty())

	assert.Equal(t, key4, s.root.BillID)
	assert.Equal(t, uint32(4), s.root.Bill.Value)

	assert.NotNil(t, s.GetRootHash())
	assert.Equal(t, uint32(total), s.root.Bill.TotalValue)

}

func TestUpdateNodeContent_Ok(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(1))
	s.addBill(newBillContent(1))

	err := s.updateBill(uint64(2), newBillContent(10))
	assert.NoError(t, err)
}

func TestUpdateNodeContent_BillNotPresent(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(1))
	s.addBill(newBillContent(1))

	err := s.updateBill(uint64(10), newBillContent(10))
	assert.Error(t, err)
	assert.Equal(t, ErrBillNotFound, err)
}

func TestAddBill_AVLTreeRotateLeft(t *testing.T) {
	s, _ := New(crypto.SHA256)

	put(key1, newBillContent(1), nil, &s.root)
	put(key2, newBillContent(2), nil, &s.root)
	put(key3, newBillContent(3), nil, &s.root)

	assert.Equal(t, s.root.BillID, key2)
	assert.Equal(t, s.root.Children[0].BillID, key1)
	assert.Equal(t, s.root.Children[1].BillID, key3)
}

func TestAddBill_AVLTreeRotateRight(t *testing.T) {
	s, _ := New(crypto.SHA256)

	put(key3, newBillContent(3), nil, &s.root)
	put(key2, newBillContent(2), nil, &s.root)
	put(key1, newBillContent(1), nil, &s.root)

	assert.Equal(t, s.root.BillID, key2)
	assert.Equal(t, s.root.Children[0].BillID, key1)
	assert.Equal(t, s.root.Children[1].BillID, key3)
}

func TestAddBill_AVLTreeRotateLeftRight(t *testing.T) {
	s, _ := New(crypto.SHA256)

	put(key10, newBillContent(10), nil, &s.root)
	put(key20, newBillContent(20), nil, &s.root)
	put(key30, newBillContent(30), nil, &s.root)
	put(key1, newBillContent(1), nil, &s.root)
	put(key15, newBillContent(15), nil, &s.root)
	put(key12, newBillContent(12), nil, &s.root)

	assert.Equal(t, s.root.BillID, key15)
	assert.Equal(t, s.root.Children[0].BillID, key10)
	assert.Equal(t, s.root.Children[0].Children[0].BillID, key1)
	assert.Equal(t, s.root.Children[0].Children[1].BillID, key12)
	assert.Equal(t, s.root.Children[1].BillID, key20)
	assert.Equal(t, s.root.Children[1].Children[1].BillID, key30)
}

func TestAddBill_AVLTreeRotateRightLeft(t *testing.T) {
	s, _ := New(crypto.SHA256)

	put(key10, newBillContent(10), nil, &s.root)
	put(key30, newBillContent(30), nil, &s.root)
	put(key20, newBillContent(20), nil, &s.root)

	put(key25, newBillContent(25), nil, &s.root)
	put(key31, newBillContent(31), nil, &s.root)
	put(key24, newBillContent(24), nil, &s.root)

	assert.Equal(t, s.root.BillID, key25)
	assert.Equal(t, s.root.Children[0].BillID, key20)
	assert.Equal(t, s.root.Children[0].Children[0].BillID, key10)
	assert.Equal(t, s.root.Children[0].Children[1].BillID, key24)
	assert.Equal(t, s.root.Children[1].BillID, key30)
	assert.Equal(t, s.root.Children[1].Children[1].BillID, key31)
}

func TestGetBillNode_LeftChild(t *testing.T) {
	s, _ := New(crypto.SHA256)

	put(key1, newBillContent(1), nil, &s.root)
	put(key2, newBillContent(2), nil, &s.root)
	put(key3, newBillContent(3), nil, &s.root)

	node, found := s.getBill(key1)
	assert.True(t, found)
	assert.NotNil(t, node)
	assert.Equal(t, key1, node.BillID)
	assert.Equal(t, uint32(1), node.Bill.Value)
	assert.Nil(t, node.Children[0])
	assert.Nil(t, node.Children[1])

}

func TestGetBillNode_RightChild(t *testing.T) {
	s, _ := New(crypto.SHA256)

	put(key1, newBillContent(1), nil, &s.root)
	put(key2, newBillContent(2), nil, &s.root)
	put(key3, newBillContent(3), nil, &s.root)

	node, found := s.getBill(key3)
	assert.True(t, found)
	assert.NotNil(t, node)
	assert.Equal(t, key3, node.BillID)
	assert.Equal(t, uint32(3), node.Bill.Value)
	assert.Nil(t, node.Children[0])
	assert.Nil(t, node.Children[1])
}

func TestGetBillNode_NotFound(t *testing.T) {
	s, _ := New(crypto.SHA256)

	put(key1, newBillContent(1), nil, &s.root)
	put(key2, newBillContent(2), nil, &s.root)
	put(key3, newBillContent(3), nil, &s.root)

	node, found := s.getBill(key4)
	assert.False(t, found)
	assert.Nil(t, node)
}

func TestState_GetRootHash(t *testing.T) {
	s, _ := New(crypto.SHA256)

	k1 := s.addBill(newBillContent(10))
	k2 := s.addBill(newBillContent(20))
	k3 := s.addBill(newBillContent(30))

	root := s.GetRootHash()
	assert.NotNil(t, root)

	bill1, _ := s.getBill(k1)
	bill2, _ := s.getBill(k2)
	bill3, _ := s.getBill(k3)
	assert.False(t, bill1.recompute)
	assert.False(t, bill2.recompute)
	assert.False(t, bill3.recompute)

	//TODO assert root hash
}

func TestState_ProcessNilPayment(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(10))

	err := s.Process(nil)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentOrder, err)
}

func TestState_ProcessPaymentWithUnknownType(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(10))
	n, _ := s.getBill(uint64(1))

	payment := newTransferOrder(n.BillID, n.Bill.Backlink, []byte{0x1})
	payment.Type = 10
	err := s.Process(payment)

	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentType, err)
}

func TestState_ProcessTransferOrder_Ok(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(10))
	s.addBill(newBillContent(20))
	s.addBill(newBillContent(30))

	s.GetRootHash()

	n, _ := s.getBill(uint64(1))
	err := s.Process(newTransferOrder(n.BillID, n.Bill.Backlink, []byte{0x1}))

	assert.NoError(t, err)
	n, _ = s.getBill(uint64(1))
	s.GetRootHash()
	assert.NotNil(t, n)
}

func TestState_ProcessTransferOrder_AmountPresent(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(10))

	n, _ := s.getBill(uint64(1))

	order := newTransferOrder(n.BillID, n.Bill.Backlink, []byte{0x1})
	order.Amount = uint32(10)

	err := s.Process(order)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentAmount, err)
}

func TestState_ProcessTransferOrder_BillNotFound(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(10))

	n, _ := s.getBill(uint64(1))

	order := newTransferOrder(uint64(2), n.Bill.Backlink, []byte{0x1})

	err := s.Process(order)
	assert.Error(t, err)
	assert.Equal(t, ErrBillNotFound, err)
}

func TestState_ProcessTransferOrder_InvalidBacklink(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(10))

	n, _ := s.getBill(uint64(1))

	order := newTransferOrder(n.BillID, []byte("invalid"), []byte{0x1})

	err := s.Process(order)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentBacklink, err)
}

func TestState_ProcessSplitOrder_BillNotFound(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(10))

	n, _ := s.getBill(uint64(1))

	order := newSplitOrder(uint64(2), n.Bill.Backlink, []byte{0x1}, 1)

	err := s.Process(order)
	assert.Error(t, err)
	assert.Equal(t, ErrBillNotFound, err)
}

func TestState_ProcessSplitOrder_InvalidBacklink(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(10))

	n, _ := s.getBill(uint64(1))

	order := newSplitOrder(n.BillID, []byte("invalid"), []byte{0x1}, 1)

	err := s.Process(order)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentBacklink, err)
}

func TestState_ProcessSplitOrder_AmountInvalid(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(10))

	n, _ := s.getBill(uint64(1))

	order := newSplitOrder(n.BillID, n.Bill.Backlink, []byte{0x1}, 11)

	err := s.Process(order)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentAmount, err)
}

func TestState_ProcessSplitOrder_Ok(t *testing.T) {
	s, _ := New(crypto.SHA256)
	s.addBill(newBillContent(10))
	n, _ := s.getBill(uint64(1))

	order := newSplitOrder(n.BillID, n.Bill.Backlink, []byte{0x1}, 6)

	err := s.Process(order)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), s.maxBillID)

	s.GetRootHash()

	n, _ = s.getBill(uint64(1))
	assert.NotNil(t, n)

	n2, _ := s.getBill(uint64(2))
	assert.NotNil(t, n)

	assert.Equal(t, uint32(10), n.Bill.TotalValue)
	assert.Equal(t, uint32(4), n.Bill.Value)
	assert.NotEqual(t, order.PayeePredicate, n.Bill.BearerPredicate)

	assert.Equal(t, uint32(6), n2.Bill.TotalValue)
	assert.Equal(t, uint32(6), n2.Bill.Value)
	assert.Equal(t, order.PayeePredicate, n2.Bill.BearerPredicate)
}

func TestBillContent_CalculateStateHash_StateIsNotChanged(t *testing.T) {
	bc := newBillContent(10)
	hash := bc.calculateStateHash(nil, crypto.SHA256.New())
	assert.Equal(t, bc.StateHash, hash)
}

func TestBillContent_CalculateStateHash_TransferBill(t *testing.T) {
	bc := newBillContent(10)
	oldStateHash := bc.StateHash
	transfer := newTransferOrder(uint64(1), bc.Backlink, []byte{1})
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

func newTransferOrder(billID uint64, backlink []byte, newPredicate Predicate) *PaymentOrder {
	return newPaymentOrder(PaymentTypeTransfer, billID, backlink, newPredicate, 0)
}

func newSplitOrder(billID uint64, backlink []byte, newPredicate Predicate, amount uint32) *PaymentOrder {
	return newPaymentOrder(PaymentTypeSplit, billID, backlink, newPredicate, amount)
}

func newPaymentOrder(t PaymentType, billID uint64, backlink []byte, payeePredicate Predicate, amount uint32) *PaymentOrder {
	return &PaymentOrder{
		BillID:            billID,
		Type:              t,
		Amount:            amount,
		PayeePredicate:    payeePredicate,
		Backlink:          backlink,
		PredicateArgument: []byte{},
	}
}

func newBillContent(v uint32) *BillContent {
	return &BillContent{
		Value:           v,
		Backlink:        make([]byte, 32),
		StateHash:       make([]byte, 32),
		BearerPredicate: make([]byte, 32),
	}
}
