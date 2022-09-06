package backend

import (
	"sync"

	"github.com/holiman/uint256"
)

type InmemoryBillStore struct {
	blockNumber uint64
	bills       map[string]map[string]*bill // pubkey => map[bill_id]*bill
	proofs      map[string]*blockProof      // bill_id => block_proof

	mu sync.Mutex
}

func NewInmemoryBillStore() *InmemoryBillStore {
	return &InmemoryBillStore{bills: map[string]map[string]*bill{}, proofs: map[string]*blockProof{}}
}

func (s *InmemoryBillStore) GetBlockNumber() (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blockNumber, nil
}

func (s *InmemoryBillStore) SetBlockNumber(blockNumber uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blockNumber = blockNumber
	return nil
}

func (s *InmemoryBillStore) GetBills(pubkey []byte) ([]*bill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var bills []*bill
	for _, b := range s.bills[string(pubkey)] {
		bills = append(bills, b)
	}
	return bills, nil
}

func (s *InmemoryBillStore) AddBill(pubKey []byte, b *bill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bills, f := s.bills[string(pubKey)]
	if !f {
		bills = map[string]*bill{}
		s.bills[string(pubKey)] = bills
	}
	b32 := b.Id.Bytes32()
	bills[string(b32[:])] = b
	return nil
}

func (s *InmemoryBillStore) RemoveBill(pubKey []byte, id *uint256.Int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bills := s.bills[string(pubKey)]
	b32 := id.Bytes32()
	delete(bills, string(b32[:]))
	return nil
}

func (s *InmemoryBillStore) ContainsBill(pubKey []byte, id *uint256.Int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bills := s.bills[string(pubKey)]
	b32 := id.Bytes32()
	_, ok := bills[string(b32[:])]
	return ok, nil
}

func (s *InmemoryBillStore) GetBlockProof(billId []byte) (*blockProof, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.proofs[string(billId)], nil
}

func (s *InmemoryBillStore) SetBlockProof(proof *blockProof) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.proofs[string(proof.BillId)] = proof
	return nil
}
