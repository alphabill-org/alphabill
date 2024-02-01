package templates

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/predicates"
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

	P2pkh256Payload struct {
		_          struct{} `cbor:",toarray"`
		PubKeyHash []byte
	}
)

var (
	alwaysFalseBytes = []byte{0x83, 0x0, 0x0, 0xf6}
	alwaysTrueBytes  = []byte{0x83, 0x0, 0x1, 0xf6}

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

func (t *P2pkh256) Execute(predicate, sig, sigData []byte) error {
	p2pkh256Payload := &P2pkh256Payload{}
	err := cbor.Unmarshal(predicate, p2pkh256Payload)
	if err != nil {
		return fmt.Errorf("failed to decode P2PKH256 predicate template: %w", err)
	}
	p2pkh256Signature := predicates.P2pkh256Signature{}
	if err = cbor.Unmarshal(sig, &p2pkh256Signature); err != nil {
		return fmt.Errorf("failed to decode P2PKH256 signature: %w", err)
	}
	if len(p2pkh256Payload.PubKeyHash) != 32 {
		return fmt.Errorf("invalid pubkey hash size: %X, expected 32", p2pkh256Payload.PubKeyHash)
	}
	if len(p2pkh256Signature.Sig) != 65 {
		return fmt.Errorf("invalid signature size: %X, expected 65", p2pkh256Signature.Sig)
	}
	if len(p2pkh256Signature.PubKey) != 33 {
		return fmt.Errorf("invalid pubkey size: %X, expected 33", p2pkh256Signature.PubKey)
	}
	if !bytes.Equal(p2pkh256Payload.PubKeyHash, hash.Sum256(p2pkh256Signature.PubKey)) {
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

func AlwaysFalseBytes() predicates.PredicateBytes {
	return alwaysFalseBytes
}

func AlwaysTrueArgBytes() []byte {
	return cborNull
}

func AlwaysTrueBytes() predicates.PredicateBytes {
	return alwaysTrueBytes
}

func NewP2pkh256FromKey(pubKey []byte) predicates.Predicate {
	return NewP2pkh256FromKeyHash(hash.Sum256(pubKey))
}

func NewP2pkh256FromKeyHash(pubKeyHash []byte) predicates.Predicate {
	body, _ := cbor.Marshal(P2pkh256Payload{PubKeyHash: pubKeyHash})
	return predicates.Predicate{Tag: TemplateStartByte, Code: []byte{P2pkh256ID}, Params: body}
}

func NewP2pkh256BytesFromKey(pubKey []byte) predicates.PredicateBytes {
	pb, _ := cbor.Marshal(NewP2pkh256FromKey(pubKey))
	return pb
}

func NewP2pkh256BytesFromKeyHash(pubKeyHash []byte) predicates.PredicateBytes {
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
	if predicate.Code[0] != P2pkh256ID {
		return nil, fmt.Errorf("not a p2pkh predicate (id %X)", predicate.Code)
	}
	p2pkh256Payload := &P2pkh256Payload{}
	if err := cbor.Unmarshal(predicate.Params, p2pkh256Payload); err != nil {
		return nil, fmt.Errorf("extracting payload: %w", err)
	}
	return p2pkh256Payload.PubKeyHash, nil
}

func ExtractP2pkhPayload(predicate *predicates.Predicate) (*P2pkh256Payload, error) {
	p2pkh256Payload := &P2pkh256Payload{}
	if err := cbor.Unmarshal(predicate.Params, p2pkh256Payload); err != nil {
		return nil, err
	}
	return p2pkh256Payload, nil
}

func IsP2pkhTemplate(predicate *predicates.Predicate) bool {
	return predicate != nil && predicate.Tag == TemplateStartByte && len(predicate.Code) == 1 && predicate.Code[0] == P2pkh256ID
}
