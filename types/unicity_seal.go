package types

import (
	"bytes"
	"errors"
	"fmt"
	"hash"
	"sort"
	"time"

	"github.com/alphabill-org/alphabill/crypto"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
)

// GenesisTime min timestamp Thursday, April 20, 2023 6:11:24 AM GMT+00:00
// Epoch (or Unix time or POSIX time or Unix timestamp) is the number of seconds that have elapsed since January 1, 1970 (midnight UTC/GMT)
const GenesisTime uint64 = 1681971084

var (
	ErrUnicitySealIsNil          = errors.New("unicity seal is nil")
	ErrSignerIsNil               = errors.New("signer is nil")
	ErrUnicitySealHashIsNil      = errors.New("hash is nil")
	ErrInvalidBlockNumber        = errors.New("invalid block number")
	ErrUnicitySealSignatureIsNil = errors.New("no signatures")
	ErrRootValidatorInfoMissing  = errors.New("root node info is missing")
	ErrUnknownSigner             = errors.New("unknown signer")
	errInvalidTimestamp          = errors.New("invalid timestamp")
	errUnicitySealNoSignature    = errors.New("unicity seal is missing signature")
)

type SignatureMap map[string][]byte
type UnicitySeal struct {
	_                    struct{}     `cbor:",toarray"`
	RootChainRoundNumber uint64       `json:"root_chain_round_number,omitempty"`
	Timestamp            uint64       `json:"timestamp,omitempty"`
	PreviousHash         []byte       `json:"previous_hash,omitempty"`
	Hash                 []byte       `json:"hash,omitempty"`
	Signatures           SignatureMap `json:"signatures,omitempty"`
}

// Signatures are serialized as alphabetically sorted CBOR array
type signaturesCBOR []*signature
type signature struct {
	_         struct{} `cbor:",toarray"`
	NodeID    string   `json:"node_id,omitempty"`
	Signature []byte   `json:"signature,omitempty"`
}

func (s *SignatureMap) MarshalCBOR() ([]byte, error) {
	// shallow copy
	signatures := *s
	authors := make([]string, 0, len(signatures))
	for k := range *s {
		authors = append(authors, k)
	}
	sort.Strings(authors)
	sCBOR := make(signaturesCBOR, len(signatures))
	for i, author := range authors {
		sCBOR[i] = &signature{NodeID: author, Signature: signatures[author]}
	}
	return cbor.Marshal(sCBOR)
}

func (s *SignatureMap) UnmarshalCBOR(b []byte) error {
	var sCBOR signaturesCBOR
	if err := cbor.Unmarshal(b, &sCBOR); err != nil {
		return fmt.Errorf("cbor unmarshal failed, %w", err)
	}
	sigMap := make(SignatureMap)
	for _, sig := range sCBOR {
		sigMap[sig.NodeID] = sig.Signature
	}
	*s = sigMap
	return nil
}

func (s *SignatureMap) Bytes() []byte {
	var b bytes.Buffer
	for auth, sig := range *s {
		b.Write([]byte(auth))
		b.Write(sig)
	}
	return b.Bytes()
}

// NewTimestamp - returns timestamp in seconds from epoch
func NewTimestamp() uint64 {
	// Epoch in seconds
	return uint64(time.Now().Unix())
}

func (x *UnicitySeal) IsValid(verifiers map[string]crypto.Verifier) error {
	if x == nil {
		return ErrUnicitySealIsNil
	}
	if len(verifiers) == 0 {
		return ErrRootValidatorInfoMissing
	}
	if x.Hash == nil {
		return ErrUnicitySealHashIsNil
	}
	if x.RootChainRoundNumber < 1 {
		return ErrInvalidBlockNumber
	}
	if x.Timestamp < GenesisTime {
		return errInvalidTimestamp
	}
	if len(x.Signatures) == 0 {
		return ErrUnicitySealSignatureIsNil
	}
	return x.Verify(verifiers)
}

func (x *UnicitySeal) Sign(id string, signer crypto.Signer) error {
	if signer == nil {
		return ErrSignerIsNil
	}
	sig, err := signer.SignBytes(x.Bytes())
	if err != nil {
		return fmt.Errorf("sign failed, %w", err)
	}
	// initiate signatures
	if x.Signatures == nil {
		x.Signatures = make(map[string][]byte)
	}
	x.Signatures[id] = sig
	return nil
}

func (x *UnicitySeal) Bytes() []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(x.RootChainRoundNumber))
	b.Write(util.Uint64ToBytes(x.Timestamp))
	b.Write(x.PreviousHash)
	b.Write(x.Hash)
	return b.Bytes()
}

func (x *UnicitySeal) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.Bytes())
}

func (x *UnicitySeal) Verify(verifiers map[string]crypto.Verifier) error {
	if verifiers == nil {
		return ErrRootValidatorInfoMissing
	}
	if len(x.Signatures) == 0 {
		return errUnicitySealNoSignature
	}
	// Verify all signatures, all must be from known origin and valid
	for id, sig := range x.Signatures {
		// Find verifier info
		ver, f := verifiers[id]
		if !f {
			return ErrUnknownSigner
		}
		err := ver.VerifyBytes(sig, x.Bytes())
		if err != nil {
			return fmt.Errorf("invalid unicity seal signature, %w", err)
		}
	}
	return nil
}
