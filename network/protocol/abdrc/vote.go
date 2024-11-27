package abdrc

import (
	"bytes"
	gocrypto "crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/crypto"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill-go-base/types/hex"
	drctypes "github.com/alphabill-org/alphabill/rootchain/consensus/types"
)

type VoteMsg struct {
	_                struct{}             `cbor:",toarray"`
	VoteInfo         *drctypes.RoundInfo  `json:"voteInfo"`         // Proposed block hash and resulting state hash
	LedgerCommitInfo *types.UnicitySeal   `json:"ledgerCommitInfo"` // Commit info
	HighQc           *drctypes.QuorumCert `json:"highQc"`           // Sync with highest QC
	Author           string               `json:"author"`           // Voter node identifier
	Signature        hex.Bytes            `json:"signature"`        // Vote signature on hash of consensus info
}

func (x *VoteMsg) Sign(signer crypto.Signer) error {
	if signer == nil {
		return errSignerIsNil
	}
	// sanity check, make sure commit info round info hash is set
	if len(x.LedgerCommitInfo.PreviousHash) < 1 {
		return fmt.Errorf("invalid round info hash")
	}
	bs, err := x.LedgerCommitInfo.SigBytes()
	if err != nil {
		return fmt.Errorf("failed to marshal unicity seal: %w", err)
	}
	signature, err := signer.SignBytes(bs)
	if err != nil {
		return fmt.Errorf("failed to sign vote: %w", err)
	}
	x.Signature = signature
	return nil
}

func (x *VoteMsg) Verify(tb types.RootTrustBase) error {
	if x.Author == "" {
		return fmt.Errorf("author is missing")
	}
	if x.VoteInfo == nil {
		return fmt.Errorf("vote from '%s' is missing vote info", x.Author)
	}
	if err := x.VoteInfo.IsValid(); err != nil {
		return fmt.Errorf("vote from '%s' vote info error: %w", x.Author, err)
	}
	if x.LedgerCommitInfo == nil {
		return fmt.Errorf("vote from '%s' ledger commit info (unicity seal) is missing", x.Author)
	}
	// Verify hash of vote info
	hash := x.VoteInfo.Hash(gocrypto.SHA256)
	if !bytes.Equal(hash, x.LedgerCommitInfo.PreviousHash) {
		return fmt.Errorf("vote from '%s' vote info hash does not match hash in commit info", x.Author)
	}
	if x.HighQc == nil {
		return fmt.Errorf("vote from '%s' high QC is nil", x.Author)
	}
	if err := x.HighQc.Verify(tb); err != nil {
		return fmt.Errorf("vote from '%s' high QC error: %w", x.Author, err)
	}
	bs, err := x.LedgerCommitInfo.SigBytes()
	if err != nil {
		return fmt.Errorf("failed to marshal unicity seal: %w", err)
	}
	if _, err := tb.VerifySignature(bs, x.Signature, x.Author); err != nil {
		return fmt.Errorf("vote from '%s' signature verification error: %w", x.Author, err)
	}
	return nil
}
