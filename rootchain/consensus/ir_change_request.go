package consensus

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/network/protocol/certification"
)

type CertReqReason uint8

const (
	Quorum CertReqReason = iota
	QuorumNotPossible
)

type (
	IRChangeRequest struct {
		Partition types.SystemID
		Shard     types.ShardID
		Reason    CertReqReason
		Requests  []*certification.BlockCertificationRequest
		Technical certification.TechnicalRecord
	}

	CertRequestVerifier interface {
		IRRound() uint64
		IRPreviousHash() []byte
		RootRound() uint64
	}
)

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
	} else if req.RootRound() != luc.GetRootRoundNumber() {
		// Stale request, it has been sent before most recent UC was issued
		return fmt.Errorf("request root round number %v does not match luc root round %v", req.RootRound(), luc.UnicitySeal.RootChainRoundNumber)
	}
	return nil
}
