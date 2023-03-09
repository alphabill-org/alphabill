package certificates

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/internal/crypto"
	"github.com/alphabill-org/alphabill/internal/util"
)

var (
	ErrUnicitySealIsNil         = errors.New("unicity seal is nil")
	ErrSignerIsNil              = errors.New("signer is nil")
	ErrUnicitySealHashIsNil     = errors.New("commit info root hash is not valid")
	ErrRootValidatorInfoMissing = errors.New("root validator info is missing")
	ErrUnknownSigner            = errors.New("unknown signer")
	ErrRoundCreationTimeNotSet  = errors.New("round creation time not set")
	ErrRootInfoInvalidRound     = errors.New("root round info round number is not valid")
	ErrInvalidRootRoundInfoHash = errors.New("root round info latest root hash is not valid")
	ErrInvalidRootInfoHash      = errors.New("commit info root round info hash is invalid")
	ErrRootRoundInfoIsNil       = errors.New("root round info is nil")
	ErrCommitInfoIsNil          = errors.New("commit info is nil")
	ErrCommitInfoRoundInfoHash  = errors.New("invalid commit info, root round info hash is different")
)

func (x *RootRoundInfo) Hash(hash gocrypto.Hash) []byte {
	hasher := hash.New()
	hasher.Write(x.Bytes())
	return hasher.Sum(nil)
}

func (x *RootRoundInfo) Bytes() []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(x.RoundNumber))
	b.Write(util.Uint64ToBytes(x.Epoch))
	b.Write(util.Uint64ToBytes(x.Timestamp))
	b.Write(util.Uint64ToBytes(x.ParentRoundNumber))
	b.Write(x.CurrentRootHash)
	return b.Bytes()

}

func (x *RootRoundInfo) IsValid() error {
	if x.RoundNumber < 1 || x.RoundNumber <= x.ParentRoundNumber {
		return ErrRootInfoInvalidRound
	}
	if len(x.CurrentRootHash) < 1 {
		return ErrInvalidRootRoundInfoHash
	}
	if x.Timestamp == 0 {
		return ErrRoundCreationTimeNotSet
	}
	return nil
}

func (x *CommitInfo) Bytes() []byte {
	var b bytes.Buffer
	b.Write(x.RootRoundInfoHash)
	b.Write(x.RootHash)
	return b.Bytes()
}

func (x *CommitInfo) Hash(hash gocrypto.Hash) []byte {
	hasher := hash.New()
	hasher.Write(x.RootRoundInfoHash)
	hasher.Write(x.RootHash)
	return hasher.Sum(nil)
}

func (x *CommitInfo) IsValid() error {
	if len(x.RootRoundInfoHash) < 1 {
		return ErrInvalidRootInfoHash
	}
	if len(x.RootHash) < 1 {
		return ErrUnicitySealHashIsNil
	}
	return nil
}

func (x *UnicitySeal) IsValid(verifiers map[string]crypto.Verifier) error {
	if x == nil {
		return ErrUnicitySealIsNil
	}
	if len(verifiers) == 0 {
		return ErrRootValidatorInfoMissing
	}
	if x.RootRoundInfo == nil {
		return ErrRootRoundInfoIsNil
	}
	if err := x.RootRoundInfo.IsValid(); err != nil {
		return err
	}
	if x.CommitInfo == nil {
		return ErrCommitInfoIsNil
	}
	if err := x.CommitInfo.IsValid(); err != nil {
		return err
	}
	// verify all signatures
	return x.verify(verifiers)
}

func (x *UnicitySeal) Sign(id string, signer crypto.Signer) error {
	if signer == nil {
		return ErrSignerIsNil
	}
	if err := x.CommitInfo.IsValid(); err != nil {
		return err
	}
	signature, err := signer.SignBytes(x.CommitInfo.Bytes())
	if err != nil {
		return err
	}
	// initiate signatures
	if x.Signatures == nil {
		x.Signatures = make(map[string][]byte)
	}
	x.Signatures[id] = signature
	return nil
}

func (x *UnicitySeal) Bytes() []byte {
	var b bytes.Buffer
	b.Write(util.Uint64ToBytes(x.Version))
	b.Write(x.RootRoundInfo.Bytes())
	b.Write(x.CommitInfo.Bytes())
	return b.Bytes()
}

func (x *UnicitySeal) verify(verifiers map[string]crypto.Verifier) error {
	if verifiers == nil {
		return ErrRootValidatorInfoMissing
	}
	// verify root info hash matches root info hash in commit info
	hash := x.RootRoundInfo.Hash(gocrypto.SHA256)
	if !bytes.Equal(hash, x.CommitInfo.RootRoundInfoHash) {
		return ErrCommitInfoRoundInfoHash
	}
	//! todo: implement trust base, which should also contain quorum info
	quorum := ((len(x.Signatures) * 2) / 3) + 1
	if len(x.Signatures) < quorum {
		return fmt.Errorf("unicity seal is not valid, less than quorum signatures %v/%v",
			quorum, len(x.Signatures))
	}
	// Verify all signatures, all must be from known origin and valid
	for id, sig := range x.Signatures {
		// Find verifier info
		ver, f := verifiers[id]
		if !f {
			return ErrUnknownSigner
		}
		err := ver.VerifyBytes(sig, x.CommitInfo.Bytes())
		if err != nil {
			return fmt.Errorf("unicity seal signature verification failed, %w", err)
		}
	}
	return nil
}
