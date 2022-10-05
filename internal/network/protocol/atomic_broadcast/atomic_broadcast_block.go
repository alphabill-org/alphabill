package atomic_broadcast

import (
	"bytes"
	gocrypto "crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	ErrInvalidRound               = "invalid round number"
	ErrInvalidEpoch               = "invalid epoch number"
	ErrInvalidSystemId            = "invalid system identifier"
	ErrMissingPayload             = "proposed block is missing payload"
	ErrMissingQuorumCertificate   = "proposed block is missing quorum certificate"
	ErrIncompatibleReq            = "different system identifier and request system identifier"
	ErrUnknownRequest             = "unknown request, sender not known"
	ErrBothChangeQuorumAndTimeout = "invalid payload both IR change and timeout set at the same time"
)

type AtomicVerifier interface {
	GetQuorumThreshold() uint32
	VerifySignature(hash []byte, sig []byte, author peer.ID) error
	VerifyBytes(bytes []byte, sig []byte, author peer.ID) error

	ValidateQuorum(authors []string) error
	VerifyQuorumSignatures(hash []byte, signatures map[string][]byte) error
	GetVerifier(nodeId peer.ID) (crypto.Verifier, error)
	GetVerifiers() map[string]crypto.Verifier
}

func (x *IRChangeReq) IsValid(partitionTrustBase map[string]crypto.Verifier) error {
	if len(x.SystemIdentifier) != 4 {
		return errors.New(ErrInvalidSystemId)
	}
	for _, req := range x.Requests {
		if bytes.Equal(req.SystemIdentifier, x.SystemIdentifier) == false {
			return errors.New(ErrIncompatibleReq)
		}
		ver, f := partitionTrustBase[req.NodeIdentifier]
		if !f {
			return errors.New(ErrUnknownRequest)
		}
		if err := req.IsValid(ver); err != nil {
			return err
		}
	}
	return nil
}

func (x *IRChangeReq) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.SystemIdentifier)
	array := x.GetRequests()
	for _, req := range array {
		hasher.Write(req.Bytes())
	}
}

func (x *Payload) AddToHasher(hasher hash.Hash) {
	array := x.IrChangeQuorum
	for _, ir := range array {
		ir.AddToHasher(hasher)
	}
}

func (x *Payload) IsValid(partitionTrustBase map[string]crypto.Verifier) error {
	sysIdMap := map[string]protocol.SystemIdentifier{}
	// Verify all IR change request quorums are valid
	if len(x.IrChangeQuorum) > 0 {
		for _, q := range x.IrChangeQuorum {
			if err := q.IsValid(partitionTrustBase); err != nil {
				return err
			}
			sysIdMap[string(q.SystemIdentifier)] = protocol.SystemIdentifier(q.SystemIdentifier)
		}
	}
	// Check timeout systemId array:
	// 1. check that system ID is valid
	// 2. check that both IR change and timeout for the same system ID are not set at the same time
	for _, systemId := range x.TimeoutSystemIdentifiers {
		if len(systemId) != 4 {
			return errors.New(ErrInvalidSystemId)
		}
		if _, f := sysIdMap[string(systemId)]; f == true {
			return errors.New(ErrBothChangeQuorumAndTimeout)
		}
	}
	return nil
}

func (x *Payload) IsEmpty() bool {
	return len(x.IrChangeQuorum) == 0 && len(x.TimeoutSystemIdentifiers) == 0
}

func (x *BlockData) IsValid(partitionTrust map[string]crypto.Verifier, v AtomicVerifier) error {
	if x.Round < 1 {
		return errors.New(ErrInvalidRound)
	}
	if x.Epoch < 1 {
		return errors.New(ErrInvalidEpoch)
	}
	if x.Payload == nil {
		return errors.New(ErrMissingPayload)
	}
	if x.Qc == nil {
		return errors.New(ErrMissingQuorumCertificate)
	}
	// expensive op's
	if err := x.Payload.IsValid(partitionTrust); err != nil {
		return err
	}
	if err := x.Qc.Verify(v); err != nil {
		return err
	}
	return nil
}

func (x *BlockData) Hash(algo gocrypto.Hash) ([]byte, error) {
	hasher := algo.New()
	hasher.Write(util.Uint64ToBytes(x.Epoch))
	hasher.Write(util.Uint64ToBytes(x.Round))
	hasher.Write([]byte(x.Author))
	x.Payload.AddToHasher(hasher)
	x.Qc.AddToHasher(hasher)
	return hasher.Sum(nil), nil
}
