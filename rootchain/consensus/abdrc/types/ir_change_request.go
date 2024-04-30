package types

import (
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/network/protocol/certification"
	"github.com/alphabill-org/alphabill/rootchain/consensus"
	"github.com/alphabill-org/alphabill/rootchain/partitions"
	"github.com/alphabill-org/alphabill-go-sdk/types"
	"github.com/alphabill-org/alphabill-go-sdk/util"
)

const (
	Quorum IRChangeReason = iota
	QuorumNotPossible
	T2Timeout
)

type IRChangeReason uint8

type IRChangeReq struct {
	_                struct{}       `cbor:",toarray"`
	SystemIdentifier types.SystemID `json:"system_identifier,omitempty"`
	CertReason       IRChangeReason `json:"cert_reason,omitempty"`
	// IR change (quorum or no quorum possible of block certification requests)
	Requests []*certification.BlockCertificationRequest `json:"requests,omitempty"`
}

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

func (x *IRChangeReq) Verify(tb partitions.PartitionTrustBase, luc *types.UnicityCertificate, round, t2InRounds uint64) (*types.InputRecord, error) {
	if tb == nil {
		return nil, errors.New("trust base is nil")
	}
	if err := x.IsValid(); err != nil {
		return nil, fmt.Errorf("ir change request validation failed: %w", err)
	}
	// quick sanity check, there cannot be more requests than known partition nodes
	if len(x.Requests) > int(tb.GetTotalNodes()) {
		return nil, fmt.Errorf("proof contains more requests than registered partition nodes")
	}
	// verify IR change proof
	// monitor hash counts
	hashCnt := make(map[string]uint64)
	// duplicate requests
	nodeIDs := make(map[string]struct{})
	// validate all block request in the proof
	for _, req := range x.Requests {
		if err := tb.Verify(req.NodeIdentifier, req); err != nil {
			return nil, fmt.Errorf("request proof from partition %s node %v is not valid: %w",
				req.SystemIdentifier, req.NodeIdentifier, err)
		}
		if x.SystemIdentifier != req.SystemIdentifier {
			return nil, fmt.Errorf("invalid partition %s proof: node %v request system id %s does not match request",
				x.SystemIdentifier, req.NodeIdentifier, req.SystemIdentifier)
		}
		if err := consensus.CheckBlockCertificationRequest(req, luc); err != nil {
			return nil, fmt.Errorf("invalid certification request: %w", err)
		}
		if _, found := nodeIDs[req.NodeIdentifier]; found {
			return nil, fmt.Errorf("invalid partition %s proof: contains duplicate request from node %v",
				x.SystemIdentifier, req.NodeIdentifier)
		}
		// register node id
		nodeIDs[req.NodeIdentifier] = struct{}{}
		// get hash of IR and add to hash counter
		hasher := gocrypto.SHA256.New()
		req.InputRecord.AddToHasher(hasher)
		count := hashCnt[string(hasher.Sum(nil))]
		hashCnt[string(hasher.Sum(nil))] = count + 1
	}
	// match request type to proof
	switch x.CertReason {
	case Quorum:
		// 1. require that all input records served as proof are the same
		// reject requests carrying redundant info, there is no use for proofs that do not participate in quorum
		// perhaps this is a bit harsh, but let's not waste bandwidth
		if len(hashCnt) != 1 {
			return nil, fmt.Errorf("invalid partition %s quorum proof: contains proofs for different state hashes", x.SystemIdentifier)
		}
		// 2. more than 50% of the nodes must have voted for the same IR
		if count := getMaxHashCount(hashCnt); count < tb.GetQuorum() {
			return nil, fmt.Errorf("invalid partition %s quorum proof: not enough requests to prove quorum", x.SystemIdentifier)
		}
		// if any request did not extend previous state the whole IRChange request is rejected in validation step
		// NB! there was at least one request, otherwise we would not be here
		return x.Requests[0].InputRecord, nil
	case QuorumNotPossible:
		// Verify that enough partition nodes have voted for different IR change
		// a) find how many votes are missing (nof nodes - requests)
		// b) if the missing votes would also vote for the most popular hash, it must be still not enough to come to a quorum
		if int(tb.GetTotalNodes())-len(x.Requests)+int(getMaxHashCount(hashCnt)) >= int(tb.GetQuorum()) {
			return nil, fmt.Errorf("invalid partition %s no quorum proof: not enough requests to prove only no quorum is possible", x.SystemIdentifier)
		}
		// initiate repeat UC
		return luc.InputRecord.NewRepeatIR(), nil

	case T2Timeout:
		// timout does not carry proof in form of certification requests
		// again this is not fatal in itself, but we should not encourage redundant info
		if len(x.Requests) != 0 {
			return nil, fmt.Errorf("invalid partition %s timeout proof: proof contains requests", x.SystemIdentifier)
		}
		// validate timeout against LUC age
		lucAge := round - luc.UnicitySeal.RootChainRoundNumber
		if lucAge < t2InRounds {
			return nil, fmt.Errorf("invalid partition %s timeout proof: time from latest UC %v, timeout in rounds %v",
				x.SystemIdentifier, lucAge, t2InRounds)
		}
		// initiate repeat UC
		return luc.InputRecord.NewRepeatIR(), nil
	}
	// should be unreachable, since validate method already makes sure that reason is known
	return nil, fmt.Errorf("invalid request: unknown certification reason %v", x.CertReason)
}

func (x *IRChangeReq) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.SystemIdentifier.Bytes())
	hasher.Write(util.Uint32ToBytes(uint32(x.CertReason)))
	for _, req := range x.Requests {
		hasher.Write(req.Bytes())
	}
}

func (x *IRChangeReq) String() string {
	return fmt.Sprintf("%s->%s", x.SystemIdentifier, x.CertReason)
}
