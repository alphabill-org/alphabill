package types

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/crypto"
)

var ErrUnicityCertificateIsNil = errors.New("unicity certificate is nil")

type UnicityCertificate struct {
	_                      struct{}                `cbor:",toarray"`
	InputRecord            *InputRecord            `json:"input_record,omitempty"`
	UnicityTreeCertificate *UnicityTreeCertificate `json:"unicity_tree_certificate,omitempty"`
	UnicitySeal            *UnicitySeal            `json:"unicity_seal,omitempty"`
}

func (x *UnicityCertificate) IsValid(verifiers map[string]crypto.Verifier, algorithm gocrypto.Hash, systemIdentifier SystemID, systemDescriptionHash []byte) error {
	if x == nil {
		return ErrUnicityCertificateIsNil
	}
	if err := x.UnicitySeal.IsValid(verifiers); err != nil {
		return fmt.Errorf("unicity seal validation failed, %w", err)
	}
	if err := x.InputRecord.IsValid(); err != nil {
		return fmt.Errorf("intput record validation failed, %w", err)
	}
	if err := x.UnicityTreeCertificate.IsValid(x.InputRecord, systemIdentifier, systemDescriptionHash, algorithm); err != nil {
		return fmt.Errorf("unicity tree certificate validation failed, %w", err)
	}
	treeRoot := x.UnicityTreeCertificate.EvalAuthPath(algorithm)
	rootHash := x.UnicitySeal.Hash
	if !bytes.Equal(treeRoot, rootHash) {
		return fmt.Errorf("unicity seal hash %X does not match with the root hash of the unicity tree %X", rootHash, treeRoot)
	}
	return nil
}

func (x *UnicityCertificate) Hash(hash gocrypto.Hash) []byte {
	hasher := hash.New()
	if x.InputRecord != nil {
		x.InputRecord.AddToHasher(hasher)
	}
	if x.UnicityTreeCertificate != nil {
		x.UnicityTreeCertificate.AddToHasher(hasher)
	}
	if x.UnicitySeal != nil {
		x.UnicitySeal.AddToHasher(hasher)
	}
	return hasher.Sum(nil)
}

func (x *UnicityCertificate) GetStateHash() []byte {
	if x != nil && x.InputRecord != nil {
		return x.InputRecord.Hash
	}
	return nil
}

func (x *UnicityCertificate) GetPreviousStateHash() []byte {
	if x != nil && x.InputRecord != nil {
		return x.InputRecord.PreviousHash
	}
	return nil
}

func (x *UnicityCertificate) GetRoundNumber() uint64 {
	if x != nil && x.InputRecord != nil {
		return x.InputRecord.RoundNumber
	}
	return 0
}

func (x *UnicityCertificate) GetRootRoundNumber() uint64 {
	if x != nil && x.UnicitySeal != nil {
		return x.UnicitySeal.RootChainRoundNumber
	}
	return 0
}

func (x *UnicityCertificate) GetFeeSum() uint64 {
	if x != nil && x.InputRecord != nil {
		return x.InputRecord.SumOfEarnedFees
	}
	return 0
}

func (x *UnicityCertificate) GetSummaryValue() []byte {
	if x != nil && x.InputRecord != nil {
		return x.InputRecord.SummaryValue
	}
	return nil
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
	if newUC.GetRootRoundNumber() < prevUC.GetRootRoundNumber() {
		return fmt.Errorf("new certificate is from older root round %v than previous certificate %v",
			newUC.UnicitySeal.RootChainRoundNumber, prevUC.UnicitySeal.RootChainRoundNumber)
	}
	if newUC.GetRoundNumber() < prevUC.GetRoundNumber() {
		return fmt.Errorf("new certificate is from older partition round %v than previous certificate %v",
			newUC.InputRecord.RoundNumber, prevUC.InputRecord.RoundNumber)
	}
	// 1. uc.IR.n = uc′.IR.n - if the partition round number is the same then input records must also match
	if newUC.GetRoundNumber() == prevUC.GetRoundNumber() {
		if !bytes.Equal(newUC.InputRecord.Bytes(), prevUC.InputRecord.Bytes()) {
			return fmt.Errorf("equivocating UC, different input records for same partition round %v", newUC.GetRoundNumber())
		}
		// it's a Repeat UC
		return nil
	}
	// 2. not a repeat UC, then it must extend from previous state if certificates are from consecutive rounds,
	// if it is not from consecutive rounds then it is simply not possible to make any conclusions
	if newUC.GetRoundNumber() == prevUC.GetRoundNumber()+1 &&
		!bytes.Equal(newUC.InputRecord.PreviousHash, prevUC.InputRecord.Hash) {
		return fmt.Errorf("new certificate does not extend previous state hash")
	}
	// bridge 0H blocks
	// 5. uc.IR.h′ = uc.IR.h and uc.IR.h = uc.IR.h' -> extends last known state and new state does not change,
	// then new block must be empty
	if bytes.Equal(newUC.InputRecord.PreviousHash, prevUC.InputRecord.Hash) &&
		bytes.Equal(newUC.InputRecord.Hash, newUC.InputRecord.PreviousHash) {
		// then new block must not be empty
		if !isZeroHash(newUC.InputRecord.BlockHash) {
			return fmt.Errorf("new UC extends state hash, new state hash does not change, but block is not empty")
		}
	}
	// 6. uc.IR.h′ = uc'.IR.h and uc.IR.h = uc'.IR.h -> previous state hash is equal and new state is not equal,
	// then new block must be empty
	if bytes.Equal(newUC.InputRecord.PreviousHash, prevUC.InputRecord.Hash) &&
		!bytes.Equal(newUC.InputRecord.Hash, newUC.InputRecord.PreviousHash) {
		// then new block must be empty
		if isZeroHash(newUC.InputRecord.BlockHash) {
			return fmt.Errorf("new UC extends state hash, new state hash changes, but block is empty")
		}
	}
	// 7. non-empty block hash can only repeat in repeat UC
	if !isZeroHash(newUC.InputRecord.BlockHash) && bytes.Equal(newUC.InputRecord.BlockHash, prevUC.InputRecord.BlockHash) {
		return fmt.Errorf("new certificate repeats previous block hash")
	}
	return nil
}

func (x *UnicityCertificate) IsRepeat(prevUC *UnicityCertificate) bool {
	return isRepeat(prevUC, x)
}

// isRepeat - check if newUC is a repeat of previous UC.
// Everything else is the same except root round number may be bigger
func isRepeat(prevUC, newUC *UnicityCertificate) bool {
	return EqualIR(prevUC.InputRecord, newUC.InputRecord) &&
		prevUC.UnicitySeal.RootChainRoundNumber < newUC.UnicitySeal.RootChainRoundNumber
}
