package state

import (
	"crypto"
	"fmt"

	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/tree/mt"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

const CBORChecksumLength = 5

type (
	Header struct {
		_                  struct{} `cbor:",toarray"`
		SystemIdentifier   types.SystemID
		UnicityCertificate *types.UnicityCertificate
		NodeRecordCount    uint64
	}

	nodeRecord struct {
		_                  struct{} `cbor:",toarray"`
		UnitID             types.UnitID
		OwnerCondition     []byte
		UnitData           cbor.RawMessage
		UnitLedgerHeadHash []byte
		UnitTreePath       []*mt.PathItem
		HasLeft            bool
		HasRight           bool
	}

	stateSerializer struct {
		encoder       *cbor.Encoder
		hashAlgorithm crypto.Hash
		err           error
	}
)

func newStateSerializer(encoder *cbor.Encoder, hashAlgorithm crypto.Hash) *stateSerializer {
	return &stateSerializer{
		encoder:       encoder,
		hashAlgorithm: hashAlgorithm,
	}
}

func (s *stateSerializer) Traverse(n *avl.Node[types.UnitID, *Unit]) {
	if n == nil || s.err != nil {
		return
	}

	s.Traverse(n.Left())
	s.Traverse(n.Right())
	s.WriteNode(n)
}

func (s *stateSerializer) WriteNode(n *avl.Node[types.UnitID, *Unit]) {
	if s.err != nil {
		return
	}

	unit := n.Value()
	logSize := len(unit.logs)
	if logSize == 0 {
		s.err = fmt.Errorf("unit state log is empty")
	}

	latestLog := unit.logs[logSize-1]
	unitDataBytes, err := cbor.Marshal(latestLog.NewUnitData)
	if err != nil {
		s.err = fmt.Errorf("unable to encode unit data: %w", err)
		return
	}

	merkleTree := mt.New(s.hashAlgorithm, unit.logs)
	unitTreePath, err := merkleTree.GetMerklePath(logSize-1)
	if err != nil {
		s.err = fmt.Errorf("unable to extract unit tree path: %w", err)
		return
	}

	nr := &nodeRecord{
		UnitID:             n.Key(),
		OwnerCondition:     latestLog.NewBearer,
		UnitLedgerHeadHash: latestLog.UnitLedgerHeadHash,
		UnitData:           unitDataBytes,
		UnitTreePath:       unitTreePath,
		HasLeft:            n.Left() != nil,
		HasRight:           n.Right() != nil,
	}
	if err = s.encoder.Encode(nr); err != nil {
		s.err = fmt.Errorf("unable to encode node record: %w", err)
		return
	}
}
