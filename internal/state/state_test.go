package state

import (
	"crypto"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
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

func TestCreateEmptyState_Ok(t *testing.T) {
	s := New()
	assert.Equal(t, uint64(0), s.maxBillID)
	assert.Equal(t, uint64(0), s.roundNumber)
	assert.Nil(t, s.root)
	assert.True(t, s.empty())
}

func TestAddBills_Ok(t *testing.T) {
	s := New()
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
	s := New()
	s.addBill(newBillContent(1))
	s.addBill(newBillContent(1))

	err := s.updateBill(uint64(2), newBillContent(10))
	assert.NoError(t, err)
}

func TestUpdateNodeContent_BillNotPresent(t *testing.T) {
	s := New()
	s.addBill(newBillContent(1))
	s.addBill(newBillContent(1))

	err := s.updateBill(uint64(10), newBillContent(10))
	assert.Error(t, err)
	assert.Equal(t, ErrBillNotFound, err)
}

func TestAddBill_AVLTreeRotateLeft(t *testing.T) {
	s := New()

	put(key1, newBillContent(1), nil, &s.root)
	put(key2, newBillContent(2), nil, &s.root)
	put(key3, newBillContent(3), nil, &s.root)

	assert.Equal(t, s.root.BillID, key2)
	assert.Equal(t, s.root.Children[0].BillID, key1)
	assert.Equal(t, s.root.Children[1].BillID, key3)
}

func TestAddBill_AVLTreeRotateRight(t *testing.T) {
	s := New()

	put(key3, newBillContent(3), nil, &s.root)
	put(key2, newBillContent(2), nil, &s.root)
	put(key1, newBillContent(1), nil, &s.root)

	assert.Equal(t, s.root.BillID, key2)
	assert.Equal(t, s.root.Children[0].BillID, key1)
	assert.Equal(t, s.root.Children[1].BillID, key3)
}

func TestAddBill_AVLTreeRotateLeftRight(t *testing.T) {
	s := New()

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
	s := New()

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
	s := New()

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
	s := New()

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
	s := New()

	put(key1, newBillContent(1), nil, &s.root)
	put(key2, newBillContent(2), nil, &s.root)
	put(key3, newBillContent(3), nil, &s.root)

	node, found := s.getBill(key4)
	assert.False(t, found)
	assert.Nil(t, node)
}

func TestRemoveRootBill(t *testing.T) {
	s := New()
	put(key1, newBillContent(10), nil, &s.root)
	put(key2, newBillContent(20), nil, &s.root)
	put(key3, newBillContent(30), nil, &s.root)

	assert.Equal(t, s.root.BillID, key2)
	assert.Equal(t, s.root.Children[0].BillID, key1)
	assert.Equal(t, s.root.Children[1].BillID, key3)

	s.removeBill(key2)

	assert.NotNil(t, s.GetRootHash())
	assert.Equal(t, s.root.BillID, key3)
	assert.Equal(t, s.root.Children[0].BillID, key1)
	assert.Equal(t, uint32(40), s.root.Bill.TotalValue)
}

func TestRemoveBill_Left(t *testing.T) {
	s := New()
	put(key1, newBillContent(10), nil, &s.root)
	put(key2, newBillContent(20), nil, &s.root)
	put(key3, newBillContent(30), nil, &s.root)

	assert.Equal(t, s.root.BillID, key2)
	assert.Equal(t, s.root.Children[0].BillID, key1)
	assert.Equal(t, s.root.Children[1].BillID, key3)

	s.removeBill(key1)
	assert.NotNil(t, s.GetRootHash())
	assert.Equal(t, s.root.BillID, key2)
	assert.Equal(t, s.root.Children[1].BillID, key3)
	assert.Equal(t, uint32(50), s.root.Bill.TotalValue)
}

func TestRemoveBill_Right(t *testing.T) {
	s := New()

	put(key1, newBillContent(10), nil, &s.root)
	put(key2, newBillContent(20), nil, &s.root)
	put(key3, newBillContent(30), nil, &s.root)

	assert.Equal(t, s.root.BillID, key2)
	assert.Equal(t, s.root.Children[0].BillID, key1)
	assert.Equal(t, s.root.Children[1].BillID, key3)

	s.removeBill(key3)
	assert.NotNil(t, s.GetRootHash())
	assert.Equal(t, s.root.BillID, key2)
	assert.Equal(t, s.root.Children[0].BillID, key1)
	assert.Equal(t, uint32(30), s.root.Bill.TotalValue)
}

func TestState_GetRootHash(t *testing.T) {
	s := New()

	s.addBill(newBillContent(10))
	s.addBill(newBillContent(20))
	s.addBill(newBillContent(30))

	root := s.GetRootHash()
	assert.NotNil(t, root)
	// TODO assert root hash
}

func TestCalculateBillStateHash_BillNotChanged(t *testing.T) {
	bc := newBillContent(100)
	hash := bc.calculateStateHash(crypto.SHA256.New())

	assert.NotNil(t, hash)
	assert.Equal(t, make([]byte, 32), hash)
}

func TestState_ProcessTransferOrder(t *testing.T) {
	s := New()
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

func TestState_ProcessTransferOrder_JoinBillIDPresent(t *testing.T) {
	s := New()
	s.addBill(newBillContent(10))

	n, _ := s.getBill(uint64(1))

	order := newTransferOrder(n.BillID, n.Bill.Backlink, []byte{0x1})
	order.JoinBillId = uint64(2)

	err := s.Process(order)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentOrder, err)
}

func TestState_ProcessTransferOrder_AmountPresent(t *testing.T) {
	s := New()
	s.addBill(newBillContent(10))

	n, _ := s.getBill(uint64(1))

	order := newTransferOrder(n.BillID, n.Bill.Backlink, []byte{0x1})
	order.Amount = uint32(10)

	err := s.Process(order)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentAmount, err)
}

