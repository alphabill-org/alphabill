package wallet

import (
	"bytes"
	"crypto"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
)

var (
	ErrTxProofNil     = errors.New("tx proof is nil")
	ErrInvalidValue   = errors.New("invalid value")
	ErrMissingDCNonce = errors.New("dcNonce is missing")
	ErrInvalidTxHash  = errors.New("bill txHash is not equal to actual transaction hash")
	ErrInvalidTxType  = errors.New("invalid tx type")
)

type (
	Bills struct {
		Bills []*Bill `json:"bills,omitempty"`
	}

	// TODO
	// used to be protobuf defined Bill struct used as import/export/download/upload unified schema across applications
	// possibly can be removed as import/export/download/upoad feature was dropped
	Bill struct {
		Id           []byte `json:"id,omitempty"`
		Value        uint64 `json:"value,omitempty,string"`
		TxHash       []byte `json:"tx_hash,omitempty"`
		TxRecordHash []byte `json:"tx_record_hash,omitempty"`
		DcNonce      []byte `json:"dc_nonce,omitempty"`

		// fcb specific fields
		// AddFCTxHash last add fee credit tx hash
		AddFCTxHash []byte `json:"add_fc_tx_hash,omitempty"`
	}

	BillProof struct {
		Bill    *Bill
		TxProof *Proof
	}
)

func (bp *BillProof) GetID() []byte {
	if bp != nil && bp.Bill != nil {
		return bp.Bill.Id
	}
	return nil
}

func NewTxProof(txIdx int, b *types.Block, hashAlgorithm crypto.Hash) (*Proof, error) {
	txProof, _, err := types.NewTxProof(b, txIdx, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return &Proof{
		TxRecord: b.Transactions[txIdx],
		TxProof:  txProof,
	}, nil
}

func (x *Bill) GetID() []byte {
	if x != nil {
		return x.Id
	}
	return nil
}

func (x *Bill) GetValue() uint64 {
	if x != nil {
		return x.Value
	}
	return 0
}

func (x *Bill) GetTxHash() []byte {
	if x != nil {
		return x.TxRecordHash
	}
	return nil
}

func (x *Bill) GetAddFCTxHash() []byte {
	if x != nil {
		return x.AddFCTxHash
	}
	return nil
}
func (x *Bill) verifyTx(txr *types.TransactionRecord) error {
	value, isDCTx, err := x.parseTx(txr)
	if err != nil {
		return err
	}
	if x.Value != value {
		return ErrInvalidValue
	}
	if isDCTx && x.DcNonce == nil {
		return ErrMissingDCNonce
	}
	if !bytes.Equal(x.TxHash, txr.Hash(crypto.SHA256)) {
		return ErrInvalidTxHash
	}
	return nil
}

func ReadBillsFile(path string) (*Bills, error) {
	b, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	res := &Bills{}
	err = json.Unmarshal(b, res)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func WriteBillsFile(path string, res []*BillProof) error {
	b, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0600) // -rw-------
}

func (x *Bill) parseTx(txr *types.TransactionRecord) (uint64, bool, error) {
	switch txr.TransactionOrder.PayloadType() {
	case money.PayloadTypeTransfer:
		attrs := &money.TransferAttributes{}
		if err := txr.TransactionOrder.UnmarshalAttributes(attrs); err != nil {
			return 0, false, err
		}
		return attrs.TargetValue, false, nil
	case money.PayloadTypeTransDC:
		attrs := &money.TransferDCAttributes{}
		if err := txr.TransactionOrder.UnmarshalAttributes(attrs); err != nil {
			return 0, false, err
		}
		return attrs.TargetValue, true, nil
	case money.PayloadTypeSplit:
		attrs := &money.SplitAttributes{}
		if err := txr.TransactionOrder.UnmarshalAttributes(attrs); err != nil {
			return 0, false, err
		}
		if bytes.Equal(x.Id, txr.TransactionOrder.UnitID()) { // proof is for the "old" bill
			return attrs.RemainingValue, false, nil
		}
		return attrs.Amount, false, nil // proof is for the "new" bill
	case money.PayloadTypeSwapDC:
		attrs := &money.SwapDCAttributes{}
		if err := txr.TransactionOrder.UnmarshalAttributes(attrs); err != nil {
			return 0, false, err
		}
		return attrs.TargetValue, false, nil
	default:
		return 0, false, ErrInvalidTxType
	}
}

func (p *Proof) ToGenericProof() *types.TxProof {
	txProof := p.TxProof
	if txProof == nil {
		return nil
	}
	return &types.TxProof{
		BlockHeaderHash:    txProof.BlockHeaderHash,
		Chain:              txProof.Chain,
		UnicityCertificate: txProof.UnicityCertificate,
	}
}
