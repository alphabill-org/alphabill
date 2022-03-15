package unicitytree

import "hash"

// SystemDescriptionRecord describes transaction system in Alphabill framework
type SystemDescriptionRecord struct {
	Name string

	// TODO add remaining functionality from spec 2.3 System Description Record after testnet v1
}

func NewSystemDescriptionRecord(name string) *SystemDescriptionRecord {
	return &SystemDescriptionRecord{Name: name}
}

func (s *SystemDescriptionRecord) addToHasher(hasher hash.Hash) {
	hasher.Write([]byte(s.Name))
}

func (s *SystemDescriptionRecord) hash(hasher hash.Hash) []byte {
	hasher.Reset()
	s.addToHasher(hasher)
	dhash := hasher.Sum(nil)
	hasher.Reset()
	return dhash
}
