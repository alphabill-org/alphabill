package state

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill-go-base/tree/mt"
	"github.com/alphabill-org/alphabill-go-base/types"
	"github.com/alphabill-org/alphabill/tree/avl"
)

const CBORChecksumLength = 5

type (
	header struct {
		_                  struct{} `cbor:",toarray"`
		Version            types.ABVersion
		UnicityCertificate *types.UnicityCertificate
		NodeRecordCount    uint64
	}

	nodeRecord struct {
		_                  struct{} `cbor:",toarray"`
		Version            types.ABVersion
		UnitID             types.UnitID
		UnitData           types.RawCBOR
		UnitLedgerHeadHash []byte
		UnitTreePath       []*mt.PathItem
		HasLeft            bool
		HasRight           bool
	}

	stateSerializer struct {
		encode        func(any) error
		hashAlgorithm crypto.Hash
	}
)

func newStateSerializer(encoder func(any) error, hashAlgorithm crypto.Hash) *stateSerializer {
	return &stateSerializer{
		encode:        encoder,
		hashAlgorithm: hashAlgorithm,
	}
}

func (s *stateSerializer) Traverse(n *avl.Node[types.UnitID, VersionedUnit]) error {
	if n == nil {
		return nil
	}

	if err := s.Traverse(n.Left()); err != nil {
		return err
	}
	if err := s.Traverse(n.Right()); err != nil {
		return err
	}

	return s.WriteNode(n)
}

func (s *stateSerializer) WriteNode(n *avl.Node[types.UnitID, VersionedUnit]) error {
	unit, err := ToUnitV1(n.Value())
	if err != nil {
		return fmt.Errorf("failed to get unit: %w", err)
	}
	logSize := len(unit.logs)
	if logSize == 0 {
		return fmt.Errorf("unit state log is empty")
	}

	latestLog := unit.logs[logSize-1]
	unitDataBytes, err := types.Cbor.Marshal(latestLog.NewUnitData)
	if err != nil {
		return fmt.Errorf("unable to encode unit data: %w", err)
	}

	merkleTree, err := mt.New(s.hashAlgorithm, unit.logs)
	if err != nil {
		return fmt.Errorf("unable to create Merkle tree: %w", err)
	}
	unitTreePath, err := merkleTree.GetMerklePath(logSize - 1)
	if err != nil {
		return fmt.Errorf("unable to extract unit tree path: %w", err)
	}

	nr := &nodeRecord{
		Version:            unit.GetVersion(),
		UnitID:             n.Key(),
		UnitLedgerHeadHash: latestLog.UnitLedgerHeadHash,
		UnitData:           unitDataBytes,
		UnitTreePath:       unitTreePath,
		HasLeft:            n.Left() != nil,
		HasRight:           n.Right() != nil,
	}
	if err = s.encode(nr); err != nil {
		return fmt.Errorf("unable to encode node record: %w", err)
	}
	return nil
}
