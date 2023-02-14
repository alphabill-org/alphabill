package tokens

import "github.com/alphabill-org/alphabill/internal/txsystem/tokens"

const (
	AllAccounts uint64 = 0
)

type (
	PredicateInput struct {
		// first priority
		Argument tokens.Predicate
		// if Argument empty, check AccountNumber
		AccountNumber uint64
	}
)
