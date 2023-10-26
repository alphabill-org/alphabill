package state

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	"github.com/alphabill-org/alphabill/common/mt"
	hasherUtil "github.com/alphabill-org/alphabill/validator/internal/hash"
	"github.com/alphabill-org/alphabill/validator/internal/types"
	"github.com/alphabill-org/alphabill/validator/internal/util"
)

type (
	UnitStateProof struct {
		unitID             types.UnitID
		previousStateHash  []byte
		unitTreeCert       *UnitTreeCert
		dataSummary        uint64
		stateTreeCert      *StateTreeCert
		unicityCertificate *types.UnicityCertificate
	}

	UnitTreeCert struct {
		transactionRecordHash []byte // t
		unitDataHash          []byte // s
		path                  []*mt.PathItem
	}

	StateTreeCert struct {
		leftHash          []byte
		leftSummaryValue  uint64
		rightHash         []byte
		rightSummaryValue uint64

		path []*StateTreePathItem
	}

	StateTreePathItem struct {
		id                  types.UnitID // (ι′)
		hash                []byte       // (z)
		nodeSummaryInput    uint64       // (V)
		siblingHash         []byte
		subTreeSummaryValue uint64
	}

	UnicityCertificateValidator interface {
		Validate(uc *types.UnicityCertificate) error
	}
)

func VerifyUnitStateProof(u *UnitStateProof, algorithm crypto.Hash, ucv UnicityCertificateValidator) error {
	if u == nil {
		return errors.New("unit state proof is nil")
	}
	if u.unitID == nil {
		return errors.New("unit ID is nil")
	}
	if u.unitTreeCert == nil {
		return errors.New("unit tree cert is nil")
	}
	if u.stateTreeCert == nil {
		return errors.New("state tree cert is nil")
	}
	if u.unicityCertificate == nil {
		return errors.New("unicity certificate is nil")
	}
	if err := ucv.Validate(u.unicityCertificate); err != nil {
		return fmt.Errorf("invalid unicity certificate: %w", err)
	}
	ir := u.unicityCertificate.InputRecord
	hash, summary := u.CalculateSateTreeOutput(algorithm)
	if !bytes.Equal(util.Uint64ToBytes(summary), ir.SummaryValue) {
		return fmt.Errorf("invalid summary value: expected %X, got %X", ir.SummaryValue, util.Uint64ToBytes(summary))
	}
	if !bytes.Equal(hash, ir.Hash) {
		return fmt.Errorf("invalid state root hash: expected %X, got %X", ir.Hash, hash)
	}
	return nil
}

func (u *UnitStateProof) CalculateSateTreeOutput(algorithm crypto.Hash) ([]byte, uint64) {
	var z []byte
	if u.unitTreeCert.transactionRecordHash == nil {
		z = hasherUtil.Sum(algorithm,
			u.previousStateHash,
			u.unitTreeCert.unitDataHash,
		)
	} else {
		z = hasherUtil.Sum(algorithm,
			hasherUtil.Sum(algorithm, u.previousStateHash, u.unitTreeCert.transactionRecordHash),
			u.unitTreeCert.unitDataHash,
		)
	}

	logRoot := mt.PlainTreeOutput(u.unitTreeCert.path, z, algorithm)
	id := u.unitID
	sc := u.stateTreeCert
	v := u.dataSummary + sc.leftSummaryValue + sc.rightSummaryValue
	h := computeHash(algorithm, id, logRoot, v, sc.leftHash, sc.leftSummaryValue, sc.rightHash, sc.rightSummaryValue)
	for _, p := range sc.path {
		vv := p.nodeSummaryInput + v + p.subTreeSummaryValue
		if id.Compare(p.id) == -1 {
			h = computeHash(algorithm, p.id, p.hash, vv, h, v, p.siblingHash, p.subTreeSummaryValue)
		} else {
			h = computeHash(algorithm, p.id, p.hash, vv, p.siblingHash, p.subTreeSummaryValue, h, v)
		}
		v = vv
	}
	return h, v
}

func computeHash(algorithm crypto.Hash, id types.UnitID, logRoot []byte, summary uint64, leftHash []byte, leftSummary uint64, rightHash []byte, rightSummary uint64) []byte {
	hasher := algorithm.New()
	hasher.Write(id)
	hasher.Write(logRoot)
	hasher.Write(util.Uint64ToBytes(summary))
	hasher.Write(leftHash)
	hasher.Write(util.Uint64ToBytes(leftSummary))
	hasher.Write(rightHash)
	hasher.Write(util.Uint64ToBytes(rightSummary))
	return hasher.Sum(nil)
}
