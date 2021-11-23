package state

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
	"hash"
)

var (
	ErrBillNotFound           = errors.New("bill not found")
	ErrInvalidPaymentAmount   = errors.New("invalid payment amount")
	ErrInvalidPaymentBacklink = errors.New("invalid payment backlink")
	ErrInvalidPaymentOrder    = errors.New("invalid payment order")
)

type (

	// State holds the state of all bills.
	State struct {
		root        *Node  // root node
		roundNumber uint64 // number of the round
		maxBillID   uint64 // maximum bill identifier
	}

	// Node is a single element within the State.
	Node struct {
		BillID    uint64       // BillID of the node/bill
		Bill      *BillContent // BillContent contains bill related information
		Parent    *Node        // Parent node
		Children  [2]*Node     // Children nodes
		Hash      []byte       // Hash of the node
		recompute bool         // true if node content (hash, value, total value, etc) needs recalculation
		balance   int8         // balance factor
	}

	// BillContent contains bill related information.
	BillContent struct {
		Value           uint32          // value of the given bill
		TotalValue      uint32          // total value
		StateHash       []byte          // state hash of the bill state
		Backlink        []byte          // backlink to the payment/emission order
		BearerPredicate Predicate       // bearer predicate
		PaymentOrder    *PaymentOrder   // payment or emission order
		JoinOrders      []*PaymentOrder // incoming join orders
	}

	Predicate []byte
)

// New instantiates a new empty state.
func New() *State {
	return &State{}
}

// Process validates and processes a payment order.
func (s *State) Process(payment PaymentOrder) error {
	switch payment.Type {
	case PaymentTypeTransfer:
		return s.processTransfer(payment)
	case PaymentTypeSplit:
		if payment.JoinBillId > 0 {
			return ErrInvalidPaymentOrder
		}
		// TODO
	case PaymentTypeJoin:
		//TODO implement
	}

	return errors.New("not implemented")
}

// GetRootHash returns the root hash of the State.
func (s *State) GetRootHash() []byte {
	s.recompute(s.root, sha256.New())
	return s.root.Hash
}

func (s *State) processTransfer(payment PaymentOrder) error {
	if payment.JoinBillId > 0 {
		return ErrInvalidPaymentOrder
	}
	if payment.Amount != 0 {
		return ErrInvalidPaymentAmount
	}

	b, found := s.getBill(payment.BillID)
	if !found {
		return ErrBillNotFound
	}

	if b.Bill.PaymentOrder != nil || !bytes.Equal(b.Bill.Backlink, payment.Backlink) {
		return ErrInvalidPaymentBacklink
	}

	err := script.RunScript(payment.PredicateArgument[:], b.Bill.BearerPredicate[:], payment.SigBytes()[:])
	if err != nil {
		return err
	}

	b.Bill.PaymentOrder = &payment
	b.Bill.BearerPredicate = payment.PayeePredicate
	return s.updateBill(payment.BillID, b.Bill)
}

// addBill inserts a new bill to the state. Return parameter is the bill ID.
func (s *State) addBill(content *BillContent) (billID uint64) {
	s.maxBillID++
	billID = s.maxBillID
	put(billID, content, nil, &s.root)
	return
}

// updateBill updates bill with given billID.
func (s *State) updateBill(billID uint64, content *BillContent) error {
	_, found := getNode(s, billID)
	if found {
		put(billID, content, nil, &s.root)
		return nil
	}
	return ErrBillNotFound
}

// removeBill removes the bill from the state by billID.
func (s *State) removeBill(billID uint64) {
	remove(billID, &s.root)
}

// getBill searches the bill in the state by billID and returns its value or nil if bill is not found in state.
// Second return parameter is true if bill was found, otherwise false.
func (s *State) getBill(billID uint64) (value *Node, found bool) {
	return getNode(s, billID)
}

// empty returns true if state does not contain any nodes/bills.
func (s *State) empty() bool {
	return s.root == nil
}

// recompute recalculates the parameters of the bill.
func (s *State) recompute(n *Node, hasher hash.Hash) {
	if n.recompute {
		leftTotalValue := uint32(0)
		rightTotalValue := uint32(0)
		leftHash := make([]byte, hasher.Size())
		rightHash := make([]byte, hasher.Size())

		var left = n.Children[0]
		var right = n.Children[1]
		if left != nil {
			s.recompute(left, hasher)
			leftTotalValue = left.Bill.TotalValue
			leftHash = left.Hash
		}
		if right != nil {
			s.recompute(right, hasher)
			rightTotalValue = right.Bill.TotalValue
			rightHash = right.Hash
		}

		// TODO joins
		n.Bill.TotalValue = n.Bill.Value + leftTotalValue + rightTotalValue

		billStateHash := n.Bill.calculateStateHash(hasher)
		n.Bill.StateHash = billStateHash
		hasher.Reset()

		// write bill ID
		hasher.Write(Uint64ToBytes(n.BillID))

		// write bill value
		hasher.Write(Uint32ToBytes(n.Bill.Value))

		// write bill state hash
		hasher.Write(billStateHash)

		// write left child hash
		hasher.Write(leftHash)

		// write left child totalValue
		hasher.Write(Uint32ToBytes(leftTotalValue))

		// write right child hash
		hasher.Write(rightHash)

		// write right child totalValue
		hasher.Write(Uint32ToBytes(rightTotalValue))

		n.Hash = hasher.Sum(nil)
		hasher.Reset()
		n.recompute = false
		n.Bill.PaymentOrder = nil
		n.Bill.JoinOrders = nil
	}
}

func (c *BillContent) calculateStateHash(hasher hash.Hash) []byte {
	ordersHash := c.calculateOrdersHash(hasher)
	if ordersHash != nil {
		hasher.Write(c.StateHash)
		hasher.Write(ordersHash)
		c.StateHash = hasher.Sum(nil)
	}
	return c.StateHash
}

func (c *BillContent) calculateOrdersHash(hasher hash.Hash) []byte {
	calculateOrdersHash := false
	if c.PaymentOrder != nil {
		hasher.Write(c.PaymentOrder.Bytes())
		calculateOrdersHash = true
	}
	if c.JoinOrders != nil && len(c.JoinOrders) > 0 {
		calculateOrdersHash = true
		for _, order := range c.JoinOrders {
			hasher.Write(order.Bytes())
		}
	}
	var ordersHash []byte = nil
	if calculateOrdersHash {
		ordersHash = hasher.Sum(nil)
		hasher.Reset()
	}
	return ordersHash
}

// String returns a string representation of state.
func (s *State) String() string {
	str := "State\n"
	if !s.empty() {
		output(s.root, "", true, &str)
	}
	return str
}

// String returns a string representation of the node
func (n *Node) String() string {
	m := fmt.Sprintf("ID=%v, ", n.BillID)
	if n.recompute {
		m = m + "*"
	}
	m = m + fmt.Sprintf("value=%v, total=%v, backlink=%X, stateHash=%X, bearerPredicate=%X, ",
		n.Bill.Value, n.Bill.TotalValue, n.Bill.Backlink, n.Bill.StateHash, n.Bill.BearerPredicate)

	if n.Hash != nil {
		m = m + fmt.Sprintf("hash=%X", n.Hash)
	}

	return m
}
