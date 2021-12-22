package state

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"
	"hash"

	"github.com/holiman/uint256"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/logger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/script"
)

var (
	ErrBillNotFound           = errors.New("bill not found")
	ErrInvalidPaymentAmount   = errors.New("invalid payment amount")
	ErrInvalidPaymentBacklink = errors.New("invalid payment backlink")
	ErrInvalidPaymentOrder    = errors.New("invalid payment order")
	ErrInvalidPaymentType     = errors.New("invalid payment type")
	ErrInvalidHashAlgorithm   = errors.New("invalid hash algorithm")
)

var log = logger.CreateForPackage()

type (
	GenericTransaction interface {
		UnitId() *uint256.Int
		Timeout() uint64
		OwnerProof() []byte
		Hash(hashFunc crypto.Hash) []byte
	}

	Transfer interface {
		GenericTransaction
		NewBearer() []byte
		Backlink() []byte
		TargetValue() uint64
	}

	DustTransfer interface {
		GenericTransaction
		NewBearer() []byte
		Backlink() []byte
		Nonce() []byte
		TargetValue() uint64
	}

	Split interface {
		GenericTransaction
		Amount() uint64
		TargetBearer() []byte
		RemainingValue() uint64
		Backlink() []byte
	}

	Swap interface {
		GenericTransaction
		OwnerCondition() []byte
		BillIdentifiers() []*uint256.Int
		DustTransfers() []DustTransfer
		Proofs() [][]byte
		TargetValue() uint64
	}

	// State holds the state of all bills.
	State struct {
		root          *Node       // root node
		roundNumber   uint64      // number of the round
		maxBillID     uint64      // maximum bill identifier
		hashAlgorithm crypto.Hash // hash function algorithm
	}

	// Node is a single element within the State.
	Node struct {
		ID        uint64       // ID of the node
		Bill      *BillContent // BillContent contains bill related information
		Parent    *Node        // Parent node
		Children  [2]*Node     // Children nodes
		Hash      []byte       // Hash of the node
		recompute bool         // true if node content (hash, value, total value, etc) needs recalculation
		balance   int8         // balance factor
	}

	// BillContent contains bill related information.
	BillContent struct {
		Value           uint32           // value of the given bill
		TotalValue      uint32           // total value
		StateHash       []byte           // state hash of the bill state
		Backlink        []byte           // pre-calculated backlink value
		BearerPredicate domain.Predicate // bearer predicate
	}
)

// New instantiates a new empty state with given hash function and the initial state.
// Initial state may be nil, but then the state is empty and not usable.
func New(hashAlgorithm crypto.Hash, initialState []*BillContent) (*State, error) {
	if hashAlgorithm != crypto.SHA256 && hashAlgorithm != crypto.SHA512 {
		return nil, ErrInvalidHashAlgorithm
	}

	s := &State{hashAlgorithm: hashAlgorithm}

	for _, bc := range initialState {
		s.addBill(bc)
	}

	return s, nil
}

// NewInitialBill creates BillContent for creating initial state.
func NewInitialBill(value uint32, bearerPredicate domain.Predicate) *BillContent {
	return &BillContent{
		Value:           value,
		BearerPredicate: bearerPredicate,
	}
}

// Process processes the transaction. Will return an error if the transaction type is unknown or validation fails.
func (s *State) Process(gtx GenericTransaction) error {
	switch tx := gtx.(type) {
	case Transfer:
		log.Debug("Processing transfer %v", tx)
		return nil
	// TODO ... other types
	default:
		return errors.New(fmt.Sprintf("Unknown type %T", gtx))
	}
}

func (s *State) ProcessPayment(payment *domain.PaymentOrder) error {
	if payment == nil {
		return ErrInvalidPaymentOrder
	}
	switch payment.Type {
	case domain.PaymentTypeTransfer:
		return s.processTransfer(payment)
	case domain.PaymentTypeSplit:
		return s.processSplit(payment)
	}
	return ErrInvalidPaymentType
}

// GetRootHash returns the root hash of the State.
func (s *State) GetRootHash() []byte {
	s.recompute(s.root, s.hashAlgorithm.New())
	return s.root.Hash
}

func (s *State) processTransfer(payment *domain.PaymentOrder) error {
	if payment.Amount != 0 {
		return ErrInvalidPaymentAmount
	}

	b, found := s.getBill(payment.BillID)
	if !found {
		return ErrBillNotFound
	}

	if !bytes.Equal(b.Backlink, payment.Backlink) {
		return ErrInvalidPaymentBacklink
	}

	err := script.RunScript(payment.PredicateArgument[:], b.BearerPredicate[:], payment.Bytes()[:])
	if err != nil {
		return err
	}

	hasher := s.hashAlgorithm.New()
	paymentHash := payment.Hash(hasher)
	hasher.Reset()
	b.Backlink = paymentHash[:]
	b.calculateStateHash(payment, hasher)
	b.BearerPredicate = payment.PayeePredicate

	return s.updateBill(payment.BillID, b)
}

func (s *State) processSplit(payment *domain.PaymentOrder) error {
	b, found := s.getBill(payment.BillID)
	if !found {
		return ErrBillNotFound
	}
	if !bytes.Equal(b.Backlink, payment.Backlink) {
		return ErrInvalidPaymentBacklink
	}
	amount := payment.Amount
	if b.Value < amount {
		return ErrInvalidPaymentAmount
	}

	err := script.RunScript(payment.PredicateArgument[:], b.BearerPredicate[:], payment.Bytes()[:])
	if err != nil {
		return err
	}

	hasher := s.hashAlgorithm.New()
	paymentHash := payment.Hash(hasher)
	hasher.Reset()

	b.Backlink = paymentHash[:]
	b.calculateStateHash(payment, hasher)
	b.Value = b.Value - payment.Amount

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
	_, found := s.getBill(billID)
	if found {
		put(billID, content, nil, &s.root)
		return nil
	}
	return ErrBillNotFound
}

// getBill searches the bill in the state by billID and returns its content or nil if bill is not found in state.
// Second return parameter is true if bill was found, otherwise false.
func (s *State) getBill(billID uint64) (*BillContent, bool) {
	node, b := getNode(s.root, billID)
	if !b {
		return nil, b
	}
	return node.Bill, b
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
		hasher.Write(domain.Uint64ToBytes(n.ID))

		// write bill value
		hasher.Write(domain.Uint32ToBytes(n.Bill.Value))

		// write bill state hash
		hasher.Write(n.Bill.StateHash)

		// write left child hash
		hasher.Write(leftHash)

		// write left child totalValue
		hasher.Write(domain.Uint32ToBytes(leftTotalValue))

		// write right child hash
		hasher.Write(rightHash)

		// write right child totalValue
		hasher.Write(domain.Uint32ToBytes(rightTotalValue))

		n.Hash = hasher.Sum(nil)
		hasher.Reset()
		n.recompute = false
	}
}

func (c *BillContent) calculateStateHash(payment *domain.PaymentOrder, hasher hash.Hash) []byte {
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
	m := fmt.Sprintf("ID=%v, ", n.ID)
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
