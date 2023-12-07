package wallet

import (
	"github.com/alphabill-org/alphabill/hash"
	"github.com/alphabill-org/alphabill/types"
)

const (
	LockReasonAddFees = 1 + iota
	LockReasonReclaimFees
	LockReasonCollectDust
	LockReasonManual
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

type LockReason uint64

func (r LockReason) String() string {
	switch r {
	case LockReasonAddFees:
		return "locked for adding fees"
	case LockReasonReclaimFees:
		return "locked for reclaiming fees"
	case LockReasonCollectDust:
		return "locked for dust collection"
	case LockReasonManual:
		return "manually locked by user"
	}
	return ""
}

type RoundNumber struct {
	RoundNumber            uint64 `json:"roundNumber,string"`            // last known round number
	LastIndexedRoundNumber uint64 `json:"lastIndexedRoundNumber,string"` // last indexed round number
}
