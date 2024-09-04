package predicates

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/predicates/templates"
	"github.com/alphabill-org/alphabill-go-base/types"
)

func ExtractPubKey(ownerProof []byte) ([]byte, error) {
	if len(ownerProof) == 0 {
		return nil, errors.New("empty owner proof as input")
	}
	sig := templates.P2pkh256Signature{}
	if err := types.Cbor.Unmarshal(ownerProof, &sig); err != nil {
		return nil, fmt.Errorf("decoding owner proof as Signature: %w", err)
	}
	return sig.PubKey, nil
}
