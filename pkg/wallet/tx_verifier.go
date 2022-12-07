package wallet

import (
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
)

type DefaultTxVerifier struct {
	trustBase map[string]crypto.Verifier
}

type AlwaysValidTxVerifier struct {
}

func (t *DefaultTxVerifier) Verify(tx txsystem.GenericTransaction, genericBlock *block.GenericBlock) error {
	proof, err := block.NewPrimaryProof(genericBlock, util.Uint256ToBytes(tx.UnitID()), gocrypto.SHA256)
	if err != nil {
		return fmt.Errorf("failed to extract proof: %v", err)
	}
	if proof.ProofType == block.ProofType_NOTRANS || proof.ProofType == block.ProofType_EMPTYBLOCK {
		return fmt.Errorf("tx with unit ID %X is not part of a block with number %v", util.Uint256ToBytes(tx.UnitID()), genericBlock.BlockNumber)
	}
	return proof.Verify(util.Uint256ToBytes(tx.UnitID()), tx, t.trustBase, gocrypto.SHA256)
}

func (t *AlwaysValidTxVerifier) Verify(txsystem.GenericTransaction, *block.GenericBlock) error {
	return nil
}
