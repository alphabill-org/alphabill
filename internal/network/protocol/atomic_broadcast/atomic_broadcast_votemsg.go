package atomic_broadcast

import (
	"bytes"
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/util"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/libp2p/go-libp2p/core/peer"
)

func (x *VoteMsg) IsTimeout() bool {
	if x.TimeoutSignature == nil {
		return false
	}
	return true
}

func (x *VoteMsg) AddSignature(signer crypto.Signer) error {
	if signer == nil {
		return errors.New(ErrSignerIsNil)
	}
	signature, err := signer.SignBytes(x.LedgerCommitInfo.Bytes())
	if err != nil {
		return err
	}
	x.Signature = signature
	return nil
}

func (x *VoteMsg) Verify(v AtomicVerifier) error {
	if v == nil {
		return errors.New(ErrVerifierIsNil)
	}
	if err := x.VoteInfo.IsValid(); err != nil {
		return errors.Wrap(err, "invalid vote message")
	}
	// Verify hash of vote info
	hasher := gocrypto.SHA256.New()
	x.VoteInfo.AddToHasher(hasher)
	if !bytes.Equal(hasher.Sum(nil), x.LedgerCommitInfo.VoteInfoHash) {
		return errors.New("vote info hash verification failed")
	}
	if len(x.Author) == 0 {
		return errors.New("Vote message is missing author")
	}
	// verify signature
	if err := v.VerifyBytes(x.LedgerCommitInfo.Bytes(), x.Signature, peer.ID(x.Author)); err != nil {
		return errors.Wrap(err, "Vote message signature verification failed")
	}
	return nil
}

// NewTimeout creates new Timeout for timeout vote
func (x *VoteMsg) NewTimeout(qc *QuorumCert) *Timeout {
	return &Timeout{Epoch: x.VoteInfo.Epoch, Round: x.VoteInfo.RootRound, Hqc: qc}
}

// AddTimeoutSignature adds timeout signature to vote message, turning it into a timeout vote
func (x *VoteMsg) AddTimeoutSignature(timeout *Timeout, signature []byte) error {
	if timeout == nil {
		return errors.New(ErrTimeoutIsNil)
	}
	x.TimeoutSignature = &TimeoutWithSignature{Timeout: timeout, Signature: signature}
	return nil
}

func (x *VoteMsg) VerifyTimeoutSignature(timeout *Timeout, v crypto.Verifier) error {
	if v == nil {
		return errors.New(ErrVerifierIsNil)
	}
	if timeout == nil {
		return errors.New(ErrTimeoutIsNil)
	}
	err := v.VerifyBytes(x.TimeoutSignature.Signature, timeout.Bytes())
	if err != nil {
		return errors.Wrap(err, "invalid unicity seal signature")
	}
	return err
}

func (x *Timeout) Bytes() []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(x.Epoch))
	b.Write(util.Uint64ToBytes(x.Round))
	return b.Bytes()
}