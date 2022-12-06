package atomic_broadcast

import (
	"bytes"
	gocrypto "crypto"
	"fmt"
	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/util"
)

// NewTimeout creates new Timeout for round (epoch) and highest QC seen
func NewTimeout(round, epoch uint64, hqc *QuorumCert) (*Timeout, error) {
	if hqc == nil {
		return nil, fmt.Errorf("invalid timeout, high qc is nil")
	}
	if round <= hqc.VoteInfo.RootRound {
		return nil, fmt.Errorf("invalid timeout round, round is smaller or equal to highest qc seen")
	}
	return &Timeout{Epoch: epoch, Round: round, Hqc: hqc}, nil
}

func (x *VoteMsg) IsTimeout() bool {
	if x.TimeoutSignature == nil {
		return false
	}
	return true
}

func (x *VoteMsg) AddSignature(signer crypto.Signer) error {
	if signer == nil {
		return ErrSignerIsNil
	}
	// make sure ledger commit info is populated
	if err := x.LedgerCommitInfo.IsValid(); err != nil {
		return err
	}
	signature, err := signer.SignBytes(x.LedgerCommitInfo.Bytes())
	if err != nil {
		return err
	}
	x.Signature = signature
	return nil
}

func (x *VoteMsg) Verify(quorum uint32, rootTrust map[string]crypto.Verifier) error {
	if x.VoteInfo == nil {
		return fmt.Errorf("invalid vote message, vote info is nil")
	}
	if err := x.VoteInfo.IsValid(); err != nil {
		return fmt.Errorf("invalid vote message, %w", err)
	}
	// Verify hash of vote info
	hash := x.VoteInfo.Hash(gocrypto.SHA256)
	if !bytes.Equal(hash, x.LedgerCommitInfo.VoteInfoHash) {
		return errors.New("invalid vote message, vote info hash verification failed")
	}
	if len(x.Author) == 0 {
		return errors.New("invalid vote message, no author")
	}
	if err := x.HighCommitQc.Verify(quorum, rootTrust); err != nil {
		return fmt.Errorf("invalid vote message, %w", err)
	}
	// verify signature
	v, f := rootTrust[x.Author]
	if !f {
		return fmt.Errorf("failed to find public key for author %v", x.Author)
	}
	if err := v.VerifyBytes(x.Signature, x.LedgerCommitInfo.Bytes()); err != nil {
		return errors.Wrap(err, "invalid vote message, signature verification failed")
	}
	if x.IsTimeout() {
		err := v.VerifyBytes(x.TimeoutSignature.Signature, x.TimeoutSignature.Timeout.Bytes())
		if err != nil {
			return errors.Wrap(err, "timeout vote signature")
		}
	}
	return nil
}

// AddTimeoutSignature adds timeout signature to vote message, turning it into a timeout vote
func (x *VoteMsg) AddTimeoutSignature(timeout *Timeout, signature []byte) error {
	if timeout == nil {
		return ErrTimeoutIsNil
	}
	if len(signature) < 1 {
		return fmt.Errorf("error timeout is not signed")
	}
	x.TimeoutSignature = &TimeoutWithSignature{Timeout: timeout, Signature: signature}
	return nil
}

func (x *Timeout) Bytes() []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(x.Epoch))
	b.Write(util.Uint64ToBytes(x.Round))
	return b.Bytes()
}
