package money

import (
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/pkg/wallet"
)

// verifyTxP2PKHOwner checks if given tx predicate is P2PKH predicate for given key.
func verifyTxP2PKHOwner(stx *txsystem.Transaction, key *wallet.KeyHashes) error {
	gtx, err := txConverter.ConvertTx(stx)
	if err != nil {
		return err
	}
	switch tx := gtx.(type) {
	case money.Transfer:
		if wallet.VerifyP2PKHOwner(key, tx.NewBearer()) {
			return nil
		}
	case money.TransferDC:
		if wallet.VerifyP2PKHOwner(key, tx.TargetBearer()) {
			return nil
		}
	case money.Split:
		if wallet.VerifyP2PKHOwner(key, tx.TargetBearer()) {
			return nil
		}
	case money.Swap:
		if wallet.VerifyP2PKHOwner(key, tx.OwnerCondition()) {
			return nil
		}
	default:
		return errors.New("unknown transaction type")
	}
	return errors.New("invalid bearer predicate")
}
