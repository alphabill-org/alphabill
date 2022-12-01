package atomic_broadcast

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"

	"github.com/libp2p/go-libp2p/core/peer"
)

var (
	ErrTimeoutIsNil          = errors.New("timeout is nil")
	ErrSealNotSignedByQuorum = errors.New("seal not signed by quorum")
	ErrMissingVoteInfo       = errors.New("vote info is nil")
	ErrLedgerCommitInfoIsNil = errors.New("ledger commit info is nil")
)

func NewQuorumCertificate(voteInfo *VoteInfo, commitInfo *LedgerCommitInfo, signatures map[string][]byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       signatures,
	}
}

func (x *QuorumCert) IsValid() error {
	// QC must have valid vote info
	if x.VoteInfo == nil {
		return ErrMissingVoteInfo
	}
	if err := x.VoteInfo.IsValid(); err != nil {
		return err
	}
	// and must have valid ledger commit info
	if x.LedgerCommitInfo == nil {
		return ErrLedgerCommitInfoIsNil
	}
	if err := x.LedgerCommitInfo.IsValid(); err != nil {
		return err
	}
	// Perhaps check for presents of signatures?
	return nil
}

func (x *QuorumCert) Verify(v AtomicVerifier) error {
	if err := x.IsValid(); err != nil {
		return err
	}
	// check vote info hash
	hasher := gocrypto.SHA256.New()
	x.VoteInfo.AddToHasher(hasher)
	if !bytes.Equal(hasher.Sum(nil), x.LedgerCommitInfo.VoteInfoHash) {
		return fmt.Errorf("quorum certificate not valid: vote info hash verification failed")
	}
	hasher.Reset()
	hasher.Write(x.LedgerCommitInfo.Bytes())
	// Check quorum, if not fail without checking signatures itself
	if uint32(len(x.Signatures)) < v.GetQuorumThreshold() {
		return fmt.Errorf("quorum certificate not valid: less than quorum %d/%d have signed",
			len(x.Signatures), v.GetQuorumThreshold())
	}
	// Verify all signatures
	for author, sig := range x.Signatures {
		err := v.VerifySignature(hasher.Sum(nil), sig, peer.ID(author))
		if err != nil {
			return fmt.Errorf("quorum certificate not valid: %w", err)
		}
	}
	return nil
}

func (x *QuorumCert) AddToHasher(hasher hash.Hash) {
	x.VoteInfo.AddToHasher(hasher)
	hasher.Write(x.LedgerCommitInfo.Bytes())
	// Add all signatures
	for author, sig := range x.Signatures {
		hasher.Write([]byte(author))
		hasher.Write(sig)
	}
}
