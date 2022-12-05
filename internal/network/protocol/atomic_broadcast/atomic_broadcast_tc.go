package atomic_broadcast

import (
	gocrypto "crypto"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/crypto"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/util"
)

// TimeoutSigned since the quorum certificate contains more info than needed. Only round number
// from quorum certificate is used to sign timeout vote.
type TimeoutSigned struct {
	epoch    uint64
	round    uint64
	hqcRound uint64
}

// NewTimeoutSign constructs TimoutStructure to be signed with the VoteMsg
func NewTimeoutSign(epoch uint64, round uint64, hqcRound uint64) *TimeoutSigned {
	return &TimeoutSigned{
		epoch:    epoch,
		round:    round,
		hqcRound: hqcRound,
	}
}

// Hash computes hash of the TimeoutSign structure that is gets signed and added as timeout vote
func (t *TimeoutSigned) Hash(algo gocrypto.Hash) []byte {
	hasher := algo.New()
	hasher.Write(util.Uint64ToBytes(t.epoch))
	hasher.Write(util.Uint64ToBytes(t.round))
	hasher.Write(util.Uint64ToBytes(t.hqcRound))
	return hasher.Sum(nil)
}

// Verify verifies timeout vote received.
func (x *Timeout) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	// Make sure that the quorum certificate received with the vote does not have higher round than the round
	// voted timeout
	if x.Hqc.VoteInfo.RootRound > x.Round {
		return fmt.Errorf("invalid timeout, qc round %v is bigger than timeout round %v", x.Hqc.VoteInfo.RootRound, x.Round)
	}
	// Verify attached quorum certificate
	return x.Hqc.Verify(quorum, rootTrust)
}

func (x *TimeoutCert) GetAuthors() []string {
	authors := make([]string, 0, len(x.Signatures))
	for k := range x.Signatures {
		authors = append(authors, k)
	}
	return authors
}

func (x *TimeoutCert) AddSignature(author string, signedTimeout *TimeoutWithSignature) {
	// Todo: Make sure that both epoch and round are from current epoch and round
	// Keep the highest QC certificate
	hqcRound := signedTimeout.Timeout.Hqc.VoteInfo.RootRound
	// If received highest QC round was bigger than previously seen, replace timeout struct
	if hqcRound > x.Timeout.Hqc.VoteInfo.RootRound {
		x.Timeout = signedTimeout.Timeout
	}
	x.Signatures[author] = &TimeoutVote{HqcRound: hqcRound, Signature: signedTimeout.Signature}
}

// Verify timeout certificate
func (x *TimeoutCert) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	// 1. verify stored quorum certificate is valid and contains quorum of signatures
	err := x.Timeout.Verify(quorum, rootTrust)
	if err != nil {
		return errors.Wrap(err, "timeout certificate not valid")
	}
	// 2. Check if there is quorum of signatures for TC
	if uint32(len(x.Signatures)) < quorum {
		return fmt.Errorf("timeout certificate not valid: less than quorum %v/%v have signed certificate", len(x.Signatures), quorum)
	}
	maxSignedRound := uint64(0)
	// Extract attached the highest QC round number and compare it later to the round extracted from signatures
	highQcRound := x.Timeout.Hqc.VoteInfo.RootRound
	// 3. Check all signatures and remember the max QC round over all the signatures received
	for author, timeoutSig := range x.Signatures {
		// verify signature
		timeout := NewTimeoutSign(x.Timeout.GetRound(), x.Timeout.GetEpoch(), timeoutSig.HqcRound)
		v, f := rootTrust[author]
		if !f {
			return fmt.Errorf("failed to find public key for author %v", author)
		}
		err := v.VerifyHash(timeoutSig.Signature, timeout.Hash(gocrypto.SHA256))
		if err != nil {
			return errors.Wrap(err, "timeout certificate not valid: invalid signature")
		}
		if maxSignedRound < timeoutSig.HqcRound {
			maxSignedRound = timeoutSig.HqcRound
		}
	}
	// 4. Verify that the highest quorum certificate stored has max QC round over all timeout votes received
	if highQcRound != maxSignedRound {
		return errors.New("timeout certificate not valid: QC and max QC round do not match")
	}
	return nil
}
