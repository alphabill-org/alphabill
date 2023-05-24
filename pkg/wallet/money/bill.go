package money

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	moneytx "github.com/alphabill-org/alphabill/pkg/wallet/backend/bp"
	"github.com/holiman/uint256"
)

type (
	Bill struct {
		Id      *uint256.Int   `json:"id"`
		Value   uint64         `json:"value,string"`
		TxHash  []byte         `json:"txHash"`
		TxProof *types.TxProof `json:"txProof"`

		// dc bill specific fields
		IsDcBill  bool   `json:"dcBill"`
		DcTimeout uint64 `json:"dcTimeout"`
		DcNonce   []byte `json:"dcNonce"`
		// DcExpirationTimeout blockHeight when dc bill gets removed from state tree
		DcExpirationTimeout uint64 `json:"dcExpirationTimeout"`

		// fcb specific fields
		// FCBlockNumber block number when fee credit bill balance was last updated
		FCBlockNumber uint64 `json:"fcBlockNumber,string"`
	}

	// deprecated: can be replaced with types.TxProof
	//BlockProof struct {
	//	Tx          *types.TransactionRecord `json:"tx"`
	//	Proof       *types.TxProof           `json:"proof"`
	//	BlockNumber uint64                   `json:"blockNumber"`
	//}
)

//func NewBlockProof(tx *types.TransactionRecord, proof *types.TxProof, blockNumber uint64) (*types.TxProof, error) {
//	if tx == nil {
//		return nil, errors.New("tx is nil")
//	}
//	if proof == nil {
//		return nil, errors.New("proof is nil")
//	}
//	return proof, nil
//}
//
//func (b *BlockProof) Verify(verifiers map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
//	return types.VerifyTxProof(b.Proof, b.Tx, verifiers, hashAlgorithm)
//}

// GetID returns bill id in 32-byte big endian array
func (b *Bill) GetID() []byte {
	if b != nil {
		return util.Uint256ToBytes(b.Id)
	}
	return nil
}

func (b *Bill) ToProto() *moneytx.Bill {
	return &moneytx.Bill{
		Id:      b.GetID(),
		Value:   b.Value,
		TxHash:  b.TxHash,
		TxProof: b.TxProof,
	}
}

// isExpired returns true if dcBill, that was left unswapped, should be deleted
func (b *Bill) isExpired(blockHeight uint64) bool {
	return b.IsDcBill && blockHeight >= b.DcExpirationTimeout
}

func (b *Bill) addProof(txIdx int, bl *types.Block) error {
	proof, err := createProof(txIdx, bl, crypto.SHA256)
	if err != nil {
		return err
	}
	b.TxProof = proof
	return nil
}

func createProof(txIdx int, b *types.Block, hashAlgorithm crypto.Hash) (*types.TxProof, error) {
	txProof, err := types.NewTxProof(b, txIdx, hashAlgorithm)
	if err != nil {
		return nil, err
	}
	return txProof, nil
}

func (b *Bill) GetTxHash() []byte {
	if b != nil {
		return b.TxHash
	}
	return nil
}

func (b *Bill) GetValue() uint64 {
	if b != nil {
		return b.Value
	}
	return 0
}
