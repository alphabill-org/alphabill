package state

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"hash"
)

var (
	ErrBillNotFound           = errors.New("bill not found")
	ErrInvalidPaymentAmount   = errors.New("invalid payment amount")
	ErrInvalidPaymentBacklink = errors.New("invalid payment backlink")
	ErrInvalidPaymentOrder    = errors.New("invalid payment order")
	ErrInvalidPaymentType     = errors.New("invalid payment type")

	ErrInvalidHashAlgorithm = errors.New("invalid hash algorithm")
)

type (

	// State holds the state of all bills.
	State struct {
		root          *Node       // root node
		roundNumber   uint64      // number of the round
		maxBillID     uint64      // maximum bill identifier
		hashAlgorithm crypto.Hash // hash function algorithm
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
		Value           uint32    // value of the given bill
		TotalValue      uint32    // total value
		StateHash       []byte    // state hash of the bill state
		Backlink        []byte    // backlink to the payment/emission order
		BearerPredicate Predicate // bearer predicate

	}

	Predicate []byte
)

// New instantiates a new empty state with given hash function.
func New(hashAlgorithm crypto.Hash) (*State, error) {
	if hashAlgorithm != crypto.SHA256 && hashAlgorithm != crypto.SHA512 {
		return nil, ErrInvalidHashAlgorithm
	}

	return &State{
		hashAlgorithm: hashAlgorithm,
	}, nil
}

// Process validates and processes a payment order.
func (s *State) Process(payment *PaymentOrder) error {
	if payment == nil {
		return ErrInvalidPaymentOrder
	}
	switch payment.Type {
	case PaymentTypeTransfer:
		return s.processTransfer(payment)
	case PaymentTypeSplit:
		return s.processSplit(payment)
	}
	return ErrInvalidPaymentType
}

// GetRootHash returns the root hash of the State.
func (s *State) GetRootHash() []byte {
	s.recompute(s.root, s.hashAlgorithm.New())
	return s.root.Hash
}

func (s *State) processTransfer(payment *PaymentOrder) error {
	if payment.Amount != 0 {
		return ErrInvalidPaymentAmount
	}
	// TODO check predicate

	b, found := s.getBill(payment.BillID)
	if !found {
		return ErrBillNotFound
	}

	if !bytes.Equal(b.Bill.Backlink, payment.Backlink) {
		return ErrInvalidPaymentBacklink
	}
	hasher := s.hashAlgorithm.New()
	paymentHash := payment.Hash(hasher)
	hasher.Reset()
	b.Bill.Backlink = paymentHash[:]
	b.Bill.calculateStateHash(payment, hasher)
	b.Bill.BearerPredicate = payment.PayeePredicate

	return s.updateBill(payment.BillID, b.Bill)
}

func (s *State) processSplit(payment *PaymentOrder) error {
	b, found := s.getBill(payment.BillID)
	if !found {
		return ErrBillNotFound
	}
	if !bytes.Equal(b.Bill.Backlink, payment.Backlink) {
		return ErrInvalidPaymentBacklink
	}
	amount := payment.Amount
	if b.Bill.Value < amount {
		return ErrInvalidPaymentAmount
	}

	hasher := s.hashAlgorithm.New()
	paymentHash := payment.Hash(hasher)
	hasher.Reset()

	b.Bill.Backlink = paymentHash[:]
	b.Bill.calculateStateHash(payment, hasher)
	b.Bill.Value = b.Bill.Value - payment.Amount

	bc := &BillContent{
		Value:           payment.Amount,
		StateHash:       make([]byte, 32),
		Backlink:        paymentHash[:],
		BearerPredicate: payment.PayeePredicate,
	}
	s.addBill(bc)
	return nil
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
		n.Bill.TotalValue = n.Bill.Value + leftTotalValue + rightTotalValue
		hasher.Reset()

		// write bill ID
		hasher.Write(Uint64ToBytes(n.BillID))

		// write bill value
		hasher.Write(Uint32ToBytes(n.Bill.Value))

		// write bill state hash
		hasher.Write(n.Bill.StateHash)

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
	}
}

func (c *BillContent) calculateStateHash(payment *PaymentOrder, hasher hash.Hash) []byte {
	if payment == nil {
		return c.StateHash
	}
	// calculate payment order hash
	hasher.Write(payment.Bytes())
	paymentHash := hasher.Sum(nil)
	hasher.Reset()
	// calculate state hash
	hasher.Write(c.StateHash)
	hasher.Write(paymentHash)
	c.StateHash = hasher.Sum(nil)
	return c.StateHash
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
