package ab_consensus

import (
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"
	"sort"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	errInvalidRound             = errors.New("invalid round number")
	errMissingPayload           = errors.New("proposed block is missing payload")
	errMissingQuorumCertificate = errors.New("proposed block is missing quorum certificate")
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
			return fmt.Errorf("IR change request for %X not valid, %w", req.SystemIdentifier, err)
		}
		// Timeout requests do not contain proof
		if req.CertReason == IRChangeReqMsg_T2_TIMEOUT && len(req.Requests) > 0 {
			return fmt.Errorf("partition %X timeout proof contains requests", req.SystemIdentifier)
		}
		if _, found := sysIdSet[string(req.SystemIdentifier)]; found {
			return fmt.Errorf("duplicate requests for parition %X", req.SystemIdentifier)
		}
		sysIdSet[string(req.SystemIdentifier)] = true
	}
	return nil
}

func (x *Payload) IsEmpty() bool {
	return len(x.Requests) == 0
}

func (x *BlockData) IsValid() error {
	if x.Round < 1 {
		return errInvalidRound
	}
	if x.Payload == nil {
		return errMissingPayload
	}
	// does not verify request signatures, this will need to be done later
	if err := x.Payload.IsValid(); err != nil {
		return fmt.Errorf("payload validation failed, %w", err)
	}
	if x.Qc == nil {
		return errMissingQuorumCertificate
	}
	if err := x.Qc.IsValid(); err != nil {
		return fmt.Errorf("quorum certificate validation failed, %w", err)
	}
	if x.Round <= x.Qc.VoteInfo.RoundNumber {
		return fmt.Errorf("invalid block round %v, round is less or equal to qc round %v", x.Round, x.Qc.VoteInfo.RoundNumber)
	}
	return nil
}

func (x *BlockData) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("block data validation failed, %w", err)
	}
	if err := x.Qc.Verify(quorum, rootTrust); err != nil {
		return fmt.Errorf("block data quorum certificate validation failed, %w", err)
	}
	return nil
}

func (x *BlockData) Hash(algo gocrypto.Hash) ([]byte, error) {
	if err := x.IsValid(); err != nil {
		return nil, fmt.Errorf("block data validation failed, %w", err)
	}
	hasher := algo.New()
	// Block ID is defined as block hash, so hence it is not included
	hasher.Write([]byte(x.Author))
	hasher.Write(util.Uint64ToBytes(x.Round))
	hasher.Write(util.Uint64ToBytes(x.Epoch))
	hasher.Write(util.Uint64ToBytes(x.Timestamp))
	x.Payload.AddToHasher(hasher)
	// Only add QC signatures
	signatures := x.Qc.Signatures
	authors := make([]string, 0, len(signatures))
	for k := range signatures {
		authors = append(authors, k)
	}
	// sort the slice by keys
	sort.Strings(authors)
	// add signatures to hash in alphabetical order
	for _, author := range authors {
		sig, _ := signatures[author]
		hasher.Write(sig)
	}
	return hasher.Sum(nil), nil
}

func (x *BlockData) GetParentRound() uint64 {
	if x != nil {
		return x.Qc.GetRound()
	}
	return 0
}
