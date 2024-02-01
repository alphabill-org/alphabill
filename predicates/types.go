package predicates

import "github.com/fxamacker/cbor/v2"

const MaxBearerBytes = 65536

type (
	PredicateBytes []byte

	Predicate struct {
		_      struct{} `cbor:",toarray"`
		Tag    uint64
		Code   cbor.RawMessage
		Params cbor.RawMessage
	}

	PredicateRunner interface {
		Execute(p *Predicate, sig []byte, sigData []byte) error
	}
)

func ExtractPredicate(predicateBytes []byte) (*Predicate, error) {
	predicate := &Predicate{}
	if err := cbor.Unmarshal(predicateBytes, predicate); err != nil {
		return nil, err
	}
	return predicate, nil
}
