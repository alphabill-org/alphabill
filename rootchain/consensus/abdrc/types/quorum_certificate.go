package types

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"sort"

	"github.com/alphabill-org/alphabill-go-base/types"
)

var (
	errVoteInfoIsNil         = errors.New("vote info is nil")
	errLedgerCommitInfoIsNil = errors.New("ledger commit info is nil")
	errInvalidRoundInfoHash  = errors.New("round info has is missing")
)

type QuorumCert struct {
	_                struct{}           `cbor:",toarray"`
	VoteInfo         *RoundInfo         `json:"vote_info,omitempty"`          // Consensus data
	LedgerCommitInfo *types.UnicitySeal `json:"ledger_commit_info,omitempty"` // Commit info
	Signatures       map[string][]byte  `json:"signatures,omitempty"`         // Node identifier to signature map (NB! aggregated signature schema in spec)
}

func NewQuorumCertificateFromVote(voteInfo *RoundInfo, commitInfo *types.UnicitySeal, signatures map[string][]byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: commitInfo,
		Signatures:       signatures,
	}
}

func NewQuorumCertificate(voteInfo *RoundInfo, commitHash []byte) *QuorumCert {
	return &QuorumCert{
		VoteInfo:         voteInfo,
		LedgerCommitInfo: &types.UnicitySeal{PreviousHash: voteInfo.Hash(gocrypto.SHA256), Hash: commitHash},
		Signatures:       map[string][]byte{},
	}
}

func (x *QuorumCert) GetRound() uint64 {
	if x != nil {
		return x.VoteInfo.GetRound()
	}
	return 0
}

func (x *QuorumCert) GetParentRound() uint64 {
	if x != nil {
		return x.VoteInfo.GetParentRound()
	}
	return 0
}

func (x *QuorumCert) GetCommitRound() uint64 {
	if x.LedgerCommitInfo != nil && x.LedgerCommitInfo.Hash != nil {
		return x.LedgerCommitInfo.RootChainRoundNumber
	}
	return 0
}

func (x *QuorumCert) IsValid() error {
	if x.VoteInfo == nil {
		return errVoteInfoIsNil
	}
	if err := x.VoteInfo.IsValid(); err != nil {
		return fmt.Errorf("invalid vote info: %w", err)
	}

	// and must have valid ledger commit info
	if x.LedgerCommitInfo == nil {
		return errLedgerCommitInfoIsNil
	}
	// todo: should call x.LedgerCommitInfo.IsValid but that requires some refactoring
	// not to require trustbase parameter?
	// PreviousHash must not be empty, it always contains vote info hash (name is misleading)
	if len(x.LedgerCommitInfo.PreviousHash) < 1 {
		return errInvalidRoundInfoHash
	}

	return nil
}

func (x *QuorumCert) Verify(tb types.RootTrustBase) error {
	if err := x.IsValid(); err != nil {
		return fmt.Errorf("invalid quorum certificate: %w", err)
	}
	// check vote info hash
	if !bytes.Equal(x.VoteInfo.Hash(gocrypto.SHA256), x.LedgerCommitInfo.PreviousHash) {
		return fmt.Errorf("vote info hash verification failed")
	}
	/* todo: call LedgerCommitInfo.Verify but first refactor it so that it takes quorum param?
	if err := x.LedgerCommitInfo.Verify(rootTrust); err != nil {
		return fmt.Errorf("invalid commit info: %w", err)
	}*/

	if err, _ := tb.VerifyQuorumSignatures(x.LedgerCommitInfo.Bytes(), x.Signatures); err != nil {
		return fmt.Errorf("failed to verify quorum signatures: %w", err)
	}
	return nil
}

// SignatureBytes serializes signatures.
func (x *QuorumCert) SignatureBytes() []byte {
	var b bytes.Buffer
	if x != nil {
		// From QC signers (in the alphabetical order of signer ID!) must be included
		signatures := x.Signatures
		authors := make([]string, 0, len(signatures))
		for k := range signatures {
			authors = append(authors, k)
		}
		sort.Strings(authors)
		for _, author := range authors {
			b.Write(signatures[author])
		}
	}
	return b.Bytes()
}
