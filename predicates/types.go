package predicates

//
//import (
//	"github.com/alphabill-org/alphabill/types"
//	"github.com/fxamacker/cbor/v2"
//)
//
//const MaxBearerBytes = 65536
//
//type (
//	Predicate struct {
//		_      struct{} `cbor:",toarray"`
//		Tag    uint64
//		Code   []byte
//		Params []byte
//	}
//
//	PredicateContext struct {
//		Input        []byte
//		PayloadBytes []byte
//		Tx           *types.TransactionOrder
//	}
//
//	PredicateRunner interface {
//		Execute(p *Predicate, ctx *PredicateContext) error
//	}
//)
//
//func ExtractPredicate(predicateBytes []byte) (*Predicate, error) {
//	predicate := &Predicate{}
//	if err := cbor.Unmarshal(predicateBytes, predicate); err != nil {
//		return nil, err
//	}
//	return predicate, nil
//}
