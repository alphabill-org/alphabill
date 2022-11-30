package money

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
)

type (
	Bill struct {
		Id         *uint256.Int `json:"id"`
		Value      uint64       `json:"value"`
		TxHash     []byte       `json:"txHash"`
		BlockProof *BlockProof  `json:"blockProof"`

		// dc bill specific fields
		IsDcBill  bool   `json:"dcBill"`
		DcTimeout uint64 `json:"dcTimeout"`
		DcNonce   []byte `json:"dcNonce"`
		// DcExpirationTimeout blockHeight when dc bill gets removed from state tree
		DcExpirationTimeout uint64 `json:"dcExpirationTimeout"`
	}

	BlockProof struct {
		Tx          *txsystem.Transaction `json:"tx"`
		Proof       *block.BlockProof     `json:"proof"`
		BlockNumber uint64                `json:"blockNumber"`
	}
)

func NewBlockProof(tx *txsystem.Transaction, proof *block.BlockProof, blockNumber uint64) (*BlockProof, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if proof == nil {
		return nil, errors.New("proof is nil")
	}
	return &BlockProof{
		Tx:          tx,
		Proof:       proof,
		BlockNumber: blockNumber,
	}, nil
}

func (b *BlockProof) Verify(unitID []byte, verifiers map[string]abcrypto.Verifier, hashAlgorithm crypto.Hash) error {
	gtx, err := txConverter.ConvertTx(b.Tx)
	if err != nil {
		return err
	}
	return b.Proof.Verify(unitID, gtx, verifiers, hashAlgorithm)
}

// GetID returns bill id in 32-byte big endian array
func (b *Bill) GetID() []byte {
	return util.Uint256ToBytes(b.Id)
}

func (b *Bill) ToProto() *block.Bill {
	return &block.Bill{
		Id:      b.GetID(),
		Value:   b.Value,
		TxHash:  b.TxHash,
		TxProof: b.BlockProof.ToSchema(),
	}
}

func (b *BlockProof) ToSchema() *block.TxProof {
	return &block.TxProof{
		Tx:          b.Tx,
		Proof:       b.Proof,
		BlockNumber: b.BlockNumber,
	}
}

// isExpired returns true if dcBill, that was left unswapped, should be deleted
func (b *Bill) isExpired(blockHeight uint64) bool {
	return b.IsDcBill && blockHeight >= b.DcExpirationTimeout
}

func (b *Bill) addProof(bl *block.Block, txPb *txsystem.Transaction) error {
	genericBlock, err := bl.ToGenericBlock(txConverter)
	if err != nil {
		return err
	}
	proof, err := block.NewPrimaryProof(genericBlock, b.GetID(), crypto.SHA256)
	if err != nil {
		return err
	}
	blockProof, err := NewBlockProof(txPb, proof, bl.BlockNumber)
	if err != nil {
		return err
	}
	b.BlockProof = blockProof
	return nil
}
