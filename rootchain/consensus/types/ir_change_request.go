package types

import (
	"crypto"
	"errors"
	"fmt"

	abhash "github.com/alphabill-org/alphabill-go-base/hash"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

const (
	Quorum IRChangeReason = iota
	QuorumNotPossible
	T2Timeout
)

type (
	IRChangeReason uint8

	IRChangeReq struct {
		_          struct{} `cbor:",toarray"`
		Partition  types.PartitionID
		Shard      types.ShardID
		CertReason IRChangeReason
		// IR change (quorum or no quorum possible of block certification requests)
		Requests []*certification.BlockCertificationRequest
	}

	RequestVerifier interface {
		GetQuorum() uint64
		GetTotalNodes() uint64
		ValidRequest(req *certification.BlockCertificationRequest) error
	}

	CertRequestVerifier interface {
		IRRound() uint64
		IRPreviousHash() []byte
	}
)

func (r IRChangeReason) String() string {
	switch r {
	case Quorum:
		return "quorum"
	case QuorumNotPossible:
		return "no-quorum"
	case T2Timeout:
		return "timeout"
	}
	return fmt.Sprintf("unknown IR change reason %d", int(r))
}

func getMaxHashCount(hashCnt map[string]uint64) uint64 {
	var cnt uint64 = 0
	for _, c := range hashCnt {
		if c > cnt {
			cnt = c
		}
	}
	return cnt
}

func (x *IRChangeReq) IsValid() error {
	// ignore other values for now, just make sure it is not negative
	if x.CertReason > T2Timeout {
		return fmt.Errorf("unknown reason (%d)", x.CertReason)
	}
	return nil
}

func (x *IRChangeReq) Verify(tb RequestVerifier, luc *types.UnicityCertificate, round, t2InRounds uint64) (*types.InputRecord, error) {
	if tb == nil {
		return nil, errors.New("RequestVerifier is unassigned")
	}
	if err := x.IsValid(); err != nil {
		return nil, fmt.Errorf("invalid IR Change Request: %w", err)
	}
	// quick sanity check, there cannot be more requests than known partition nodes
	if len(x.Requests) > int(tb.GetTotalNodes()) {
		return nil, errors.New("IR Change Request contains more requests than registered partition nodes")
	}
	// verify IR change proof
	// monitor hash counts
	hashCnt := make(map[string]uint64)
	// duplicate requests
	nodeIDs := make(map[string]struct{})
	// validate all block request in the proof
	for _, req := range x.Requests {
		if x.Partition != req.PartitionID || !x.Shard.Equal(req.ShardID) {
			return nil, fmt.Errorf("shard of the change request is %s-%s but block certification request is for %s=%s",
				x.Partition, x.Shard, req.PartitionID, req.ShardID)
		}
		if err := tb.ValidRequest(req); err != nil {
			return nil, fmt.Errorf("invalid certification request: %w", err)
		}
		if _, found := nodeIDs[req.NodeID]; found {
			return nil, fmt.Errorf("invalid partition %s proof: contains duplicate request from node %v", x.Partition, req.NodeID)
		}
		// register node id
		nodeIDs[req.NodeID] = struct{}{}
		// get hash of IR and add to hash counter
		hasher := abhash.New(crypto.SHA256.New())
		req.InputRecord.AddToHasher(hasher)
		hash, err := hasher.Sum()
		if err != nil {
			return nil, fmt.Errorf("failed to calculate hash: %w", err)
		}
		count := hashCnt[string(hash)]
		hashCnt[string(hash)] = count + 1
	}
	// match request type to proof
	switch x.CertReason {
	case Quorum:
		// 1. require that all input records served as proof are the same
		// reject requests carrying redundant info, there is no use for proofs that do not participate in quorum
		// perhaps this is a bit harsh, but let's not waste bandwidth
		if len(hashCnt) != 1 {
			return nil, fmt.Errorf("invalid partition %s quorum proof: contains proofs for different state hashes", x.Partition)
		}
		// 2. more than 50% of the nodes must have voted for the same IR
		if count, q := getMaxHashCount(hashCnt), tb.GetQuorum(); count < q {
			return nil, fmt.Errorf("invalid partition %s quorum proof: not enough requests to prove quorum (got %d, need %d)", x.Partition, count, q)
		}
		// if any request did not extend previous state the whole IRChange request is rejected in validation step
		// NB! there was at least one request, otherwise we would not be here
		return x.Requests[0].InputRecord, nil
	case QuorumNotPossible:
		maxHC := getMaxHashCount(hashCnt)
		if maxHC >= tb.GetQuorum() {
			return nil, fmt.Errorf("can't certify 'no quorum' as one input already does have quorum (%d votes, quorum is %d)", maxHC, tb.GetQuorum())
		}
		// Verify that enough partition nodes have voted for different IR change
		// a) find how many votes are missing (nof nodes - requests)
		// b) if the missing votes would also vote for the most popular hash, it must be still not enough to come to a quorum
		if vc := int(tb.GetTotalNodes()) - len(x.Requests) + int(maxHC); vc >= int(tb.GetQuorum()) {
			return nil, fmt.Errorf("not enough votes to prove 'no quorum' - it is possible to get %d votes, quorum is %d", vc, tb.GetQuorum())
		}
		// initiate repeat UC
		return luc.InputRecord.NewRepeatIR(), nil
	case T2Timeout:
		// timeout does not carry proof in form of certification requests
		// again this is not fatal in itself, but we should not encourage redundant info
		if len(x.Requests) != 0 {
			return nil, fmt.Errorf("invalid partition %s timeout proof: proof contains requests", x.Partition)
		}
		// validate timeout against LUC age
		lucAge := round - luc.UnicitySeal.RootChainRoundNumber
		if lucAge < t2InRounds {
			return nil, fmt.Errorf("invalid partition %s timeout proof: time from latest UC %v, timeout in rounds %v",
				x.Partition, lucAge, t2InRounds)
		}
		// initiate repeat UC
		return luc.InputRecord.NewRepeatIR(), nil
	}
	// should be unreachable, since validate method already makes sure that reason is known
	return nil, fmt.Errorf("invalid request: unknown certification reason %v", x.CertReason)
}

func (x *IRChangeReq) String() string {
	return fmt.Sprintf("%s->%s", x.Partition, x.CertReason)
}
