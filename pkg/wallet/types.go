package wallet

import (
	"errors"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/crypto"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/network/protocol/genesis"
	"github.com/alphabill-org/alphabill/internal/util"
)

type (
	BlockProcessor interface {

		// ProcessBlock signals given block to be processesed
		// any error returned here signals block processor to terminate,
		ProcessBlock(b *block.Block) error
	}
	// TrustBase json schema for trust base file.
	TrustBase struct {
		RootValidators []*genesis.PublicKeyInfo `json:"root_validators"`
	}
)

func ReadTrustBaseFile(file string) (map[string]crypto.Verifier, error) {
	trustBase, err := util.ReadJsonFile(file, &TrustBase{})
	if err != nil {
		return nil, err
	}
	err = trustBase.Verify()
	if err != nil {
		return nil, err
	}
	verifiers, err := trustBase.ToVerifiers()
	if err != nil {
		return nil, err
	}
	return verifiers, nil
}

func (t *TrustBase) Verify() error {
	if len(t.RootValidators) == 0 {
		return errors.New("validators list is empty")
	}
	for _, rv := range t.RootValidators {
		if len(rv.SigningPublicKey) == 0 {
			return errors.New("missing trust base signing public key")
		}
		if len(rv.NodeIdentifier) == 0 {
			return errors.New("missing trust base node identifier")
		}
	}
	return nil
}

func (t *TrustBase) ToVerifiers() (map[string]abcrypto.Verifier, error) {
	return genesis.NewValidatorTrustBase(t.RootValidators)
}
