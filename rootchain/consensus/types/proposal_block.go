package types

import (
	gocrypto "crypto"
	"errors"
	"fmt"
	"strings"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
)

var (
	errMissingPayload           = errors.New("proposed block is missing payload")
	errMissingQuorumCertificate = errors.New("proposed block is missing quorum certificate")
)

type BlockData struct {
	_         struct{} `cbor:",toarray"`
	Version   types.ABVersion
	Author    string   `json:"author"` // NodeID of the proposer
	Round     uint64   `json:"round"`  // Root round number
	Epoch     uint64   `json:"epoch"`  // Epoch to establish valid configuration
	Timestamp uint64   `json:"timestamp"`
	Payload   *Payload `json:"payload"` // Payload that will trigger changes to the state
	// quorum certificate for ancestor
	// before payload can be applied check that local state matches state in qc
	// qc.vote_info.proposed.state_hash == h(UC[])
	Qc *QuorumCert `json:"qc"`
}

type Payload struct {
	_        struct{}       `cbor:",toarray"`
	Requests []*IRChangeReq `json:"requests"` // IR change requests with quorum or no quorum possible
}

func (x *Payload) IsValid() error {
	// there can only be one request per partition identifier in a block
	sysIdSet := map[types.PartitionID]struct{}{}

	for _, req := range x.Requests {
		if err := req.IsValid(); err != nil {
			return fmt.Errorf("invalid IR change request for %s: %w", req.Partition, err)
		}
		// Timeout requests do not contain proof
		if req.CertReason == T2Timeout && len(req.Requests) > 0 {
			return fmt.Errorf("partition %s timeout proof contains requests", req.Partition)
		}
		if _, found := sysIdSet[req.Partition]; found {
			return fmt.Errorf("duplicate requests for partition %s", req.Partition)
		}
		sysIdSet[req.Partition] = struct{}{}
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

func (x *BlockData) Verify(tb types.RootTrustBase) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("invalid block data: %w", err)
	}
	if err := x.Qc.Verify(tb); err != nil {
		return fmt.Errorf("invalid block data QC: %w", err)
	}
	return nil
}

func (x *BlockData) Hash(algo gocrypto.Hash) ([]byte, error) {
	hasher := abhash.New(algo.New())
	hasher.Write(x)
	return hasher.Sum()
}

// Bytes serializes entire struct for hash calculation.
func (x *BlockData) Bytes() ([]byte, error) {
	return x.MarshalCBOR()
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

func (x *BlockData) GetVersion() types.ABVersion {
	if x == nil || x.Version == 0 {
		return 1
	}
	return x.Version
}

func (x *BlockData) MarshalCBOR() ([]byte, error) {
	type alias BlockData
	if x.Version == 0 {
		x.Version = x.GetVersion()
	}
	return types.Cbor.MarshalTaggedValue(types.RootPartitionBlockDataTag, (*alias)(x))
}

func (x *BlockData) UnmarshalCBOR(data []byte) error {
	type alias BlockData
	if err := types.Cbor.UnmarshalTaggedValue(types.RootPartitionBlockDataTag, data, (*alias)(x)); err != nil {
		return err
	}
	return types.EnsureVersion(x, x.Version, 1)
}
