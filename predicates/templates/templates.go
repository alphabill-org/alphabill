package templates

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/predicates"
	"github.com/alphabill-org/alphabill/types"
)

const (
	AlwaysFalseID byte = iota
	AlwaysTrueID
	P2pkh256ID
)

var (
	alwaysFalseBytes = []byte{0x83, 0x00, 0x41, 0x00, 0xf6}
	alwaysTrueBytes  = []byte{0x83, 0x00, 0x41, 0x01, 0xf6}

	cborNull = []byte{0xf6}
)

func alwaysTrue_Execute(params, args []byte) (bool, error) {
	// do not allow to piggyback any additional data on "always true" predicate
	if (len(params) == 0 || (bytes.Equal(params, cborNull))) && (len(args) == 0 || (bytes.Equal(args, cborNull))) {
		return true, nil
	}

	return false, fmt.Errorf(`"always true" predicate arguments must be empty`)
}

func alwaysFalse_Execute(params, args []byte) (bool, error) {
	// do not allow to piggyback any additional data on "always false" predicate
	if (len(params) == 0 || (bytes.Equal(params, cborNull))) && (len(args) == 0 || (bytes.Equal(args, cborNull))) {
		return false, nil
	}

	return false, fmt.Errorf(`"always false" predicate arguments must be empty`)
}

func p2pkh256_Execute(pubKeyHash, sig []byte, txo *types.TransactionOrder, env predicates.TxContext) (bool, error) {
	// when AB-1012 gets resolved should call txo.PayloadBytes() instead
	payloadBytes, err := env.PayloadBytes(txo)
	if err != nil {
		return false, fmt.Errorf("reading transaction payload bytes: %w", err)
	}

	p2pkh256Signature := predicates.P2pkh256Signature{}
	if err := types.Cbor.Unmarshal(sig, &p2pkh256Signature); err != nil {
		return false, fmt.Errorf("failed to decode P2PKH256 signature: %w", err)
	}
	if len(pubKeyHash) != 32 {
		return false, fmt.Errorf("invalid pubkey hash size: expected 32, got %d (%X)", len(pubKeyHash), pubKeyHash)
	}
	if len(p2pkh256Signature.Sig) != 65 {
		return false, fmt.Errorf("invalid signature size: expected 65, got %d (%X)", len(p2pkh256Signature.Sig), p2pkh256Signature.Sig)
	}
	if len(p2pkh256Signature.PubKey) != 33 {
		return false, fmt.Errorf("invalid pubkey size: expected 33, got %d (%X)", len(p2pkh256Signature.PubKey), p2pkh256Signature.PubKey)
	}
	if !bytes.Equal(pubKeyHash, hash.Sum256(p2pkh256Signature.PubKey)) {
		return false, errors.New("pubkey hash does not match")
	}

	verifier, err := crypto.NewVerifierSecp256k1(p2pkh256Signature.PubKey)
	if err != nil {
		return false, fmt.Errorf("failed to create verifier: %w", err)
	}
	if err = verifier.VerifyBytes(p2pkh256Signature.Sig, payloadBytes); err != nil {
		return false, fmt.Errorf("failed to verify signature: %w", err)
	}
	return true, nil
}

func AlwaysFalseBytes() types.PredicateBytes {
	return alwaysFalseBytes
}

func AlwaysTrueBytes() types.PredicateBytes {
	return alwaysTrueBytes
}

func EmptyArgument() []byte {
	return cborNull
}

func NewP2pkh256FromKey(pubKey []byte) predicates.Predicate {
	return NewP2pkh256FromKeyHash(hash.Sum256(pubKey))
}

func NewP2pkh256FromKeyHash(pubKeyHash []byte) predicates.Predicate {
	return predicates.Predicate{Tag: TemplateStartByte, Code: []byte{P2pkh256ID}, Params: pubKeyHash}
}

func NewP2pkh256BytesFromKey(pubKey []byte) types.PredicateBytes {
	pb, _ := types.Cbor.Marshal(NewP2pkh256FromKey(pubKey))
	return pb
}

func NewP2pkh256BytesFromKeyHash(pubKeyHash []byte) types.PredicateBytes {
	pb, _ := types.Cbor.Marshal(NewP2pkh256FromKeyHash(pubKeyHash))
	return pb
}

func NewP2pkh256SignatureBytes(sig, pubKey []byte) []byte {
	sb, _ := types.Cbor.Marshal(predicates.P2pkh256Signature{Sig: sig, PubKey: pubKey})
	return sb
}

func ExtractPubKeyHashFromP2pkhPredicate(pb []byte) ([]byte, error) {
	predicate := &predicates.Predicate{}
	if err := types.Cbor.Unmarshal(pb, predicate); err != nil {
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
