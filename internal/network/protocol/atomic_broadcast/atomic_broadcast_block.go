package atomic_broadcast

import (
	gocrypto "crypto"
	"errors"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/util"
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

func (x *Payload) AddToHasher(hasher hash.Hash) {
	certRequests := x.Requests
	for _, r := range certRequests {
		r.AddToHasher(hasher)
	}
}

func (x *Payload) IsValid() error {
	// there can only be one request per system identifier in a block
	sysIdSet := map[string]bool{}

	for _, req := range x.Requests {
		if err := req.IsValid(); err != nil {
			return fmt.Errorf("invalid payload: IR change request for %X err %w", req.SystemIdentifier, err)
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

func (x *BlockData) IsValid() error {
	if len(x.Id) < 1 {
		return ErrInvalidBlockId
	}
	if x.Round < 1 {
		return ErrInvalidRound
	}
	if x.Payload == nil {
		return ErrMissingPayload
	}
	// does not verify request signatures, this will need to be done later
	if err := x.Payload.IsValid(); err != nil {
		return err
	}
	if x.Qc == nil {
		return ErrMissingQuorumCertificate
	}
	if err := x.Qc.IsValid(); err != nil {
		return err
	}
	if x.Round <= x.Qc.VoteInfo.RootRound {
		return fmt.Errorf("invalid block round: round %v is not bigger than last qc round %v", x.Round, x.Qc.VoteInfo.RootRound)
	}
	return nil
}

func (x *BlockData) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	if err := x.IsValid(); err != nil {
		return err
	}
	if err := x.Qc.Verify(quorum, rootTrust); err != nil {
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
