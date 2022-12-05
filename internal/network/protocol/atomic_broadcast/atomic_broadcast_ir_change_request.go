package atomic_broadcast

import (
	"bytes"
	"errors"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/internal/network/protocol"

	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrMissingStateHash             = errors.New("ir change request is missing state hash")
	ErrMissingAuthor                = errors.New("author is missing")
	ErrInvalidCertReason            = errors.New("invalid certification reason")
	ErrUnknownSigner                = errors.New("unknown author")
	ErrTimeoutRequestReasonNotEmpty = errors.New("invalid timeout certification request, proof not empty")
)

func (x *IRChangeReqMsg) IsValid(partitions PartitionStore) error {
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
	// Check requests are valid
	// a) system identifier is the same
	// b) signature is valid
	// rest of the conditions will be checked when applied
	for _, req := range x.Requests {
		if bytes.Equal(req.SystemIdentifier, x.SystemIdentifier) == false {
			return ErrIncompatibleReq
		}
		trustBase, err := partitions.GetTrustBase(protocol.SystemIdentifier(x.SystemIdentifier))
		// partition is unknown?
		if err != nil {
			return err
		}
		v, f := trustBase[req.NodeIdentifier]
		if !f {
			return fmt.Errorf("uknnonwn partition %X node %v", x.SystemIdentifier, req.NodeIdentifier)
		}
		err = v.VerifyBytes(req.Bytes(), req.Signature)
		if err != nil {
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

func (x *IRChangeReqMsg) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.SystemIdentifier)
	b.Write(util.Uint32ToBytes(uint32(x.CertReason)))
	for _, req := range x.Requests {
		b.Write(req.Bytes())
	}
	return b.Bytes()
}
