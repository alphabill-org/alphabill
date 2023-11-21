package wallet

import (
	"crypto"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/types"
)

type TxHash []byte

type UnitID types.UnitID

type TransactionOrder types.TransactionOrder

type Transactions struct {
	_            struct{} `cbor:",toarray"`
	Transactions []*types.TransactionOrder
}

type Predicate []byte

type PubKey []byte

type PubKeyHash []byte

// TxProof type alias for block.TxProof, can be removed once block package is moved out of internal
type TxProof = types.TxProof

// Proof wrapper struct around TxRecord and TxProof
type Proof struct {
	_        struct{}                 `cbor:",toarray"`
	TxRecord *types.TransactionRecord `json:"txRecord"`
	TxProof  *types.TxProof           `json:"txProof"`
}

func (pk PubKey) Hash() PubKeyHash {
	return hash.Sum256(pk)
}

type (
	TxHistoryRecord struct {
		_            struct{} `cbor:",toarray"`
		UnitID       types.UnitID
		TxHash       TxHash
		CounterParty []byte
		Timeout      uint64
		State        TxHistoryRecordState
		Kind         TxHistoryRecordKind
	}

	TxHistoryRecordState byte
	TxHistoryRecordKind  byte
)

const (
	OUTGOING TxHistoryRecordKind = iota
	INCOMING
)

const (
	UNCONFIRMED TxHistoryRecordState = iota
	CONFIRMED
	FAILED
)

func (t *TransactionOrder) Cast() *types.TransactionOrder {
	return (*types.TransactionOrder)(t)
}

func (t *TransactionOrder) Timeout() uint64 {
	return t.Cast().Timeout()
}

func (t *TransactionOrder) UnitID() UnitID {
	return UnitID(t.Cast().UnitID())
}

func (t *TransactionOrder) SystemID() []byte {
	return t.Cast().SystemID()
}

func (t *TransactionOrder) PayloadType() string {
	return t.Cast().PayloadType()
}

func (t *TransactionOrder) UnmarshalAttributes(v any) error {
	return t.Cast().UnmarshalAttributes(v)
}

func (t *TransactionOrder) Hash(algorithm crypto.Hash) []byte {
	return t.Cast().Hash(algorithm)
}
