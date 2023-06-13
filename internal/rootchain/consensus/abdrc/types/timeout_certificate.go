package types

import (
	"bytes"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/util"
)

type Timeout struct {
	_      struct{}    `cbor:",toarray"`
	Epoch  uint64      `json:"epoch,omitempty"`   // Epoch to establish valid configuration
	Round  uint64      `json:"round,omitempty"`   // Root round number
	HighQc *QuorumCert `json:"high_qc,omitempty"` // Highest quorum certificate of the validator
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
	if x.Round <= x.HighQc.VoteInfo.RoundNumber {
		return fmt.Errorf("timeout info round is smaller or equal to highest qc seen")
	}
	return nil
}

// Verify verifies timeout vote received.
func (x *Timeout) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	// Make sure that the quorum certificate received with the vote does not have higher round than the round
	// voted timeout
	if x.HighQc.VoteInfo.RoundNumber > x.Round {
		return fmt.Errorf("invalid timeout, qc round %v is bigger than timeout round %v", x.HighQc.VoteInfo.RoundNumber, x.Round)
	}
	// Verify attached quorum certificate
	return x.HighQc.Verify(quorum, rootTrust)
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
	// if already added then reject
	if _, found := x.Signatures[author]; found {
		return fmt.Errorf("%s already voted", author)
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

// Verify timeout certificate
func (x *TimeoutCert) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	// 1. verify stored quorum certificate is valid and contains quorum of signatures
	if err := x.Timeout.Verify(quorum, rootTrust); err != nil {
		return fmt.Errorf("timeout validation failed, %w", err)
	}
	// 2. Check if there is quorum of signatures for TC
	if uint32(len(x.Signatures)) < quorum {
		return fmt.Errorf("certificate is has less %d sinatures than required by quorum %d", len(x.Signatures), quorum)
	}
	maxSignedRound := uint64(0)
	// Extract attached the highest QC round number and compare it later to the round extracted from signatures
	highQcRound := x.Timeout.HighQc.VoteInfo.RoundNumber
	// 3. Check all signatures and remember the max QC round over all the signatures received
	for author, timeoutSig := range x.Signatures {
		// verify signature
		timeout := BytesFromTimeoutVote(x.Timeout, author, timeoutSig)
		v, f := rootTrust[author]
		if !f {
			return fmt.Errorf("failed to find public key for author %v", author)
		}
		err := v.VerifyBytes(timeoutSig.Signature, timeout)
		if err != nil {
			return fmt.Errorf("timeout certificate signature verification failed, %w", err)
		}
		if maxSignedRound < timeoutSig.HqcRound {
			maxSignedRound = timeoutSig.HqcRound
		}
	}
	// 4. Verify that the highest quorum certificate stored has max QC round over all timeout votes received
	if highQcRound != maxSignedRound {
		return fmt.Errorf("timeout high qc round %d does not match max signed qc round %d", highQcRound, maxSignedRound)
	}
	return nil
}
