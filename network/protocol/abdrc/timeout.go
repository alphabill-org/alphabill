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
	_         struct{}       `cbor:",toarray"`
	Timeout   *abdrc.Timeout `json:"timeout,omitempty"`
	Author    string         `json:"author,omitempty"`
	Signature []byte         `json:"signature,omitempty"`
}

// NewTimeoutMsg constructs a new atomic broadcast timeout message
func NewTimeoutMsg(timeout *abdrc.Timeout, author string) *TimeoutMsg {
	return &TimeoutMsg{Timeout: timeout, Author: author}
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

func (x *TimeoutMsg) Verify(tb types.RootTrustBase) error {
	if _, err := tb.VerifySignature(x.Bytes(), x.Signature, x.Author); err != nil {
		return fmt.Errorf("signature verification failed: %w", err)
	}
	if err := x.Timeout.Verify(tb); err != nil {
		return fmt.Errorf("timeout data verification failed: %w", err)
	}
	return nil
}

func (x *TimeoutMsg) GetRound() uint64 {
	if x == nil || x.Timeout == nil {
		return 0
	}
	return x.Timeout.Round
}
