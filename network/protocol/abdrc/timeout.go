package abdrc

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/util"
	abdrc "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type TimeoutMsg struct {
	_         struct{}           `cbor:",toarray"`
	Timeout   *abdrc.Timeout     `json:"timeout,omitempty"`
	Author    string             `json:"author,omitempty"`
	Signature []byte             `json:"signature,omitempty"`
	LastTC    *abdrc.TimeoutCert `json:"last_tc,omitempty"` // TC for Timeout.Round−1 if Timeout.HighQC.Round != Timeout.Round−1 (nil otherwise)
}

// NewTimeoutMsg constructs a new atomic broadcast timeout message
func NewTimeoutMsg(timeout *abdrc.Timeout, author string, lastTC *abdrc.TimeoutCert) *TimeoutMsg {
	return &TimeoutMsg{Timeout: timeout, Author: author, LastTC: lastTC}
}

func (x *TimeoutMsg) Bytes() []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(x.Timeout.Round))
	b.Write(util.Uint64ToBytes(x.Timeout.Epoch))
	b.Write(util.Uint64ToBytes(x.Timeout.HighQc.VoteInfo.RoundNumber))
	b.Write([]byte(x.Author))
	return b.Bytes()
}

func (x *TimeoutMsg) IsValid() error {
	if x.Timeout == nil {
		return fmt.Errorf("timeout info is nil")
	}
	if err := x.Timeout.IsValid(); err != nil {
		return fmt.Errorf("invalid timeout data: %w", err)
	}
	if x.Author == "" {
		return fmt.Errorf("timeout message is missing author")
	}

	// if highQC is not for previous round we must have TC for previous round
	if prevRound := x.GetRound() - 1; prevRound != x.Timeout.HighQc.GetRound() {
		if x.LastTC == nil {
			return fmt.Errorf("last TC is missing for round %d", prevRound)
		}
		if err := x.LastTC.IsValid(); err != nil {
			return fmt.Errorf("invalid timeout certificate: %w", err)
		}
		if prevRound != x.LastTC.GetRound() {
			return fmt.Errorf("last TC must be for round %d but is for round %d", prevRound, x.LastTC.GetRound())
		}
	}
	return nil
}

func (x *TimeoutMsg) Verify(tb types.RootTrustBase) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}
	if _, err := tb.VerifySignature(x.Bytes(), x.Signature, x.Author); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	if err := x.Timeout.Verify(tb); err != nil {
		return fmt.Errorf("timeout data verification failed: %w", err)
	}
	if x.LastTC != nil {
		if err := x.LastTC.Verify(tb); err != nil {
			return fmt.Errorf("invalid last TC: %w", err)
		}
	}
	return nil
}

func (x *TimeoutMsg) Sign(s crypto.Signer) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("timeout validation failed, %w", err)
	}
	sig, err := s.SignBytes(x.Bytes())
	if err != nil {
		return fmt.Errorf("sign error, %w", err)
	}
	x.Signature = sig
	return nil
}

func (x *TimeoutMsg) GetRound() uint64 {
	if x == nil || x.Timeout == nil {
		return 0
	}
	return x.Timeout.Round
}
