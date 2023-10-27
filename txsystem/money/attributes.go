package money

import (
	"github.com/alphabill-org/alphabill/api/types"
)

const (
	PayloadTypeTransfer = "trans"
	PayloadTypeSplit    = "split"
	PayloadTypeTransDC  = "transDC"
	PayloadTypeSwapDC   = "swapDC"
	PayloadTypeLock     = "lock"
	PayloadTypeUnlock   = "unlock"
)

type (
	TransferAttributes struct {
		_           struct{} `cbor:",toarray"`
		NewBearer   []byte
		TargetValue uint64
		Backlink    []byte
	}

	TransferDCAttributes struct {
		_                  struct{} `cbor:",toarray"`
		Value              uint64
		TargetUnitID       []byte
		TargetUnitBacklink []byte
		Backlink           []byte
	}

	SplitAttributes struct {
		_              struct{} `cbor:",toarray"`
		TargetUnits    []*TargetUnit
		RemainingValue uint64
		Backlink       []byte
	}

	TargetUnit struct {
		_              struct{} `cbor:",toarray"`
		Amount         uint64
		OwnerCondition []byte
	}

	SwapDCAttributes struct {
		_                struct{} `cbor:",toarray"`
		OwnerCondition   []byte
		DcTransfers      []*types.TransactionRecord
		DcTransferProofs []*types.TxProof
		TargetValue      uint64 // value added to target bill
	}

	LockAttributes struct {
		_          struct{} `cbor:",toarray"`
		LockStatus uint64   // status of the lock, non-zero value means locked
		Backlink   []byte
	}

	UnlockAttributes struct {
		_        struct{} `cbor:",toarray"`
		Backlink []byte
	}
)
