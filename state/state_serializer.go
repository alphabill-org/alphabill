package state

import (
	"fmt"

	"github.com/alphabill-org/alphabill/tree/avl"
	"github.com/alphabill-org/alphabill/types"
	"github.com/fxamacker/cbor/v2"
)

const CBORChecksumLength = 5

type (
	StateFileHeader struct {
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
		HasLeft            bool
		HasRight           bool
	}

	stateSerializer struct {
		encoder *cbor.Encoder
		err     error
	}
)

func newStateSerializer(encoder *cbor.Encoder) *stateSerializer {
	return &stateSerializer{encoder: encoder}
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

	logSize := len(n.Value().logs)
	var stateHash []byte
	if logSize > 0 {
		stateHash = n.Value().logs[logSize-1].UnitLedgerHeadHash
	}

	unitDataBytes, err := cbor.Marshal(n.Value().Data())
	if err != nil {
		s.err = fmt.Errorf("unable to encode unit data: %w", err)
		return
	}

	nr := &nodeRecord{
		UnitID:             n.Key(),
		OwnerCondition:     n.Value().Bearer(),
		UnitLedgerHeadHash: stateHash,
		UnitData:           unitDataBytes,
		HasLeft:            n.Left() != nil,
		HasRight:           n.Right() != nil,
	}
	if err = s.encoder.Encode(nr); err != nil {
		s.err = fmt.Errorf("unable to encode node record: %w", err)
		return
	}
}
