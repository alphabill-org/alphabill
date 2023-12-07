package templates

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
)

const (
	AlwaysFalseID uint64 = iota
	AlwaysTrueID
	P2pkh256ID
)

type (
	PredicateTemplate interface {
		ID() uint64
		Execute(bytes, sig, sigData []byte) error
	}

	AlwaysTrue struct{}

	AlwaysFalse struct{}

	P2pkh256 struct{}

	P2pkh256Payload struct {
		_          struct{} `cbor:",toarray"`
		PubKeyHash []byte
	}

	P2pkh256Signature struct {
		_      struct{} `cbor:",toarray"`
		Sig    []byte
		PubKey []byte
	}
)

var (
	alwaysTrueBytes  []byte
	alwaysFalseBytes []byte

	cborNull = []byte{0xf6}
)

func init() {
	alwaysTrueBytes, _ = cbor.Marshal(predicates.Predicate{Tag: TemplateStartByte, ID: AlwaysTrueID})
	alwaysFalseBytes, _ = cbor.Marshal(predicates.Predicate{Tag: TemplateStartByte, ID: AlwaysFalseID})
}

func (t *AlwaysTrue) ID() uint64 {
	return AlwaysTrueID
}

func (t *AlwaysTrue) Execute(_, sig, _ []byte) error {
	if len(sig) == 0 || (bytes.Equal(sig, cborNull)) {
		return nil
	}

	return fmt.Errorf("always true predicate requires signature to be empty, got: '%X'", sig)
}

func (t *AlwaysFalse) ID() uint64 {
	return AlwaysFalseID
}

func (t *AlwaysFalse) Execute(_, _, _ []byte) error {
	// we can ignore the signature here because the tx will never be accepted into the block
	return errors.New("always false")
}

func (t *P2pkh256) ID() uint64 {
	return P2pkh256ID
}

func (t *P2pkh256) Execute(predicate, sig, sigData []byte) error {
	p2pkh256Payload := &P2pkh256Payload{}
	err := cbor.Unmarshal(predicate, p2pkh256Payload)
	if err != nil {
		return fmt.Errorf("failed to decode P2PKH256 predicate template: %w", err)
	}
	p2pkh256Signature := &P2pkh256Signature{}
	err = cbor.Unmarshal(sig, p2pkh256Signature)
	if err != nil {
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
	if !bytes.Equal(p2pkh256Payload.PubKeyHash, util.Sum256(p2pkh256Signature.PubKey)) {
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
	return alwaysFalseBytes // 0x830000F6
}

func AlwaysTrueArgBytes() []byte {
	return cborNull
}

func AlwaysTrueBytes() predicates.PredicateBytes {
	return alwaysTrueBytes // 0x830001F6
}

func NewP2pkh256FromKey(pubKey []byte) predicates.Predicate {
	return NewP2pkh256FromKeyHash(util.Sum256(pubKey))
}

func NewP2pkh256FromKeyHash(pubKeyHash []byte) predicates.Predicate {
	body, _ := cbor.Marshal(P2pkh256Payload{PubKeyHash: pubKeyHash})
	return predicates.Predicate{Tag: TemplateStartByte, ID: P2pkh256ID, Body: body}
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
	sb, _ := cbor.Marshal(P2pkh256Signature{Sig: sig, PubKey: pubKey})
	return sb
}

func ExtractPubKey(p2pkhSig []byte) ([]byte, error) {
	if len(p2pkhSig) == 0 {
		return nil, errors.New("empty predicate signature")
	}
	sig := &P2pkh256Signature{}
	err := cbor.Unmarshal(p2pkhSig, sig)
	if err != nil {
		return nil, err
	}
	return sig.PubKey, nil
}

func ExtractPubKeyHash(pb []byte) ([]byte, error) {
	predicate := &predicates.Predicate{}
	if err := cbor.Unmarshal(pb, predicate); err != nil {
		return nil, err
	}
	p2pkh256Payload := &P2pkh256Payload{}
	if err := cbor.Unmarshal(predicate.Body, p2pkh256Payload); err != nil {
		return nil, err
	}
	return p2pkh256Payload.PubKeyHash, nil
}
