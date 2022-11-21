package atomic_broadcast

import (
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/internal/network/protocol"

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
}

type PartitionVerifier interface {
	VerifySignature(id protocol.SystemIdentifier, nodeId string, sig []byte, tlg []byte) error
}

func (x *Payload) AddToHasher(hasher hash.Hash) {
	certRequests := x.Requests
	for _, r := range certRequests {
		r.AddToHasher(hasher)
	}
}

func (x *Payload) IsValid(partitionVer PartitionVerifier) error {
	// there can only be one request per system identifier in a block
	sysIdSet := map[string]bool{}

	for _, req := range x.Requests {
		if err := req.IsValid(partitionVer); err != nil {
			return fmt.Errorf("invalid payload: %X invalid proof, err %w", req.SystemIdentifier, err)
		}
		// Timeout requests do not contain proof
		if req.CertReason == IRChangeReqMsg_T2_TIMEOUT && len(req.Requests) > 0 {
			return fmt.Errorf("invalid payload: %X timeout contains requests", req.SystemIdentifier)
		}
		if _, found := sysIdSet[string(req.SystemIdentifier)]; found {
			return fmt.Errorf("invalid payload: duplicate request for %X", req.SystemIdentifier)
		}
		sysIdSet[string(req.SystemIdentifier)] = true
	}
	return nil
}

func (x *Payload) IsEmpty() bool {
	return len(x.Requests) == 0
}

func (x *BlockData) IsValid(partitionVer PartitionVerifier, rootVer AtomicVerifier) error {
	if len(x.Id) < 1 {
		return ErrInvalidBlockId
	}
	if x.Round < 1 {
		return ErrInvalidRound
	}
	if x.Payload == nil {
		return ErrMissingPayload
	}
	if x.Qc == nil {
		return ErrMissingQuorumCertificate
	}
	// expensive op's
	if err := x.Payload.IsValid(partitionVer); err != nil {
		return err
	}
	if err := x.Qc.Verify(rootVer); err != nil {
		return err
	}
	return nil
}

func (x *BlockData) Hash(algo gocrypto.Hash) ([]byte, error) {
	if x.Payload == nil {
		return nil, ErrMissingPayload
	}
	if x.Qc == nil {
		return nil, ErrMissingQuorumCertificate
	}
	if err := x.Qc.IsValid(); err != nil {
		return nil, err
	}
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
