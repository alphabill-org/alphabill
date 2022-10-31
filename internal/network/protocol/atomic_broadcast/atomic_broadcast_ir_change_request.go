package atomic_broadcast

import (
	"bytes"
	"errors"
	"hash"

	aberrors "github.com/alphabill-org/alphabill/internal/errors"

	"github.com/alphabill-org/alphabill/internal/util"

	"github.com/alphabill-org/alphabill/internal/crypto"
)

var (
	ErrMissingStateHash  = errors.New("ir change request is missing state hash")
	ErrMissingAuthor     = errors.New("author is missing")
	ErrInvalidCertReason = errors.New("invalid certification reason")
	ErrUnknownSigner     = errors.New("unknown author")
)

func (x *CertificationReqWithProof) IsValid(partitionTrustBase map[string]crypto.Verifier) error {
	if len(x.SystemIdentifier) != 4 {
		return ErrInvalidSystemId
	}
	// ignore other values for now, just make sure it is not negative
	if x.CertReason < 0 {
		return ErrInvalidCertReason
	}
	// Check requests are valid
	// a) system identifier is the same
	// b) signature is valid
	// rest of the conditions will be checked when applied
	for _, req := range x.Requests {
		if bytes.Equal(req.SystemIdentifier, x.SystemIdentifier) == false {
			return ErrIncompatibleReq
		}
		ver, f := partitionTrustBase[req.NodeIdentifier]
		if !f {
			return ErrUnknownRequest
		}
		if err := req.IsValid(ver); err != nil {
			return aberrors.Wrap(err, "certification request ")
		}
	}
	return nil
}

func (x *CertificationReqWithProof) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.SystemIdentifier)
	hasher.Write(util.Uint32ToBytes(uint32(x.CertReason)))
	for _, req := range x.Requests {
		hasher.Write(req.Bytes())
	}
}

func (x *CertificationReqWithProof) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.SystemIdentifier)
	b.Write(util.Uint32ToBytes(uint32(x.CertReason)))
	for _, req := range x.Requests {
		b.Write(req.Bytes())
	}
	return b.Bytes()
}

func (x *IRChangeReqMsg) IsValid(rootTrustBase, partitionTrustBase map[string]crypto.Verifier) error {
	if len(x.StateHash) == 0 {
		return ErrMissingStateHash
	}
	if len(x.Author) == 0 {
		return ErrMissingAuthor
	}
	// verify message, before looking into the request
	// 1. Find author from trust base
	v, found := rootTrustBase[x.Author]
	if !found {
		return ErrUnknownSigner
	}
	// 2. Verify signature
	if err := v.VerifyBytes(x.Signature, x.Bytes()); err != nil {
		return err
	}

	// Is request valid
	if err := x.Request.IsValid(partitionTrustBase); err != nil {
		return err
	}
	return nil
}

func (x *IRChangeReqMsg) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.StateHash)
	x.Request.AddToHasher(hasher)
	hasher.Write([]byte(x.Author))
}

func (x *IRChangeReqMsg) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.StateHash)
	b.Write(x.Request.Bytes())
	b.Write([]byte(x.Author))
	return b.Bytes()
}

func (x *IRChangeReqMsg) Sign(signer crypto.Signer) error {
	if signer == nil {
		return errors.New("signer is nil")
	}
	signature, err := signer.SignBytes(x.Bytes())
	if err != nil {
		return err
	}
	x.Signature = signature
	return nil
}
