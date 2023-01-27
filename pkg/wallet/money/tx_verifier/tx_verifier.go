package txverifier

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/wallet/account"
)

var ErrVerificationFailed = errors.New("p2pkh predicate verification failed")

// VerifyTxP2PKHOwner checks if given tx predicate is P2PKH predicate for given key.
func VerifyTxP2PKHOwner(gtx txsystem.GenericTransaction, key *account.KeyHashes) error {
	if gtx == nil {
		return fmt.Errorf("%w: %s", ErrVerificationFailed, "tx is nil")
	}
	if key == nil {
		return fmt.Errorf("%w: %s", ErrVerificationFailed, "key is nil")
	}
	switch tx := gtx.(type) {
	case money.Transfer:
		if account.VerifyP2PKHOwner(key, tx.NewBearer()) {
			return nil
		}
	case money.TransferDC:
		if account.VerifyP2PKHOwner(key, tx.TargetBearer()) {
			return nil
		}
	case money.Split:
		if account.VerifyP2PKHOwner(key, tx.TargetBearer()) {
			return nil
		}
	case money.Swap:
		if account.VerifyP2PKHOwner(key, tx.OwnerCondition()) {
			return nil
		}
	default:
		return fmt.Errorf("%w: %s", ErrVerificationFailed, "unknown transaction type")
	}
	return fmt.Errorf("%w: %s", ErrVerificationFailed, "invalid bearer predicate")
}
