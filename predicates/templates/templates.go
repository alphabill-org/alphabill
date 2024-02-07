package templates

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

const (
	AlwaysFalseID byte = iota
	AlwaysTrueID
	P2pkh256ID
)

type (
	PredicateTemplate interface {
		ID() byte
		Execute(bytes, sig, sigData []byte) error
	}

	AlwaysTrue struct{}

	AlwaysFalse struct{}

	P2pkh256 struct{}
)

var (
	alwaysFalseBytes = []byte{0x83, 0x00, 0x41, 0x00, 0xf6}
	alwaysTrueBytes  = []byte{0x83, 0x00, 0x41, 0x01, 0xf6}

	cborNull = []byte{0xf6}
)

func (t *AlwaysTrue) ID() byte {
	return AlwaysTrueID
}

func (t *AlwaysTrue) Execute(_, sig, _ []byte) error {
	if len(sig) == 0 || (bytes.Equal(sig, cborNull)) {
		return nil
	}

	return fmt.Errorf("always true predicate requires signature to be empty, got: '%X'", sig)
}

func (t *AlwaysFalse) ID() byte {
	return AlwaysFalseID
}

func (t *AlwaysFalse) Execute(_, _, _ []byte) error {
	// we can ignore the signature here because the tx will never be accepted into the block
	return errors.New("always false")
}

func (t *P2pkh256) ID() byte {
	return P2pkh256ID
}

func (t *P2pkh256) Execute(pubKeyHash, sig, sigData []byte) error {
	p2pkh256Signature := predicates.P2pkh256Signature{}
	if err := cbor.Unmarshal(sig, &p2pkh256Signature); err != nil {
		return fmt.Errorf("failed to decode P2PKH256 signature: %w", err)
	}
	if len(pubKeyHash) != 32 {
		return fmt.Errorf("invalid pubkey hash size: expected 32, got %d (%X)", len(pubKeyHash), pubKeyHash)
	}
	if len(p2pkh256Signature.Sig) != 65 {
		return fmt.Errorf("invalid signature size:expected 65, got %d (%X)", len(p2pkh256Signature.Sig), p2pkh256Signature.Sig)
	}
	if len(p2pkh256Signature.PubKey) != 33 {
		return fmt.Errorf("invalid pubkey size: expected 33, got %d (%X)", len(p2pkh256Signature.PubKey), p2pkh256Signature.PubKey)
	}
	if !bytes.Equal(pubKeyHash, hash.Sum256(p2pkh256Signature.PubKey)) {
		return errors.New("pubkey hash does not match")
	}

	verifier, err := crypto.NewVerifierSecp256k1(p2pkh256Signature.PubKey)
	if err != nil {
		return fmt.Errorf("failed to create verifier: %w", err)
	}
	if err = verifier.VerifyBytes(p2pkh256Signature.Sig, sigData); err != nil {
		return fmt.Errorf("failed to verify signature: %w", err)
	}
	return nil
}

func AlwaysFalseBytes() types.PredicateBytes {
	return alwaysFalseBytes
}

func AlwaysTrueArgBytes() []byte {
	return cborNull
}

func AlwaysTrueBytes() types.PredicateBytes {
	return alwaysTrueBytes
}

func NewP2pkh256FromKey(pubKey []byte) predicates.Predicate {
	return NewP2pkh256FromKeyHash(hash.Sum256(pubKey))
}

func NewP2pkh256FromKeyHash(pubKeyHash []byte) predicates.Predicate {
	return predicates.Predicate{Tag: TemplateStartByte, Code: []byte{P2pkh256ID}, Params: pubKeyHash}
}

func NewP2pkh256BytesFromKey(pubKey []byte) types.PredicateBytes {
	pb, _ := cbor.Marshal(NewP2pkh256FromKey(pubKey))
	return pb
}

func NewP2pkh256BytesFromKeyHash(pubKeyHash []byte) types.PredicateBytes {
	pb, _ := cbor.Marshal(NewP2pkh256FromKeyHash(pubKeyHash))
	return pb
}

func NewP2pkh256SignatureBytes(sig, pubKey []byte) []byte {
	sb, _ := cbor.Marshal(predicates.P2pkh256Signature{Sig: sig, PubKey: pubKey})
	return sb
}

func ExtractPubKeyHashFromP2pkhPredicate(pb []byte) ([]byte, error) {
	predicate := &predicates.Predicate{}
	if err := cbor.Unmarshal(pb, predicate); err != nil {
		return nil, fmt.Errorf("extracting predicate: %w", err)
	}
	if predicate.Tag != TemplateStartByte {
		return nil, fmt.Errorf("not a predicate template (tag %d)", predicate.Tag)
	}
	if len(predicate.Code) != 1 && predicate.Code[0] != P2pkh256ID {
		return nil, fmt.Errorf("not a p2pkh predicate (id %X)", predicate.Code)
	}
	return predicate.Params, nil
}

func IsP2pkhTemplate(predicate *predicates.Predicate) bool {
	return predicate != nil && predicate.Tag == TemplateStartByte && len(predicate.Code) == 1 && predicate.Code[0] == P2pkh256ID
}
