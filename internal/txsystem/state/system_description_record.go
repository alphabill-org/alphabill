package state

import "hash"

// SystemDescriptionRecord Every transaction system registered in the AlphaBill framework is described with given data structure
type SystemDescriptionRecord struct {
	Name string

	// TODO add remaining functionality from spec 2.3 System Description Record after testnet v1
}

func NewSystemDescriptionRecord(name string) *SystemDescriptionRecord {
	return &SystemDescriptionRecord{Name: name}
}

func (s *SystemDescriptionRecord) AddToHasher(hasher hash.Hash) {
	hasher.Write([]byte(s.Name))
}
