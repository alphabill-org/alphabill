package backend

import "github.com/holiman/uint256"

type InmemoryWalletBackendStore struct {
	blockNumber uint64
	bills       map[string]map[string]*bill // pubkey => map[bill_id]*bill
	proofs      map[string]*blockProof      // bill_id => block_proof
}

func NewInmemoryWalletBackendStore() *InmemoryWalletBackendStore {
	return &InmemoryWalletBackendStore{bills: map[string]map[string]*bill{}, proofs: map[string]*blockProof{}}
}

func (s *InmemoryWalletBackendStore) GetBlockNumber() uint64 {
	return s.blockNumber
}

func (s *InmemoryWalletBackendStore) GetBills(pubkey []byte) []*bill {
	var bills []*bill
	for _, b := range s.bills[string(pubkey)] {
		bills = append(bills, b)
	}
	return bills
}

func (s *InmemoryWalletBackendStore) GetBlockProof(unitId []byte) *blockProof {
	return s.proofs[string(unitId)]
}

func (s *InmemoryWalletBackendStore) SetBlockNumber(blockNumber uint64) {
	s.blockNumber = blockNumber
}

func (s *InmemoryWalletBackendStore) SetBlockProof(proof *blockProof) {
	s.proofs[string(proof.BillId)] = proof
}

func (s *InmemoryWalletBackendStore) SetBill(pubKey []byte, b *bill) {
	bills, f := s.bills[string(pubKey)]
	if !f {
		bills = map[string]*bill{}
		s.bills[string(pubKey)] = bills
	}
	b32 := b.Id.Bytes32()
	bills[string(b32[:])] = b
}

func (s *InmemoryWalletBackendStore) RemoveBill(key *pubkey, id *uint256.Int) {
	bills := s.bills[string(key.pubkey)]
	b32 := id.Bytes32()
	delete(bills, string(b32[:]))
}

func (s *InmemoryWalletBackendStore) ContainsBill(key *pubkey, id *uint256.Int) bool {
	bills := s.bills[string(key.pubkey)]
	b32 := id.Bytes32()
	_, ok := bills[string(b32[:])]
	return ok
}
