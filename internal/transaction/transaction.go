package transaction

import (
	"crypto"

	"google.golang.org/protobuf/proto"
)

// Hash returns the hash value of the transaction.
func (x *Transaction) Hash(hashAlgorithm crypto.Hash) ([]byte, error) {
	hasher := hashAlgorithm.New()
	bytes, err := x.Bytes()
	if err != nil {
		return nil, err
	}
	_, err = hasher.Write(bytes)
	if err != nil {
		return nil, err
	}
	return hasher.Sum(nil), nil
}

func (x *Transaction) Bytes() ([]byte, error) {
	x.unknownFields = []byte{}
	// Setting "Deterministic" option guarantees that repeated serialization of
	// the same message will return the same bytes, and that different
	// processes of the same binary (which may be executing on different
	// machines) will serialize equal messages to the same bytes.
	//
	// NB! Note that the deterministic serialization is NOT canonical across
	// languages!
	// TODO implement a better transaction serialization.
	return proto.MarshalOptions{Deterministic: true}.Marshal(x)
}
