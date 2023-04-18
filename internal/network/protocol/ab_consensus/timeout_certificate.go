package ab_consensus

import (
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
)

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

func (x *TimeoutCert) Add(author string, timeout *Timeout, signature []byte) {
	// Keep the highest QC certificate
	hqcRound := timeout.HighQc.VoteInfo.RoundNumber
	// If received highest QC round was bigger than previously seen, replace timeout struct
	if hqcRound > x.Timeout.HighQc.VoteInfo.RoundNumber {
		x.Timeout = timeout
	}
	x.Signatures[author] = &TimeoutVote{HqcRound: hqcRound, Signature: signature}
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
