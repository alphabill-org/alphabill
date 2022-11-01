package atomic_broadcast

import (
	"bytes"
	gocrypto "crypto"
	"hash"

	"github.com/alphabill-org/alphabill/internal/errors"
)

const (
	ErrTimeoutIsNil          = "timeout is nil"
	ErrSealNotSignedByQuorum = "seal not signed by quorum"
)

func NewQuorumCertificate(voteInfo *VoteInfo, commitInfo *LedgerCommitInfo, signatures map[string][]byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       signatures,
	}
}

func (x *QuorumCert) Verify(v AtomicVerifier) error {
	// verify that QC is valid
	if err := x.VoteInfo.IsValid(); err != nil {
		return err
	}
	// Check Consensus info
	if err := x.LedgerCommitInfo.IsValid(); err != nil {
		return err
	}
	// check vote info hash
	hasher := gocrypto.SHA256.New()
	x.VoteInfo.AddToHasher(hasher)
	if !bytes.Equal(hasher.Sum(nil), x.LedgerCommitInfo.VoteInfoHash) {
		return errors.New("vote info hash verification failed")
	}
	// verify that it is signed by quorum
	// if less than quorum, then there is no need to verify the signatures
	if uint32(len(x.Signatures)) < v.GetQuorumThreshold() {
		return errors.New(ErrSealNotSignedByQuorum)
	}
	hasher.Reset()
	hasher.Write(x.LedgerCommitInfo.Bytes())
	err := v.VerifyQuorumSignatures(hasher.Sum(nil), x.Signatures)
	if err != nil {
		errors.Wrap(err, "QC verify failed")
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
