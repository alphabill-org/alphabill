package atomic_broadcast

import (
	"bytes"
	gocrypto "crypto"

	"github.com/alphabill-org/alphabill/internal/errors"
	"github.com/alphabill-org/alphabill/internal/rootvalidator"
)

const (
	ErrTimeoutIsNil          = "timeout is nil"
	ErrSealNotSignedByQuorum = "seal not signed by quorum"
)

func NewQuorumCertificate(voteInfo *VoteInfo, commitInfo *CommitInfo, signatures map[string][]byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:   voteInfo,
		CommitInfo: commitInfo,
		Signatures: signatures,
	}
}

func NewTimeoutCertificate() {

}

func (x *QuorumCert) Verify(v rootvalidator.RootVerifier) error {
	// verify that QC is valid
	if err := x.VoteInfo.IsValid(); err != nil {
		return err
	}
	// Check Consensus info
	if err := x.CommitInfo.IsValid(); err != nil {
		return err
	}
	// check vote info hash
	hasher := gocrypto.SHA256.New()
	x.VoteInfo.AddToHasher(hasher)
	if !bytes.Equal(hasher.Sum(nil), x.CommitInfo.VoteInfoHash) {
		return errors.New("vote info hash verification failed")
	}
	// verify that it is signed by quorum
	// if less than quorum, then there is no need to verify the signatures
	if uint32(len(x.Signatures)) < v.GetQuorumThreshold() {
		return errors.New(ErrSealNotSignedByQuorum)
	}
	hasher.Reset()
	hasher.Write(x.CommitInfo.Bytes())
	err := v.VerifyQuorumSignatures(hasher.Sum(nil), x.Signatures)
	if err != nil {
		errors.Wrap(err, "QC verify failed")
	}
	return nil
}
