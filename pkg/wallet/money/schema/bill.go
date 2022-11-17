package schema

import (
	"bytes"
	"crypto"

	"github.com/alphabill-org/alphabill/internal/block"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/go-playground/validator/v10"
)

// use a single instance of Validate, it caches struct info
var validate = validator.New()

type (
	// Bill import/export schema.
	Bill struct {
		Id         []byte      `json:"id" validate:"required,len=32"`
		Value      uint64      `json:"value" validate:"required"`
		TxHash     []byte      `json:"txHash" validate:"required"`
		BlockProof *BlockProof `json:"blockProof" validate:"required"`
	}
	// BlockProof import/export schema.
	BlockProof struct {
		BlockNumber uint64                `json:"blockNumber" validate:"required,gt=0"`
		Tx          *txsystem.Transaction `json:"tx" validate:"required"`
		Proof       *block.BlockProof     `json:"proof" validate:"required"`
	}
)

func (b *Bill) Verify(verifiers map[string]abcrypto.Verifier) error {
	err := b.validate()
	if err != nil {
		return err
	}
	tx, err := money.NewMoneyTx([]byte{0, 0, 0, 0}, b.BlockProof.Tx)
	if err != nil {
		return err
	}
	if !bytes.Equal(b.TxHash, tx.Hash(crypto.SHA256)) {
		return errors.New("bill txHash is not equal to actual transaction hash")
	}
	return b.BlockProof.Proof.Verify(tx, verifiers, crypto.SHA256)
}

func (b *Bill) validate() error {
	return validate.Struct(b)
}
