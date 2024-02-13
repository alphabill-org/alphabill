package types

import (
	"bytes"
	"crypto"
	"errors"
	"fmt"

	hasherUtil "github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/tree/mt"
	"github.com/alphabill-org/alphabill/util"
	"github.com/fxamacker/cbor/v2"
)

type (
	UnitStateProof struct {
		_                  struct{} `cbor:",toarray"`
		UnitID             UnitID
		PreviousStateHash  []byte
		UnitTreeCert       *UnitTreeCert
		DataSummary        uint64
		StateTreeCert      *StateTreeCert
		UnicityCertificate *UnicityCertificate
	}

	UnitTreeCert struct {
		_                     struct{} `cbor:",toarray"`
		TransactionRecordHash []byte   // t
		UnitDataHash          []byte   // s
		Path                  []*mt.PathItem
	}

	StateTreeCert struct {
		_                 struct{} `cbor:",toarray"`
		LeftHash          []byte
		LeftSummaryValue  uint64
		RightHash         []byte
		RightSummaryValue uint64

		Path []*StateTreePathItem
	}

	StateTreePathItem struct {
		_                   struct{} `cbor:",toarray"`
		ID                  UnitID   // (ι′)
		Hash                []byte   // (z)
		NodeSummaryInput    uint64   // (V)
		SiblingHash         []byte
		SubTreeSummaryValue uint64
	}

	StateUnitData struct {
		Data   cbor.RawMessage
		Bearer PredicateBytes
	}

	UnitDataAndProof struct {
		_        struct{} `cbor:",toarray"`
		UnitData *StateUnitData
		Proof    *UnitStateProof
	}

	UnicityCertificateValidator interface {
		Validate(uc *UnicityCertificate) error
	}
)

func VerifyUnitStateProof(u *UnitStateProof, algorithm crypto.Hash, unitData *StateUnitData, ucv UnicityCertificateValidator) error {
	if u == nil {
		return errors.New("unit state proof is nil")
	}
	if u.UnitID == nil {
		return errors.New("unit ID is nil")
	}
	if u.UnitTreeCert == nil {
		return errors.New("unit tree cert is nil")
	}
	if u.StateTreeCert == nil {
		return errors.New("state tree cert is nil")
	}
	if u.UnicityCertificate == nil {
		return errors.New("unicity certificate is nil")
	}
	if unitData == nil {
		return errors.New("unit data is nil")
	}
	if err := ucv.Validate(u.UnicityCertificate); err != nil {
		return fmt.Errorf("invalid unicity certificate: %w", err)
	}
	hash := unitData.Hash(algorithm)
	if !bytes.Equal(u.UnitTreeCert.UnitDataHash, hash) {
		return errors.New("unit data hash does not match hash in unit tree")
	}
	ir := u.UnicityCertificate.InputRecord
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
	if u.UnitTreeCert.TransactionRecordHash == nil {
		z = hasherUtil.Sum(algorithm,
			u.PreviousStateHash,
			u.UnitTreeCert.UnitDataHash,
		)
	} else {
		z = hasherUtil.Sum(algorithm,
			hasherUtil.Sum(algorithm, u.PreviousStateHash, u.UnitTreeCert.TransactionRecordHash),
			u.UnitTreeCert.UnitDataHash,
		)
	}

	logRoot := mt.PlainTreeOutput(u.UnitTreeCert.Path, z, algorithm)
	id := u.UnitID
	sc := u.StateTreeCert
	v := u.DataSummary + sc.LeftSummaryValue + sc.RightSummaryValue
	h := computeHash(algorithm, id, logRoot, v, sc.LeftHash, sc.LeftSummaryValue, sc.RightHash, sc.RightSummaryValue)
	for _, p := range sc.Path {
		vv := p.NodeSummaryInput + v + p.SubTreeSummaryValue
		if id.Compare(p.ID) == -1 {
			h = computeHash(algorithm, p.ID, p.Hash, vv, h, v, p.SiblingHash, p.SubTreeSummaryValue)
		} else {
			h = computeHash(algorithm, p.ID, p.Hash, vv, p.SiblingHash, p.SubTreeSummaryValue, h, v)
		}
		v = vv
	}
	return h, v
}

func (up *UnitDataAndProof) UnmarshalUnitData(v any) error {
	if up.UnitData == nil {
		return fmt.Errorf("unit data is nil")
	}
	return up.UnitData.UnmarshalData(v)
}

func (sd *StateUnitData) UnmarshalData(v any) error {
	if sd.Data == nil {
		return fmt.Errorf("state unit data is nil")
	}
	return cbor.Unmarshal(sd.Data, v)
}

func (sd *StateUnitData) Hash(hashAlgo crypto.Hash) []byte {
	hasher := hashAlgo.New()
	hasher.Write(sd.Bearer)
	hasher.Write(sd.Data)
	return hasher.Sum(nil)
}

func computeHash(algorithm crypto.Hash, id UnitID, logRoot []byte, summary uint64, leftHash []byte, leftSummary uint64, rightHash []byte, rightSummary uint64) []byte {
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
