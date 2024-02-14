package txsystem

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/types"
)

var (
	ErrTransactionExpired      = errors.New("transaction timeout must be greater than current block number")
	ErrInvalidSystemIdentifier = errors.New("error invalid system identifier")
)

func VerifyUnitOwnerProof(tx *types.TransactionOrder, bearer predicates.PredicateBytes) error {
	payloadBytes, err := tx.PayloadBytes()
	if err != nil {
		return fmt.Errorf("failed to marshal payload bytes: %w", err)
	}

	if err = predicates.RunPredicate(bearer, tx.OwnerProof, payloadBytes); err != nil {
		return fmt.Errorf("invalid owner proof: %w [txOwnerProof=0x%x unitOwnerCondition=0x%x sigData=0x%x]",
			err, tx.OwnerProof, bearer, payloadBytes)
	}

	return nil
}
