package predicates

import (
	"github.com/fxamacker/cbor/v2"
)

const MaxBearerBytes = 65536

type (
	Predicate struct {
		_    struct{} `cbor:",toarray"`
		Tag  byte
		ID   uint64
		Body cbor.RawMessage
	}

	PredicateRunner interface {
		Execute(p *Predicate, sig []byte, sigData []byte) error
	}
)
