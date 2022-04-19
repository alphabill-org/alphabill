package transaction

import (
	"crypto"

	"google.golang.org/protobuf/proto"
)

// Hash returns the hash value of the transaction.
func (x *Transaction) Hash(hashAlgorithm crypto.Hash) ([]byte, error) {
	hasher := hashAlgorithm.New()
	bytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(x)
	if err != nil {
		return nil, err
	}
	_, err = hasher.Write(bytes)
	if err != nil {
		return nil, err
	}
	return hasher.Sum(nil), nil
}
