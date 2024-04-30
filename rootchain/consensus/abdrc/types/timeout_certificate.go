package types

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill-go-sdk/crypto"
	"github.com/alphabill-org/alphabill-go-sdk/util"
)

type Timeout struct {
	_      struct{}     `cbor:",toarray"`
	Epoch  uint64       `json:"epoch,omitempty"`   // Epoch to establish valid configuration
	Round  uint64       `json:"round,omitempty"`   // Root round number
	HighQc *QuorumCert  `json:"high_qc,omitempty"` // Highest quorum certificate of the validator
	LastTC *TimeoutCert `json:"last_tc,omitempty"` // TC for Round−1 if HighQC.Round != Round−1 (nil otherwise)
}

type TimeoutVote struct {
	_         struct{} `cbor:",toarray"`
	HqcRound  uint64   `json:"hqc_round,omitempty"` // round from timeout.high_qc.voteInfo.round
	Signature []byte   `json:"signature,omitempty"` // timeout signature is TimeoutMsg signature - round, epoch, hqc_round, author
}

type TimeoutCert struct {
	_          struct{}                `cbor:",toarray"`
	Timeout    *Timeout                `json:"timeout,omitempty"`    // Round and epoch of the timeout event
	Signatures map[string]*TimeoutVote `json:"signatures,omitempty"` // 2f+1 signatures from nodes confirming TC
}

// NewTimeout creates new Timeout for round (epoch) and highest QC seen
func NewTimeout(round, epoch uint64, hqc *QuorumCert, lastTC *TimeoutCert) *Timeout {
	return &Timeout{Epoch: epoch, Round: round, HighQc: hqc, LastTC: lastTC}
}

func (x *Timeout) IsValid() error {
	if x.HighQc == nil {
		return fmt.Errorf("high QC is unassigned")
	}
	if err := x.HighQc.IsValid(); err != nil {
		return fmt.Errorf("invalid high QC: %w", err)
	}

	if x.Round <= x.HighQc.VoteInfo.RoundNumber {
		return fmt.Errorf("timeout round (%d) must be greater than high QC round (%d)", x.Round, x.HighQc.VoteInfo.RoundNumber)
	}

	// if highQC is not for previous round we must have TC for previous round
	if prevRound := x.Round - 1; prevRound != x.HighQc.GetRound() {
		if x.LastTC == nil {
			return fmt.Errorf("last TC is missing")
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

// Verify verifies timeout vote received.
func (x *Timeout) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("invalid timeout data: %w", err)
	}

	if err := x.HighQc.Verify(quorum, rootTrust); err != nil {
		return fmt.Errorf("invalid high QC: %w", err)
	}

	if x.LastTC != nil {
		if err := x.LastTC.Verify(quorum, rootTrust); err != nil {
			return fmt.Errorf("invalid last TC: %w", err)
		}
	}

	return nil
}

func (x *Timeout) GetRound() uint64 {
	if x != nil {
		return x.Round
	}
	return 0
}

func (x *Timeout) GetHqcRound() uint64 {
	if x != nil {
		return x.HighQc.GetRound()
	}
	return 0
}

func (x *TimeoutCert) GetRound() uint64 {
	if x != nil {
		return x.Timeout.GetRound()
	}
	return 0
}

func (x *TimeoutCert) GetHqcRound() uint64 {
	if x != nil {
		return x.Timeout.GetHqcRound()
	}
	return 0
}

func (x *TimeoutCert) GetAuthors() []string {
	authors := make([]string, 0, len(x.Signatures))
	for k := range x.Signatures {
		authors = append(authors, k)
	}
	return authors
}

func (x *TimeoutCert) Add(author string, timeout *Timeout, signature []byte) error {
	if x.Timeout.Round != timeout.Round {
		return fmt.Errorf("TC is for round %d not %d", x.Timeout.Round, timeout.Round)
	}
	// if already added then reject
	if _, found := x.Signatures[author]; found {
		return fmt.Errorf("%s already voted in round %d", author, x.Timeout.Round)
	}

	// Keep the highest QC certificate
	hqcRound := timeout.HighQc.VoteInfo.RoundNumber
	// If received highest QC round was bigger than previously seen, replace timeout struct
	if hqcRound > x.Timeout.HighQc.VoteInfo.RoundNumber {
		x.Timeout = timeout
	}
	x.Signatures[author] = &TimeoutVote{HqcRound: hqcRound, Signature: signature}
	return nil
}

func BytesFromTimeoutVote(t *Timeout, author string, vote *TimeoutVote) []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(t.Round))
	b.Write(util.Uint64ToBytes(t.Epoch))
	b.Write(util.Uint64ToBytes(vote.HqcRound))
	b.Write([]byte(author))
	return b.Bytes()
}

func (x *TimeoutCert) IsValid() error {
	if x.Timeout == nil {
		return fmt.Errorf("timeout data is unassigned")
	}
	return nil
}

func (x *TimeoutCert) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("invalid certificate: %w", err)
	}

	if err := x.Timeout.Verify(quorum, rootTrust); err != nil {
		return fmt.Errorf("invalid timeout data: %w", err)
	}

	if uint32(len(x.Signatures)) < quorum {
		return fmt.Errorf("quorum requires %d signatures but certificate has %d", quorum, len(x.Signatures))
	}

	maxSignedRound := uint64(0)
	highQcRound := x.Timeout.HighQc.VoteInfo.RoundNumber
	// Check all signatures and remember the max QC round over all the signatures received
	for author, timeoutSig := range x.Signatures {
		v, f := rootTrust[author]
		if !f {
			return fmt.Errorf("signer %q is not part of trustbase", author)
		}
		timeout := BytesFromTimeoutVote(x.Timeout, author, timeoutSig)
		if err := v.VerifyBytes(timeoutSig.Signature, timeout); err != nil {
			return fmt.Errorf("timeout certificate signature verification failed: %w", err)
		}
		if maxSignedRound < timeoutSig.HqcRound {
			maxSignedRound = timeoutSig.HqcRound
		}
	}
	// Verify that the highest quorum certificate stored has max QC round over all timeout votes received
	if highQcRound != maxSignedRound {
		return fmt.Errorf("high QC round %d does not match max signed QC round %d", highQcRound, maxSignedRound)
	}
	return nil
}
