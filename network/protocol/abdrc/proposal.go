package abdrc

import (
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	abdrc "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

var (
	errSignerIsNil = errors.New("signer is nil")
)

type ProposalMsg struct {
	_           struct{}           `cbor:",toarray"`
	Block       *abdrc.BlockData   `json:"block"`       // Proposed change
	LastRoundTc *abdrc.TimeoutCert `json:"lastRoundTc"` // Last timeout certificate for block.round - 1 if block.qc.round != block.round - 1
	Signature   hex.Bytes          `json:"signature"`
}

func (x *ProposalMsg) getLastTcRound() uint64 {
	if x.LastRoundTc == nil {
		return 0
	}
	return x.LastRoundTc.Timeout.Round
}

func (x *ProposalMsg) IsValid() error {
	if x.Block == nil {
		return fmt.Errorf("block is nil")
	}
	if err := x.Block.IsValid(); err != nil {
		return fmt.Errorf("invalid block: %w", err)
	}
	// proposal round must follow last round Qc or Tc
	highestCertifiedRound := max(x.Block.Qc.VoteInfo.RoundNumber, x.getLastTcRound())
	if x.Block.Round-1 != highestCertifiedRound {
		return fmt.Errorf("proposed block round %d does not follow attached quorum certificate round %d", x.Block.Round, highestCertifiedRound)
	}
	// if previous round was timeout, then new proposal Block QC must be the same as TC high QC
	// this is the common round from where we will extend the blockchain. verify this too?
	return nil
}

func (x *ProposalMsg) Verify(tb types.RootTrustBase) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	if err := x.Block.Verify(tb); err != nil {
		return fmt.Errorf("block verification failed: %w", err)
	}
	if _, err := tb.VerifySignature(x.Block.Bytes(), x.Signature, x.Block.Author); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}

	// Optional timeout certificate
	if x.LastRoundTc != nil {
		if err := x.LastRoundTc.Verify(tb); err != nil {
			return fmt.Errorf("invalid timeout certificate: %w", err)
		}
	}
	return nil
}

func (x *ProposalMsg) Sign(signer crypto.Signer) error {
	if signer == nil {
		return errSignerIsNil
	}
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("invalid proposal: %w", err)
	}

	// Sign block hash
	hash := x.Block.Hash(gocrypto.SHA256)
	signature, err := signer.SignHash(hash)
	if err != nil {
		return fmt.Errorf("sign failed: %w", err)
	}
	x.Signature = signature
	return nil
}
