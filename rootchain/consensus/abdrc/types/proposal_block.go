package types

import (
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"
	"strings"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
)

var (
	errMissingPayload           = errors.New("proposed block is missing payload")
	errMissingQuorumCertificate = errors.New("proposed block is missing quorum certificate")
)

type BlockData struct {
	_         struct{} `cbor:",toarray"`
	Author    string   `json:"author,omitempty"` // NodeIdentifier of the proposer
	Round     uint64   `json:"round,omitempty"`  // Root round number
	Epoch     uint64   `json:"epoch,omitempty"`  // Epoch to establish valid configuration
	Timestamp uint64   `json:"timestamp,omitempty"`
	Payload   *Payload `json:"payload,omitempty"` // Payload that will trigger changes to the state
	// quorum certificate for ancestor
	// before payload can be applied check that local state matches state in qc
	// qc.vote_info.proposed.state_hash == h(UC[])
	Qc *QuorumCert `json:"qc,omitempty"`
}

type Payload struct {
	_        struct{}       `cbor:",toarray"`
	Requests []*IRChangeReq `json:"requests,omitempty"` // IR change requests with quorum or no quorum possible
}

func (x *Payload) AddToHasher(hasher hash.Hash) {
	if x != nil {
		for _, r := range x.Requests {
			r.AddToHasher(hasher)
		}
	}
}

func (x *Payload) IsValid() error {
	// there can only be one request per system identifier in a block
	sysIdSet := map[types.SystemID]struct{}{}

	for _, req := range x.Requests {
		if err := req.IsValid(); err != nil {
			return fmt.Errorf("invalid IR change request for %s: %w", req.SystemIdentifier, err)
		}
		// Timeout requests do not contain proof
		if req.CertReason == T2Timeout && len(req.Requests) > 0 {
			return fmt.Errorf("partition %s timeout proof contains requests", req.SystemIdentifier)
		}
		if _, found := sysIdSet[req.SystemIdentifier]; found {
			return fmt.Errorf("duplicate requests for partition %s", req.SystemIdentifier)
		}
		sysIdSet[req.SystemIdentifier] = struct{}{}
	}
	return nil
}

func (x *Payload) IsEmpty() bool {
	return len(x.Requests) == 0
}

func (x *BlockData) IsValid() error {
	if x.Round < 1 {
		return errRoundNumberUnassigned
	}
	if x.Payload == nil {
		return errMissingPayload
	}
	// does not verify request signatures, this will need to be done later
	if err := x.Payload.IsValid(); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	if x.Qc == nil {
		return errMissingQuorumCertificate
	}
	if err := x.Qc.IsValid(); err != nil {
		return fmt.Errorf("invalid quorum certificate: %w", err)
	}
	if x.Round <= x.Qc.VoteInfo.RoundNumber {
		return fmt.Errorf("invalid block round %d, round is less or equal to QC round %d", x.Round, x.Qc.VoteInfo.RoundNumber)
	}
	return nil
}

func (x *BlockData) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("invalid block data: %w", err)
	}
	if err := x.Qc.Verify(quorum, rootTrust); err != nil {
		return fmt.Errorf("invalid block data QC: %w", err)
	}
	return nil
}

func (x *BlockData) Hash(algo gocrypto.Hash) []byte {
	hasher := algo.New()
	// Block ID is defined as block hash, so hence it is not included
	hasher.Write([]byte(x.Author))
	hasher.Write(util.Uint64ToBytes(x.Round))
	hasher.Write(util.Uint64ToBytes(x.Epoch))
	hasher.Write(util.Uint64ToBytes(x.Timestamp))
	x.Payload.AddToHasher(hasher)
	// From QC signatures (in the alphabetical order of signer ID!) must be included
	// Genesis block does not have a QC
	x.Qc.AddSignersToHasher(hasher)
	return hasher.Sum(nil)
}

func (x *BlockData) GetRound() uint64 {
	if x != nil {
		return x.Round
	}
	return 0
}

func (x *BlockData) GetParentRound() uint64 {
	if x != nil {
		return x.Qc.GetRound()
	}
	return 0
}

// Summary - stringer returns a payload summary
func (x *BlockData) String() string {
	if x.Payload == nil || len(x.Payload.Requests) == 0 {
		return fmt.Sprintf("round: %v, time: %v, payload: empty", x.Round, x.Timestamp)
	}
	var changed []string
	for _, req := range x.Payload.Requests {
		changed = append(changed, req.String())
	}
	return fmt.Sprintf("round: %v, time: %v, payload: %s", x.Round, x.Timestamp, strings.Join(changed, ", "))
}
