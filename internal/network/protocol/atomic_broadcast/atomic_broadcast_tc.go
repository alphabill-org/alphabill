package atomic_broadcast

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

// TimeoutSigned since the quorum certificate contains more info than needed. Only round number
// from quorum certificate is used to sign timeout vote.
type TimeoutSigned struct {
	epoch    uint64
	round    uint64
	hqcRound uint64
}

// NewTimeoutSign constructs TimoutStructure to be signed with the VoteMsg
func NewTimeoutSign(epoch uint64, round uint64, hgqRound uint64) *TimeoutSigned {
	return &TimeoutSigned{
		epoch:    epoch,
		round:    round,
		hqcRound: hgqRound,
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
func (x *Timeout) Verify(v AtomicVerifier) error {
	// Make sure that the quorum certificate received with the vote does not have higher round than the round
	// voted timeout
	if x.Hqc.VoteInfo.RootRound > x.Round {
		return errors.New("Malformed timeout, consensus round is bigger that timeout round")
	}
	// Verify attached quorum certificate
	return x.Hqc.Verify(v)
}

func NewPartialTimeoutCertificate(vote *VoteMsg) *TimeoutCert {
	return &TimeoutCert{
		Timeout:    vote.TimeoutSignature.Timeout,
		Signatures: make(map[string]*TimeoutVote),
	}
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
func (x *TimeoutCert) Verify(v AtomicVerifier) error {
	// 1. verify stored quorum certificate is valid and contains quorum of signatures
	err := x.Timeout.Verify(v)
	if err != nil {
		return errors.Wrap(err, "TimeoutCertificate verify failed")
	}
	// 2. Check if there is quorum of signatures for TC
	authors := x.GetAuthors()
	err = v.ValidateQuorum(authors)
	if err != nil {
		return errors.Wrap(err, "TimeoutCertificate verify failed")
	}
	maxSignedRound := uint64(0)
	// Extract attached the highest QC round number and compare it later to the round extracted from signatures
	highQcRound := x.Timeout.Hqc.VoteInfo.RootRound
	// 3. Check all signatures and remember the max QC round over all the signatures received
	for author, timeoutSig := range x.Signatures {
		// verify signature
		timeout := NewTimeoutSign(x.Timeout.GetRound(), x.Timeout.GetEpoch(), timeoutSig.HqcRound)
		err := v.VerifySignature(timeout.Hash(gocrypto.SHA256), timeoutSig.Signature, peer.ID(author))
		if err != nil {
			return errors.Wrap(err, "TimeoutCertificate verify failed, invalid signature")
		}
		if maxSignedRound < timeoutSig.HqcRound {
			maxSignedRound = timeoutSig.HqcRound
		}
	}
	// 4. Verify that the highest quorum certificate stored has max QC round over all timeout votes received
	if highQcRound != maxSignedRound {
		return errors.New("TimeoutCertificate verify failed, QC and max QC round do not match")
	}
	return nil
}
