package atomic_broadcast

import (
	"bytes"
	gocrypto "crypto"
	"fmt"
	"hash"
	"time"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/rootvalidator/partition_store"
	"github.com/alphabill-org/alphabill/internal/util"
)

func getMaxHashCount(hashCnt map[string]uint64) uint64 {
	var cnt uint64 = 0
	for _, c := range hashCnt {
		if c > cnt {
			cnt = c
		}
	}
	return cnt
}

func (x *IRChangeReqMsg) IsValid() error {
	if len(x.SystemIdentifier) != 4 {
		return fmt.Errorf("invalid ir change request, invalid system identifier %v", x.SystemIdentifier)
	}
	// ignore other values for now, just make sure it is not negative
	if x.CertReason < 0 || x.CertReason > IRChangeReqMsg_T2_TIMEOUT {
		return fmt.Errorf("invalid ir change request, unknown reason %v", x.CertReason)
	}
	return nil
}

func (x *IRChangeReqMsg) Verify(partitionInfo partition_store.PartitionInfo, luc *certificates.UnicityCertificate, lucAge time.Duration) (*certificates.InputRecord, error) {
	if err := x.IsValid(); err != nil {
		return nil, err
	}
	nofNodes := len(partitionInfo.TrustBase)
	// quick sanity check, there cannot be more requests than known partition nodes
	if len(x.Requests) > len(partitionInfo.TrustBase) {
		return nil, fmt.Errorf("invalid ir change request, proof contains more requests than registered partition nodes")
	}
	// verify IR change proof
	// monitor hash counts
	hashCnt := make(map[string]uint64)
	// duplicate requests
	nodeIDs := make(map[string]struct{})
	for _, req := range x.Requests {
		v, f := partitionInfo.TrustBase[req.NodeIdentifier]
		if !f {
			return nil, fmt.Errorf("invalid ir change request, unknown node %v for partition %X", req.NodeIdentifier, x.SystemIdentifier)
		}
		if err := req.IsValid(v); err != nil {
			return nil, fmt.Errorf("invalid ir change request, proof from system id %X node %v is not valid: %w",
				req.SystemIdentifier, req.NodeIdentifier, err)
		}
		if !bytes.Equal(x.SystemIdentifier, req.SystemIdentifier) {
			return nil, fmt.Errorf("invalid ir change request proof, %X node %v proof system id %X does not match",
				x.SystemIdentifier, req.NodeIdentifier, req.SystemIdentifier)
		}
		// todo: AB-505 add partition round number and epoch check, also skipping root round number checks for now as they are not part of the new spec.
		// validate against last unicity certificate
		if !bytes.Equal(req.InputRecord.PreviousHash, luc.InputRecord.Hash) {
			return nil, fmt.Errorf("invalid ir change request proof, partition %X validator %v input records does not extend last certified",
				x.SystemIdentifier, req.NodeIdentifier)
		}
		if _, found := nodeIDs[req.NodeIdentifier]; found {
			return nil, fmt.Errorf("invalid ir change request proof, partition %X proof contains duplicate request from node %v",
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
	case IRChangeReqMsg_QUORUM:
		// 1. require that all input records served as proof are the same
		// reject requests carrying redundant info, there is no use for proofs that do not participate in quorum
		// perhaps this is a bit harsh, but let's not waste bandwidth
		if len(hashCnt) != 1 {
			return nil, fmt.Errorf("invalid ir change request quorum proof, partition %X contains proofs for no quorum", x.SystemIdentifier)
		}
		// 2. more than 50% of the nodes must have voted for the same IR
		if count := getMaxHashCount(hashCnt); count < partitionInfo.GetQuorum() {
			return nil, fmt.Errorf("invalid ir change request quorum proof, partition %X not enough requests to prove quorum", x.SystemIdentifier)
		}
		// NB! there was at least one request, otherwise we would not be here
		return x.Requests[0].InputRecord, nil
	case IRChangeReqMsg_QUORUM_NOT_POSSIBLE:
		// Verify that enough partition nodes have voted for different IR change
		// a) find how many votes are missing (nof nodes - requests)
		// b) if the missing votes would also vote for the most popular hash, it must be still not enough to come to a quorum
		if nofNodes-len(x.Requests)+int(getMaxHashCount(hashCnt)) >= int(partitionInfo.GetQuorum()) {
			return nil, fmt.Errorf("invalid ir change request no quorum proof for %X, not enough requests to prove no quorum is possible", x.SystemIdentifier)
		}
		// initiate repeat UC
		return luc.InputRecord, nil
	case IRChangeReqMsg_T2_TIMEOUT:
		// timout does not carry proof in form of certification requests
		// again this is not fatal in itself, but we should not encourage redundant info
		if len(x.Requests) != 0 {
			return nil, fmt.Errorf("invalid ir change request timeout proof, proof contains requests")
		}
		// validate timeout against LUC age
		if lucAge < time.Duration(partitionInfo.SystemDescription.T2Timeout)*time.Millisecond {
			return nil, fmt.Errorf("invalid ir change request timeout proof, partition %X time from latest UC %v, timeout %v",
				x.SystemIdentifier, lucAge, partitionInfo.SystemDescription.T2Timeout)
		}
		// initiate repeat UC
		return luc.InputRecord, nil
	}
	// should be unreachable, since validate method already makes sure that reason is known
	return nil, fmt.Errorf("unknown certificatio reason %v", x.CertReason)
}

func (x *IRChangeReqMsg) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.SystemIdentifier)
	hasher.Write(util.Uint32ToBytes(uint32(x.CertReason)))
	for _, req := range x.Requests {
		hasher.Write(req.Bytes())
	}
}
