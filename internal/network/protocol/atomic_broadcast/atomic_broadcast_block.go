package atomic_broadcast

import (
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

var (
	ErrInvalidRound             = errors.New("invalid round number")
	ErrInvalidEpoch             = errors.New("invalid epoch number")
	ErrInvalidSystemId          = errors.New("invalid system identifier")
	ErrMissingPayload           = errors.New("proposed block is missing payload")
	ErrMissingQuorumCertificate = errors.New("proposed block is missing quorum certificate")
	ErrIncompatibleReq          = errors.New("different system identifier and request system identifier")
	ErrUnknownRequest           = errors.New("unknown request, sender not known")
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

type PartitionStore interface {
	GetInfo(id protocol.SystemIdentifier) (partition_store.PartitionInfo, error)
}

func (x *Payload) AddToHasher(hasher hash.Hash) {
	certRequests := x.Requests
	for _, r := range certRequests {
		r.AddToHasher(hasher)
	}
}

func (x *Payload) IsValid(partitions PartitionStore) error {
	// there can only be one request per system identifier in a block
	sysIdSet := map[string]bool{}

	for _, req := range x.Requests {
		sysId := protocol.SystemIdentifier(req.SystemIdentifier)
		info, err := partitions.GetInfo(sysId)
		if err != nil {
			return err
		}
		if err := req.IsValid(info.TrustBase); err != nil {
			return fmt.Errorf("payload contains invalid request from system id %X, err %w", req.SystemIdentifier, err)
		}
		// If reason is timeout, then there is no proof
		if req.CertReason == IRChangeReqMsg_T2_TIMEOUT && len(req.Requests) > 0 {
			return fmt.Errorf("payload is not valid, invalid timeout request")
		}
		if _, found := sysIdSet[string(req.SystemIdentifier)]; found {
			return fmt.Errorf("payload is not valid, duplicate request for system identifier %X", req.SystemIdentifier)
		}
		sysIdSet[string(req.SystemIdentifier)] = true
	}
	return nil
}

func (x *Payload) IsEmpty() bool {
	return len(x.Requests) == 0
}

func (x *BlockData) IsValid(p PartitionStore, v AtomicVerifier) error {
	if x.Round < 1 {
		return ErrInvalidRound
	}
	if x.Epoch < 1 {
		return ErrInvalidEpoch
	}
	if x.Payload == nil {
		return ErrMissingPayload
	}
	if x.Qc == nil {
		return ErrMissingQuorumCertificate
	}
	// expensive op's
	if err := x.Payload.IsValid(p); err != nil {
		return err
	}
	if err := x.Qc.Verify(v); err != nil {
		return err
	}
	return nil
}

func (x *BlockData) Hash(algo gocrypto.Hash) ([]byte, error) {
	hasher := algo.New()
	// Block ID is defined as block hash, so hence it is not included
	hasher.Write([]byte(x.Author))
	hasher.Write(util.Uint64ToBytes(x.Round))
	hasher.Write(util.Uint64ToBytes(x.Epoch))
	hasher.Write(util.Uint64ToBytes(x.Timestamp))
	x.Payload.AddToHasher(hasher)
	x.Qc.AddToHasher(hasher)
	return hasher.Sum(nil), nil
}
