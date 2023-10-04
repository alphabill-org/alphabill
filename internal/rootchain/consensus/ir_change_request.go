package consensus

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
	"github.com/alphabill-org/alphabill/internal/types"
)

type CertReqReason uint8

const (
	Quorum CertReqReason = iota
	QuorumNotPossible
)

type (
	IRChangeRequest struct {
		SystemIdentifier types.SystemID
		Reason           CertReqReason
		Requests         []*certification.BlockCertificationRequest
	}
)

func CheckBlockCertificationRequest(req *certification.BlockCertificationRequest, luc *types.UnicityCertificate) error {
	if req == nil {
		return errors.New("block certification request is nil")
	}
	if luc == nil {
		return errors.New("unicity certificate is nil")
	}
	if req.InputRecord.RoundNumber != luc.InputRecord.RoundNumber+1 {
		// Older UC, return current.
		return fmt.Errorf("invalid partition round number %v, last certified round number %v", req.InputRecord.RoundNumber, luc.InputRecord.RoundNumber)
	} else if !bytes.Equal(req.InputRecord.PreviousHash, luc.InputRecord.Hash) {
		// Extending of unknown State.
		return fmt.Errorf("request extends unknown state: expected hash: %v, got: %v", luc.InputRecord.Hash, req.InputRecord.PreviousHash)
	} else if req.RootRoundNumber < luc.GetRootRoundNumber() {
		// Stale request, it has been sent before most recent UC was issued
		return fmt.Errorf("stale request, IR's root round number %v, last certified root round number %v", req.RootRoundNumber, luc.UnicitySeal.RootChainRoundNumber)
	}
	return nil
}
