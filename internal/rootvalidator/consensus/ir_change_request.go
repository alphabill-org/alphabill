package consensus

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/network/protocol"
	"github.com/alphabill-org/alphabill/internal/network/protocol/certification"
)

type CertReqReason uint8

const (
	Quorum CertReqReason = iota
	QuorumNotPossible
)

type (
	IRChangeRequest struct {
		SystemIdentifier protocol.SystemIdentifier
		Reason           CertReqReason
		IR               *certificates.InputRecord
		Requests         []*certification.BlockCertificationRequest
	}
)

func CheckBlockCertificationRequest(req *certification.BlockCertificationRequest, luc *certificates.UnicityCertificate) error {
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
		return fmt.Errorf("request extends unknown state: expected hash: %v, got: %v", luc.UnicitySeal.CommitInfo.RootHash, req.InputRecord.PreviousHash)
	}
	return nil
}
