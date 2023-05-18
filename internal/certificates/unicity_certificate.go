package certificates

import (
	"bytes"
	gocrypto "crypto"
	"errors"
	"fmt"
	"hash"

	"github.com/alphabill-org/alphabill/internal/crypto"
)

var ErrUnicityCertificateIsNil = errors.New("unicity certificate is nil")

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
	if x != nil {
		return x.InputRecord.GetRoundNumber()
	}
	return 0
}

// CheckNonEquivocatingCertificates checks if provided certificates are equivocating
// NB! order is important, and it is assumed that validity is checked before
func CheckNonEquivocatingCertificates(prevUC *UnicityCertificate, newUC *UnicityCertificate) error {
	if newUC == nil {
		return ErrUCIsNil
	}
	if prevUC == nil {
		return ErrLastUCIsNil
	}
	if newUC.UnicitySeal.RootChainRoundNumber < prevUC.UnicitySeal.RootChainRoundNumber {
		return fmt.Errorf("new certificate is from older root round %v than previous certificate %v",
			newUC.UnicitySeal.RootChainRoundNumber, prevUC.UnicitySeal.RootChainRoundNumber)
	}
	if newUC.InputRecord.RoundNumber < prevUC.InputRecord.RoundNumber {
		return fmt.Errorf("new certificate is from older partition round %v than previous certificate %v",
			newUC.InputRecord.RoundNumber, prevUC.InputRecord.RoundNumber)
	}
	// 1. uc.IR.hB = 0H - empty block must not change state - this is done when certificate validity is verified
	if isZeroHash(newUC.InputRecord.BlockHash) && !bytes.Equal(newUC.InputRecord.PreviousHash, newUC.InputRecord.Hash) {
		return fmt.Errorf("state hash has changed, but block is zero hash")
	}
	// 2. uc.IR.n = uc′ .IR.n - if the partition round number is the same then input records must also match
	if newUC.InputRecord.RoundNumber == prevUC.InputRecord.RoundNumber {
		if !bytes.Equal(newUC.InputRecord.Bytes(), prevUC.InputRecord.Bytes()) {
			return fmt.Errorf("different input records for same partition round %v", newUC.InputRecord.RoundNumber)
		}
		// ok, these are just duplicates
	}
	// 3. uc.IR.h′ = uc′.IR.h′ - if the certificates input record previous hash is the same
	if bytes.Equal(newUC.InputRecord.PreviousHash, prevUC.InputRecord.PreviousHash) {
		// then either: - the new state hash is equal as well - both repeat UC's or
		//				- the new UC extends from the previous UC
		if !bytes.Equal(newUC.InputRecord.Hash, prevUC.InputRecord.Hash) &&
			!bytes.Equal(prevUC.InputRecord.Hash, newUC.InputRecord.PreviousHash) {
			return fmt.Errorf("previous state hash is equal, but new certificate does not extend it nor is it a repeat certificate")
		}
	}
	// 4. uc.IR.h = uc′ .IR.h - if certificates new input record new state hash is the same
	if bytes.Equal(newUC.InputRecord.Hash, prevUC.InputRecord.Hash) {
		// then either: - they are both repeat UC's old state hash is also the same or
		//              - new UC is repeat UC from previous state
		if !bytes.Equal(newUC.InputRecord.PreviousHash, prevUC.InputRecord.PreviousHash) &&
			!bytes.Equal(newUC.InputRecord.Hash, newUC.InputRecord.PreviousHash) {
			return fmt.Errorf("new state hash is equal, but both are not repeat certificates of the same state nor is the new repeat certificate")
		}
	}
	// 5. uc.IR.n = uc′ .IR.n + 1 - if this new UC follows previous UC
	if newUC.InputRecord.RoundNumber == prevUC.InputRecord.RoundNumber+1 {
		if !bytes.Equal(newUC.InputRecord.PreviousHash, prevUC.InputRecord.Hash) {
			return fmt.Errorf("new certiticte does not extend previous state hash")
		}
	}
	return nil
}
