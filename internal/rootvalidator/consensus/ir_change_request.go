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
	seal := luc.UnicitySeal
	if req.RootRoundNumber < seal.RootRoundInfo.RoundNumber {
		// Older UC, return current.
		return fmt.Errorf("old request: root round number %v, partition node round number %v", seal.RootRoundInfo.RoundNumber, req.RootRoundNumber)
	} else if req.RootRoundNumber > seal.RootRoundInfo.RoundNumber {
		// should not happen, partition has newer UC
		return fmt.Errorf("partition has never unicity certificate: root round number %v, partition node round number %v", seal.RootRoundInfo.RoundNumber, req.RootRoundNumber)
	} else if !bytes.Equal(req.InputRecord.PreviousHash, luc.InputRecord.Hash) {
		// Extending of unknown State.
		return fmt.Errorf("request extends unknown state: expected hash: %v, got: %v", seal.CommitInfo.RootHash, req.InputRecord.PreviousHash)
	}
	return nil
}
