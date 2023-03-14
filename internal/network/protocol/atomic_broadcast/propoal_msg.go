package atomic_broadcast

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrSignerIsNil   = errors.New("signer is nil")
	ErrVerifierIsNil = errors.New("verifier is nil")
)

func (x *ProposalMsg) getLastTcRound() uint64 {
	if x.LastRoundTc == nil {
		return 0
	}
	return x.LastRoundTc.Timeout.Round
}

func (x *ProposalMsg) IsValid() error {
	if x.Block == nil {
		return fmt.Errorf("proposal msg not valid, block is nil")
	}
	if err := x.Block.IsValid(); err != nil {
		return fmt.Errorf("proposal msg not valid, block error: %w", err)
	}
	previousRound := x.Block.Round - 1
	higestCertifiedRound := util.Max(x.Block.Qc.VoteInfo.RoundNumber, x.getLastTcRound())
	// proposal round must follow last round Qc or Tc
	if previousRound != higestCertifiedRound {
		return fmt.Errorf("proposal round %v does not follow certified round %v", x.Block.Round, higestCertifiedRound)
	}
	// if previous round was timeout, then new proposal Block QC must be the same as TC high QC
	// this is the common round from where we will extend the blockchain. verify this too?
	return nil
}

func (x *ProposalMsg) Sign(signer crypto.Signer) error {
	if signer == nil {
		return ErrSignerIsNil
	}
	if err := x.Block.IsValid(); err != nil {
		return err
	}
	hash, err := x.Block.Hash(gocrypto.SHA256)
	if err != nil {
		return err
	}
	// Sign block hash
	signature, err := signer.SignHash(hash)
	if err != nil {
		return err
	}
	x.Signature = signature
	return nil
}

func (x *ProposalMsg) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	hash, err := x.Block.Hash(gocrypto.SHA256)
	if err != nil {
		return fmt.Errorf("proposal msg, unexpected hash error %w", err)
	}
	// Find author public key
	v, f := rootTrust[x.Block.Author]
	if !f {
		return fmt.Errorf("proposal msg error, failed to find public key for root validator %v", x.Block.Author)
	}
	if err = v.VerifyHash(x.Signature, hash); err != nil {
		return fmt.Errorf("proposal msg signature verification error, %w", err)
	}
	if err = x.IsValid(); err != nil {
		return fmt.Errorf("proposal msg not valid, %w", err)
	}
	// Optional timeout certificate
	if x.LastRoundTc != nil {
		if err = x.LastRoundTc.Verify(quorum, rootTrust); err != nil {
			return fmt.Errorf("proposal msg tc verification failed, %w", err)
		}
	}
	if err = x.Block.Verify(quorum, rootTrust); err != nil {
		return fmt.Errorf("proposal msg block verification failed, %w", err)
	}
	return nil
}
