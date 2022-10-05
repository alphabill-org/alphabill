package rootvalidator

import (
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/network/protocol/atomic_broadcast"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/libp2p/go-libp2p/core/peer"
)

type (
	// TimeoutSign since the quorum certificate contains more info than needed. Only round number
	// from quorum certificate is used to sign timeout vote.
	TimeoutSign struct {
		epoch    uint64
		round    uint64
		hqcRound uint64
	}
	// Timeout structure
	Timeout struct {
		// epoch number (active validator set)
		Epoch uint64
		// root round number
		Round uint64
		// highest QC the validator has seen
		Hqc *atomic_broadcast.QuorumCert
	}
	// TimeoutTuple keeps both highest QC round and signature for all timeout votes received
	TimeoutTuple struct {
		// remember qc round received with a vote (needed to check signature later)
		qcRound uint64
		// timeout vote signature (timeout vote signature = sign(hash(TimeoutSign{epoch, round, hqcRound}))
		signature []byte
	}
	// TimeoutCertificate proves that 2f+1 validators have signed the certificate for round r, thus it is ok
	// to time out this round and start round r+1
	TimeoutCertificate struct {
		timeout    *Timeout
		signatures map[string]TimeoutTuple
	}
)

// newTimeoutSign constructs TimoutStructure to be signed with the VoteMsg
func newTimeoutSign(epoch uint64, round uint64, hgqRound uint64) *TimeoutSign {
	return &TimeoutSign{
		epoch:    epoch,
		round:    round,
		hqcRound: hgqRound,
	}
}

// Hash computes hash of the TimeoutSign structure that is gets signed and added as timeout vote
func (t *TimeoutSign) Hash(algo gocrypto.Hash) []byte {
	hasher := algo.New()
	hasher.Write(util.Uint64ToBytes(t.epoch))
	hasher.Write(util.Uint64ToBytes(t.round))
	hasher.Write(util.Uint64ToBytes(t.hqcRound))
	return hasher.Sum(nil)
}

// newTimeout constructs new local timeout store for timeout vote msg
func newTimeout(signedTimeout *atomic_broadcast.TimeoutWithSignature) *Timeout {
	return &Timeout{
		Epoch: signedTimeout.Timeout.Epoch,
		Round: signedTimeout.Timeout.Round,
		Hqc:   signedTimeout.Timeout.Hqc,
	}
}

func (t *Timeout) GetQcRound() uint64 {
	return t.Hqc.VoteInfo.Proposed.Round
}

// Verify verifies timeout vote received.
func (t *Timeout) Verify(v RootVerifier) error {
	// Make sure that the quorum certificate received with the vote does not have higher round than the round
	// voted timeout
	if t.Hqc.VoteInfo.Proposed.Round > t.Round {
		return errors.New("Malformed timeout, consensus round is bigger that timeout round")
	}
	// Verify attached quorum certificate
	return t.Hqc.Verify(v)
}

// NewTimeoutCertificate constructs partial timeout certificate from first timeout vote received
func NewTimeoutCertificate(voteMsg *atomic_broadcast.VoteMsg) *TimeoutCertificate {
	return &TimeoutCertificate{
		timeout:    newTimeout(voteMsg.TimeoutSignature),
		signatures: make(map[string]TimeoutTuple),
	}
}

func (x *TimeoutCertificate) GetRound() uint64 {
	return x.timeout.Round
}

func (x *TimeoutCertificate) GetEpoch() uint64 {
	return x.timeout.Epoch
}

func (x *TimeoutCertificate) AddSignature(author string, signedTimeout *atomic_broadcast.TimeoutWithSignature) {
	// Todo: Make sure that both epoch and round are from current epoch and round
	// Keep the highest QC certificate
	hqcRound := signedTimeout.Timeout.Hqc.VoteInfo.Proposed.Round
	// If received highest QC round was bigger than replace timeout struct
	if hqcRound > x.timeout.Hqc.VoteInfo.Proposed.Round {
		x.timeout = newTimeout(signedTimeout)
	}
	x.signatures[author] = TimeoutTuple{hqcRound, signedTimeout.Signature}
}

func (x *TimeoutCertificate) RemoveSignature(author string) {
	delete(x.signatures, author)
}

func (x *TimeoutCertificate) GetSignatures() *map[string]TimeoutTuple {
	return &x.signatures
}

func (x *TimeoutCertificate) signData() *TimeoutSign {
	return &TimeoutSign{}
}

func (x *TimeoutCertificate) getAuthors() []string {
	authors := make([]string, 0, len(x.signatures))
	for k := range x.signatures {
		authors = append(authors, k)
	}
	return authors
}

// Verify timeout certificate
func (x *TimeoutCertificate) Verify(v RootVerifier) error {
	// 1. verify stored quorum certificate is valid and contains quorum of signatures
	err := x.timeout.Verify(v)
	if err != nil {
		return errors.Wrap(err, "TimeoutCertificate verify failed")
	}
	// 2. Check if there is quorum of signatures for TC
	authors := x.getAuthors()
	err = v.ValidateQuorum(authors)
	if err != nil {
		return errors.Wrap(err, "TimeoutCertificate verify failed")
	}
	maxSignedRound := uint64(0)
	highQcRound := x.timeout.Hqc.VoteInfo.Proposed.Round
	// 3. Check all signatures and remember the max QC round over all the signatures received
	for author, timeoutSig := range x.signatures {
		// verify signature
		timeout := newTimeoutSign(x.GetRound(), x.GetEpoch(), timeoutSig.qcRound)
		err := v.VerifySignature(timeout.Hash(gocrypto.SHA256), timeoutSig.signature, peer.ID(author))
		if err != nil {
			return errors.Wrap(err, "TimeoutCertificate verify failed, invalid signature")
		}
		if maxSignedRound < timeoutSig.qcRound {
			maxSignedRound = timeoutSig.qcRound
		}
	}
	// 4. Verify that the highest quorum certificate stored has max QC round over all timeout votes received
	if highQcRound != maxSignedRound {
		return errors.New("TimeoutCertificate verify failed, QC and max QC round do not match")
	}
	return nil
}
