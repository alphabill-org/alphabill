package backend

import (
	"sync"
)

type InmemoryBillStore struct {
	blockNumber uint64
	pubkeyIndex map[string]map[string]bool // pubkey => map[bill_id]blank
	bills       map[string]*Bill           // bill_id => bill
	keys        map[string]*Pubkey         // pubkey => hashed pubkeys

	mu sync.Mutex
}

func NewInmemoryBillStore() *InmemoryBillStore {
	return &InmemoryBillStore{
		pubkeyIndex: map[string]map[string]bool{},
		bills:       map[string]*Bill{},
		keys:        map[string]*Pubkey{},
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
	for k := range s.pubkeyIndex[string(pubkey)] {
		bills = append(bills, s.bills[k])
	}
	return bills, nil
}

func (s *InmemoryBillStore) RemoveBill(pubKey []byte, id []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bills := s.pubkeyIndex[string(pubKey)]
	delete(bills, string(id))
	delete(s.bills, string(id))
	return nil
}

func (s *InmemoryBillStore) ContainsBill(id []byte) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.bills[string(id)]
	return exists, nil
}

func (s *InmemoryBillStore) GetBill(billId []byte) (*Bill, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bills[string(billId)], nil
}

func (s *InmemoryBillStore) SetBills(pubkey []byte, billsIn ...*Bill) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, bill := range billsIn {
		s.bills[string(bill.Id)] = bill
		bills, f := s.pubkeyIndex[string(pubkey)]
		if !f {
			bills = map[string]bool{}
			s.pubkeyIndex[string(pubkey)] = bills
		}
		bills[string(bill.Id)] = true
	}
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
