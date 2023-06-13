package abdrc

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
	abtypes "github.com/alphabill-org/alphabill/internal/rootchain/consensus/abdrc/types"
	"github.com/alphabill-org/alphabill/internal/types"
)

type VoteMsg struct {
	_                struct{}            `cbor:",toarray"`
	VoteInfo         *abtypes.RoundInfo  `json:"vote_info,omitempty"`          // Proposed block hash and resulting state hash
	LedgerCommitInfo *types.UnicitySeal  `json:"ledger_commit_info,omitempty"` // Commit info
	HighQc           *abtypes.QuorumCert `json:"high_qc,omitempty"`            // Sync with highest QC
	Author           string              `json:"author,omitempty"`             // Voter node identifier
	Signature        []byte              `json:"signature,omitempty"`          // Vote signature on hash of consensus info
}

func (x *VoteMsg) Sign(signer crypto.Signer) error {
	if signer == nil {
		return errSignerIsNil
	}
	// sanity check, make sure commit info round info hash is set
	if len(x.LedgerCommitInfo.RootInternalInfo) < 1 {
		return fmt.Errorf("invalid round info hash")
	}
	signature, err := signer.SignBytes(x.LedgerCommitInfo.Bytes())
	if err != nil {
		return fmt.Errorf("vote sign failed, %w", err)
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
	if !bytes.Equal(hash, x.LedgerCommitInfo.RootInternalInfo) {
		return fmt.Errorf("vote info hash does not match hash in commit info")
	}
	if len(x.Author) == 0 {
		return fmt.Errorf("invalid vote message, no author")
	}
	if err := x.HighQc.Verify(quorum, rootTrust); err != nil {
		return fmt.Errorf("invalid vote message, %w", err)
	}
	// verify signature
	v, f := rootTrust[x.Author]
	if !f {
		return fmt.Errorf("failed to find public key for author %v", x.Author)
	}
	if err := v.VerifyBytes(x.Signature, x.LedgerCommitInfo.Bytes()); err != nil {
		return fmt.Errorf("message signature verification failed, %w", err)
	}
	return nil
}
