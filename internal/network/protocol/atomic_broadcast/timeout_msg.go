package atomic_broadcast

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/util"
)

// NewTimeout creates new Timeout for round (epoch) and highest QC seen
func NewTimeout(round, epoch uint64, hqc *QuorumCert) *Timeout {
	return &Timeout{Epoch: epoch, Round: round, HighQc: hqc}
}

func (x *Timeout) IsValid() error {
	if x.Round == 0 {
		return fmt.Errorf("timeout info round is 0")
	}
	if x.HighQc == nil {
		return fmt.Errorf("timeout info high qc is nil")
	}
	if err := x.HighQc.IsValid(); err != nil {
		return fmt.Errorf("timeout info invalid high qc, %w", err)
	}
	if x.Round <= x.HighQc.VoteInfo.RootRound {
		return fmt.Errorf("timeout info round is smaller or equal to highest qc seen")
	}
	return nil
}

// NewTimeoutMsg constructs a new atomic broadcast timeout message
func NewTimeoutMsg(timeout *Timeout, author string) *TimeoutMsg {
	return &TimeoutMsg{Timeout: timeout, Author: author}
}

func (x *TimeoutMsg) Bytes() []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(x.Timeout.Round))
	b.Write(util.Uint64ToBytes(x.Timeout.Epoch))
	b.Write(util.Uint64ToBytes(x.Timeout.HighQc.VoteInfo.RootRound))
	b.Write([]byte(x.Author))
	return b.Bytes()
}

func BytesFromTimeoutVote(t *Timeout, author string, vote *TimeoutVote) []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(t.Round))
	b.Write(util.Uint64ToBytes(t.Epoch))
	b.Write(util.Uint64ToBytes(vote.HqcRound))
	b.Write([]byte(author))
	return b.Bytes()
}

func (x *TimeoutMsg) IsValid() error {
	if x.Timeout == nil {
		return fmt.Errorf("invalid timeout: timeout info is nil")
	}
	if err := x.Timeout.IsValid(); err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}
	if len(x.Author) < 1 {
		return fmt.Errorf("invalid timeout: missing author")
	}
	return nil
}

func (x *TimeoutMsg) Sign(s crypto.Signer) error {
	if err := x.IsValid(); err != nil {
		return err
	}
	sig, err := s.SignBytes(x.Bytes())
	if err != nil {
		return err
	}
	x.Signature = sig
	return nil
}

func (x *TimeoutMsg) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	// verify signature
	v, f := rootTrust[x.Author]
	if !f {
		return fmt.Errorf("timeout message verify failed: unable to find public key for author %v", x.Author)
	}
	// verify signature
	if err := v.VerifyBytes(x.Signature, x.Bytes()); err != nil {
		return fmt.Errorf("timeout message verify failed: invalid signature")
	}
	if err := x.Timeout.HighQc.Verify(quorum, rootTrust); err != nil {
		return fmt.Errorf("timeout message verify failed: %w", err)
	}
	return nil
}
