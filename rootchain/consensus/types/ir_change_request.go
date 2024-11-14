package types

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	abcrypto "github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
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

	TrustBase interface {
		GetQuorum() uint64
		GetTotalNodes() uint64
		Verify(nodeID string, f func(v abcrypto.Verifier) error) error
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

func (x *IRChangeReq) Verify(tb TrustBase, luc *types.UnicityCertificate, round, t2InRounds uint64) (*types.InputRecord, error) {
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
		if err := tb.Verify(req.NodeIdentifier, req.IsValid); err != nil {
			return nil, fmt.Errorf("request proof from partition %s node %v is not valid: %w",
				req.Partition, req.NodeIdentifier, err)
		}
		if x.Partition != req.Partition {
			return nil, fmt.Errorf("invalid partition %s proof: node %v request partition id %s does not match request",
				x.Partition, req.NodeIdentifier, req.Partition)
		}
		if err := CheckBlockCertificationRequest(req, luc); err != nil {
			return nil, fmt.Errorf("invalid certification request: %w", err)
		}
		if _, found := nodeIDs[req.NodeIdentifier]; found {
			return nil, fmt.Errorf("invalid partition %s proof: contains duplicate request from node %v",
				x.Partition, req.NodeIdentifier)
		}
		// register node id
		nodeIDs[req.NodeIdentifier] = struct{}{}
		// get hash of IR and add to hash counter
		hasher := crypto.SHA256.New()
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
		// Verify that enough partition nodes have voted for different IR change
		// a) find how many votes are missing (nof nodes - requests)
		// b) if the missing votes would also vote for the most popular hash, it must be still not enough to come to a quorum
		if int(tb.GetTotalNodes())-len(x.Requests)+int(getMaxHashCount(hashCnt)) >= int(tb.GetQuorum()) {
			return nil, fmt.Errorf("invalid partition %s no quorum proof: not enough requests to prove only no quorum is possible", x.Partition)
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

// Bytes serializes entire struct.
func (x *IRChangeReq) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.Partition.Bytes())
	b.Write(util.Uint32ToBytes(uint32(x.CertReason)))
	for _, req := range x.Requests {
		b.Write(req.Bytes())
	}
	return b.Bytes()
}

func (x *IRChangeReq) String() string {
	return fmt.Sprintf("%s->%s", x.Partition, x.CertReason)
}

func CheckBlockCertificationRequest(req CertRequestVerifier, luc *types.UnicityCertificate) error {
	if req == nil {
		return errors.New("block certification request is nil")
	}
	if luc == nil {
		return errors.New("unicity certificate is nil")
	}
	if req.IRRound() != luc.InputRecord.RoundNumber+1 {
		// Older UC, return current.
		return fmt.Errorf("invalid partition round number %v, last certified round number %v", req.IRRound(), luc.InputRecord.RoundNumber)
	} else if !bytes.Equal(req.IRPreviousHash(), luc.InputRecord.Hash) {
		// Extending of unknown State.
		return fmt.Errorf("request extends unknown state: expected hash: %v, got: %v", luc.InputRecord.Hash, req.IRPreviousHash())
	}
	return nil
}
