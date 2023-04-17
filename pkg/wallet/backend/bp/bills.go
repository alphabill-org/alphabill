package bp

import (
	"bytes"
	"crypto"
	"errors"
	"os"
	"path/filepath"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/util"
	"google.golang.org/protobuf/encoding/protojson"
)

var (
	ErrTxProofNil        = errors.New("tx proof is nil")
	ErrInvalidValue      = errors.New("invalid value")
	ErrInvalidDCBillFlag = errors.New("invalid isDcBill flag")
	ErrInvalidTxHash     = errors.New("bill txHash is not equal to actual transaction hash")
	ErrInvalidTxType     = errors.New("invalid tx type")
)

type TxConverter interface {
	ConvertTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error)
}

// Verify validates struct and verifies proofs.
func (x *Bills) Verify(txConverter TxConverter, verifiers map[string]abcrypto.Verifier) error {
	for _, bill := range x.Bills {
		err := bill.Verify(txConverter, verifiers)
		if err != nil {
			return err
		}
	}
	return nil
}

// Verify validates struct and verifies proof.
func (x *Bill) Verify(txConverter TxConverter, verifiers map[string]abcrypto.Verifier) error {
	if x.TxProof == nil {
		return ErrTxProofNil
	}
	gtx, err := txConverter.ConvertTx(x.TxProof.Tx)
	if err != nil {
		return err
	}
	err = x.verifyTx(gtx)
	if err != nil {
		return err
	}
	return x.TxProof.Verify(x.Id, gtx, verifiers, crypto.SHA256)
}

func (x *Bill) verifyTx(gtx txsystem.GenericTransaction) error {
	value, isDCTx, err := x.parseTx(gtx)
	if err != nil {
		return err
	}
	if x.Value != value {
		return ErrInvalidValue
	}
	if x.IsDcBill != isDCTx {
		return ErrInvalidDCBillFlag
	}
	if !bytes.Equal(x.TxHash, gtx.Hash(crypto.SHA256)) {
		return ErrInvalidTxHash
	}
	return nil
}

func (x *Bill) parseTx(gtx txsystem.GenericTransaction) (uint64, bool, error) {
	switch tx := gtx.(type) {
	case money.Transfer:
		return tx.TargetValue(), false, nil
	case money.TransferDC:
		return tx.TargetValue(), true, nil
	case money.Split:
		if bytes.Equal(x.Id, util.Uint256ToBytes(gtx.UnitID())) { // proof is for the "old" bill
			return tx.RemainingValue(), false, nil
		}
		return tx.Amount(), false, nil // proof is for the "new" bill
	case money.SwapDC:
		return tx.TargetValue(), false, nil
	default:
		return 0, false, ErrInvalidTxType
	}
}

func ReadBillsFile(path string) (*Bills, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	res := &Bills{}
	err = protojson.Unmarshal(b, res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func WriteBillsFile(path string, res *Bills) error {
	b, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(res)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600) // -rw-------
}
