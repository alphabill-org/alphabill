package tokens

import (
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"google.golang.org/protobuf/proto"
)

const (
	txTimeoutRoundCount        = 100
	AllAccounts         uint64 = 0
)

type (
	PredicateInput struct {
		// first priority
		Argument tokens.Predicate
		// if Argument empty, check AccountNumber
		AccountNumber uint64
	}

	MintAttr interface {
		proto.Message
		SetBearer([]byte)
		SetTokenCreationPredicateSignatures([][]byte)
	}

	AttrWithSubTypeCreationInputs interface {
		proto.Message
		SetSubTypeCreationPredicateSignatures([][]byte)
	}

	AttrWithInvariantPredicateInputs interface {
		proto.Message
		SetInvariantPredicateSignatures([][]byte)
	}
)
