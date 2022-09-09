package backend

import (
	"sync"

	"github.com/holiman/uint256"
)

type InmemoryBillStore struct {
	blockNumber uint64
	bills       map[string]map[string]*Bill // pubkey => map[bill_id]*Bill
	proofs      map[string]*BlockProof      // bill_id => block_proof
	keys        map[string]*Pubkey          // pubkey => hashed pubkeys

	mu sync.Mutex
}

func NewInmemoryBillStore() *InmemoryBillStore {
	return &InmemoryBillStore{
		bills:  map[string]map[string]*Bill{},
		proofs: map[string]*BlockProof{},
		keys:   map[string]*Pubkey{},
	}
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

func (s *InmemoryBillStore) GetBills(pubkey []byte) ([]*Bill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var bills []*Bill
	for _, b := range s.bills[string(pubkey)] {
		bills = append(bills, b)
	}
	return bills, nil
}

func (s *InmemoryBillStore) AddBill(pubKey []byte, b *Bill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bills, f := s.bills[string(pubKey)]
	if !f {
		bills = map[string]*Bill{}
		s.bills[string(pubKey)] = bills
	}
	b32 := b.Id.Bytes32()
	bills[string(b32[:])] = b
	return nil
}

func (s *InmemoryBillStore) AddBillWithProof(pubKey []byte, b *Bill, p *BlockProof) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bills, f := s.bills[string(pubKey)]
	if !f {
		bills = map[string]*Bill{}
		s.bills[string(pubKey)] = bills
	}
	b32 := b.Id.Bytes32()
	bId := string(b32[:])
	bills[bId] = b

	s.proofs[bId] = p

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

func (s *InmemoryBillStore) GetBlockProof(billId []byte) (*BlockProof, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.proofs[string(billId)], nil
}

func (s *InmemoryBillStore) SetBlockProof(proof *BlockProof) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b32 := proof.BillId.Bytes32()
	s.proofs[string(b32[:])] = proof
	return nil
}

func (s *InmemoryBillStore) GetKeys() ([]*Pubkey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var keys []*Pubkey
	for _, k := range s.keys {
		keys = append(keys, k)
	}
	return keys, nil
}

func (s *InmemoryBillStore) AddKey(k *Pubkey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, f := s.keys[string(k.Pubkey)]
	if !f {
		s.keys[string(k.Pubkey)] = k
	}
	return nil
}
