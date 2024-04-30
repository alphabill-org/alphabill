package predicates

import (
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/predicates/templates"
	"github.com/alphabill-org/alphabill-go-sdk/crypto"
)

type Signer interface {
	SignBytes([]byte) ([]byte, error)
	Verifier() (crypto.Verifier, error)
}

func EncodeSignature(sig, pubKey []byte) ([]byte, error) {
	return types.Cbor.Marshal(templates.P2pkh256Signature{Sig: sig, PubKey: pubKey})
}

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

/*
OwnerProofer returns function which can be used as OwnerProof generator.
"pubKey" must be the public key of the "signer".
The generator function takes "bytes to sign" as a parameter and returns serialized
owner proof (CBOR encoded Signature struct).
*/
func OwnerProofer(signer Signer, pubKey []byte) func([]byte) ([]byte, error) {
	return func(data []byte) ([]byte, error) {
		sig, err := signer.SignBytes(data)
		if err != nil {
			return nil, fmt.Errorf("signing payload: %w", err)
		}
		return EncodeSignature(sig, pubKey)
	}
}

/*
OwnerProoferForSigner returns OwnerProof generator for the signer.
Prefer OwnerProofer(signer, pubKey) variation when pubKey of the signer
is also already available.
*/
func OwnerProoferForSigner(signer Signer) func([]byte) ([]byte, error) {
	verifier, err := signer.Verifier()
	if err != nil {
		return func([]byte) ([]byte, error) { return nil, fmt.Errorf("requesting verifier of the signer: %w", err) }
	}
	pubKey, err := verifier.MarshalPublicKey()
	if err != nil {
		return func([]byte) ([]byte, error) { return nil, fmt.Errorf("serializing public key of the signer: %w", err) }
	}
	return OwnerProofer(signer, pubKey)
}

/*
OwnerProoferSecp256K1 is like OwnerProofer but takes private / public key
pair as a parameter.
Keys are assumed to be ECDSA keys for the secp256k1 curve.
*/
func OwnerProoferSecp256K1(privKey, pubKey []byte) func([]byte) ([]byte, error) {
	signer, err := crypto.NewInMemorySecp256K1SignerFromKey(privKey)
	if err != nil {
		return func([]byte) ([]byte, error) {
			return nil, fmt.Errorf("creating signer with given private key: %w", err)
		}
	}
	return OwnerProofer(signer, pubKey)
}