func TestState_ProcessTransferOrder_BillNotFound(t *testing.T) {
	s := New()
	s.addBill(newBillContent(10))

	n, _ := s.getBill(uint64(1))

	order := newTransferOrder(uint64(2), n.Bill.Backlink, []byte{0x1})

	err := s.Process(order)
	assert.Error(t, err)
	assert.Equal(t, ErrBillNotFound, err)
}

func TestState_ProcessTransferOrder_InvalidBacklink(t *testing.T) {
	s := New()
	s.addBill(newBillContent(10))

	n, _ := s.getBill(uint64(1))

	order := newTransferOrder(n.BillID, []byte("invalid"), []byte{0x1})

	err := s.Process(order)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidPaymentBacklink, err)
}

func TestBillContent_CalculateStateHash_StateIsNotChanged(t *testing.T) {
	bc := newBillContent(10)
	hash := bc.calculateStateHash(crypto.SHA256.New())
	assert.Equal(t, bc.StateHash, hash)
}

func TestBillContent_CalculateStateHash_NewPayment(t *testing.T) {
	bc := newBillContent(10)
	oldStateHash := bc.StateHash
	transfer := newTransferOrder(uint64(1), bc.Backlink, []byte{1})
	bc.PaymentOrder = &transfer
	hash := bc.calculateStateHash(crypto.SHA256.New())

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

func TestBillContent_CalculateStateHash_JoinsAndPayment(t *testing.T) {
	t.Skip("Add Join Payment type!")
}

func newTransferOrder(billID uint64, backlink []byte, newPredicate Predicate) PaymentOrder {
	return PaymentOrder{
		BillID:            billID,
		Type:              PaymentTypeTransfer,
		JoinBillId:        0,
		Amount:            0,
		PayeePredicate:    newPredicate,
		Backlink:          backlink,
		PredicateArgument: []byte{0x53},
	}

}

func newBillContent(v uint32) *BillContent {
	return &BillContent{
		Value:           v,
		Backlink:        make([]byte, 32),
		StateHash:       make([]byte, 32),
		BearerPredicate: []byte{script.StartByte, script.OP_PUSH_BOOL, 0x01}, // always true predicate
	}
}
