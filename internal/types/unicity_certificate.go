package types

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/internal/crypto"
)

var ErrUnicityCertificateIsNil = errors.New("unicity certificate is nil")

type UnicityCertificate struct {
	_                      struct{}                `cbor:",toarray"`
	InputRecord            *InputRecord            `json:"input_record,omitempty"`
	UnicityTreeCertificate *UnicityTreeCertificate `json:"unicity_tree_certificate,omitempty"`
	UnicitySeal            *UnicitySeal            `json:"unicity_seal,omitempty"`
}

func (x *UnicityCertificate) IsValid(verifiers map[string]crypto.Verifier, algorithm gocrypto.Hash, systemIdentifier, systemDescriptionHash []byte) error {
	if x == nil {
		return ErrUnicityCertificateIsNil
	}
	if err := x.UnicitySeal.IsValid(verifiers); err != nil {
		return fmt.Errorf("unicity seal validation failed, %w", err)
	}
	if err := x.InputRecord.IsValid(); err != nil {
		return fmt.Errorf("intput record validation failed, %w", err)
	}
	if err := x.UnicityTreeCertificate.IsValid(systemIdentifier, systemDescriptionHash); err != nil {
		return fmt.Errorf("unicity tree certificate validation failed, %w", err)
	}
	hasher := algorithm.New()
	x.InputRecord.AddToHasher(hasher)
	hasher.Write(x.UnicityTreeCertificate.SystemDescriptionHash)
	treeRoot, err := x.UnicityTreeCertificate.GetAuthPath(hasher.Sum(nil), algorithm)
	if err != nil {
		return fmt.Errorf("failed to get authentication path from unicity tree certificate, %w", err)
	}
	rootHash := x.UnicitySeal.Hash
	if !bytes.Equal(treeRoot, rootHash) {
		return fmt.Errorf("unicity seal hash %X does not match with the root hash of the unicity tree %X", rootHash, treeRoot)
	}
	return nil
}

func (x *UnicityCertificate) AddToHasher(hasher hash.Hash) {
	hasher.Write(x.Bytes())
}

func (x *UnicityCertificate) Bytes() []byte {
	var b bytes.Buffer
	if x.InputRecord != nil {
		b.Write(x.InputRecord.Bytes())
	}
	if x.UnicityTreeCertificate != nil {
		b.Write(x.UnicityTreeCertificate.Bytes())
	}
	if x.UnicitySeal != nil {
		b.Write(x.UnicitySeal.Bytes())
	}
	return b.Bytes()
}

func (x *UnicityCertificate) GetRoundNumber() uint64 {
	if x != nil && x.InputRecord != nil {
		return x.InputRecord.RoundNumber
	}
	return 0
}

// CheckNonEquivocatingCertificates checks if provided certificates are equivocating
// NB! order is important, also it is assumed that validity of both UCs is checked before
// The algorithm is based on Yellowpaper: "Algorithm 6 Checking two UC-s for equivocation"
func CheckNonEquivocatingCertificates(prevUC, newUC *UnicityCertificate) error {
	if newUC == nil {
		return errUCIsNil
	}
	if prevUC == nil {
		return errLastUCIsNil
	}
	// verify order, check both partition round and root round
	if newUC.UnicitySeal.RootChainRoundNumber < prevUC.UnicitySeal.RootChainRoundNumber {
		return fmt.Errorf("new certificate is from older root round %v than previous certificate %v",
			newUC.UnicitySeal.RootChainRoundNumber, prevUC.UnicitySeal.RootChainRoundNumber)
	}
	if newUC.InputRecord.RoundNumber < prevUC.InputRecord.RoundNumber {
		return fmt.Errorf("new certificate is from older partition round %v than previous certificate %v",
			newUC.InputRecord.RoundNumber, prevUC.InputRecord.RoundNumber)
	}
	// 1. uc.IR.n = uc′.IR.n - if the partition round number is the same then input records must also match
	if newUC.InputRecord.RoundNumber == prevUC.InputRecord.RoundNumber {
		if !bytes.Equal(newUC.InputRecord.Bytes(), prevUC.InputRecord.Bytes()) {
			return fmt.Errorf("equivocating UC, different input records for same partition round %v", newUC.InputRecord.RoundNumber)
		}
		// ok, these are just duplicates
		return nil
	}
	// 2. if this is a repeat of previous state, then there is nothing more to check they are the same cert only round
	// number is bigger in the newer certificate
	if isRepeat(prevUC, newUC) {
		// new is repeat of previous UC, only round number is bigger, all is fine
		return nil
	}
	// 3. not a repeat UC, then it must extend from previous state if certificates are from consecutive rounds,
	// if it is not from consecutive rounds then it is simply not possible to make any conclusions
	if newUC.InputRecord.RoundNumber == prevUC.InputRecord.RoundNumber+1 &&
		!bytes.Equal(newUC.InputRecord.PreviousHash, prevUC.InputRecord.Hash) {
		return fmt.Errorf("new certificate does not extend previous state hash")
	}
	// 4. uc.IR.h′ = uc.IR.h -> new cert state does not change, the new certificate is for empty block
	if bytes.Equal(newUC.InputRecord.PreviousHash, newUC.InputRecord.Hash) {
		// then block must be empty and thus hash of block is 0H
		if !isZeroHash(newUC.InputRecord.BlockHash) {
			return fmt.Errorf("invalid new certificate, non-empty block, but state hash does not change")
		}
		// done, nothing more to check
		return nil
	}
	// AB-1002: allow state changes without transaction due to housekeeping (state tree pruning, dust removal, etc.)
	//// 5. uc.IR.h' != uc.IR.h - state changes, new UC with new state
	//// a. block hash must not be empty and thus block hash must not be 0h
	//if isZeroHash(newUC.InputRecord.BlockHash) {
	//	return fmt.Errorf("invalid new certificate, block can not be empty if state changes")
	//}
	// b. block hash must not repeat
	if bytes.Equal(newUC.InputRecord.BlockHash, prevUC.InputRecord.BlockHash) {
		return fmt.Errorf("new certificate repeats previous block hash")
	}
	return nil
}

// isRepeat - check if newUC is repeat of previous UC, everything else is the same but round number is bigger
func isRepeat(prevUC, newUC *UnicityCertificate) bool {
	return !isZeroHash(newUC.InputRecord.BlockHash) &&
		bytes.Equal(prevUC.InputRecord.Hash, newUC.InputRecord.Hash) &&
		bytes.Equal(prevUC.InputRecord.PreviousHash, newUC.InputRecord.PreviousHash) &&
		bytes.Equal(prevUC.InputRecord.BlockHash, newUC.InputRecord.BlockHash) &&
		bytes.Equal(prevUC.InputRecord.SummaryValue, newUC.InputRecord.SummaryValue) &&
		prevUC.InputRecord.SumOfEarnedFees == newUC.InputRecord.SumOfEarnedFees &&
		prevUC.InputRecord.RoundNumber < newUC.InputRecord.RoundNumber
}
