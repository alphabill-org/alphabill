package atomic_broadcast

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrMissingAuthor                = errors.New("author is missing")
	ErrInvalidCertReason            = errors.New("invalid certification reason")
	ErrTimeoutRequestReasonNotEmpty = errors.New("invalid timeout certification request, proof not empty")
)

func (x *IRChangeReqMsg) IsValid() error {
	if len(x.SystemIdentifier) != 4 {
		return ErrInvalidSystemId
	}
	// ignore other values for now, just make sure it is not negative
	if x.CertReason < 0 || x.CertReason > IRChangeReqMsg_T2_TIMEOUT {
		return ErrInvalidCertReason
	}
	if x.CertReason == IRChangeReqMsg_T2_TIMEOUT && len(x.Requests) != 0 {
		return ErrTimeoutRequestReasonNotEmpty
	}
	for _, req := range x.Requests {
		if len(req.NodeIdentifier) == 0 {
			return ErrMissingAuthor
		}
		if bytes.Equal(req.SystemIdentifier, x.SystemIdentifier) == false {
			return ErrIncompatibleReq
		}
	}
	return nil
}

func (x *IRChangeReqMsg) Verify(partitionTrustBase map[string]crypto.Verifier) error {
	if err := x.IsValid(); err != nil {
		return err
	}
	// Check requests signatures are valid
	// rest of the conditions will be checked when applied
	for _, req := range x.Requests {
		v, f := partitionTrustBase[req.NodeIdentifier]
		if !f {
			return fmt.Errorf("invalid IR request, unknown node %v for partition %X", req.NodeIdentifier, x.SystemIdentifier)
		}
		if err := v.VerifyBytes(req.Signature, req.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func (x *IRChangeReqMsg) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.SystemIdentifier)
	hasher.Write(util.Uint32ToBytes(uint32(x.CertReason)))
	for _, req := range x.Requests {
		hasher.Write(req.Bytes())
	}
}
