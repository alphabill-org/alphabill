package wallet

import (
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/types"
)

type TxHash []byte

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

func (p *Proof) GetActualFee() uint64 {
	if p == nil {
		return 0
	}
	return p.TxRecord.GetActualFee()
}
