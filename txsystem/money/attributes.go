package money

import (
	"github.com/alphabill-org/alphabill/types"
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
		Counter     uint64
	}

	TransferDCAttributes struct {
		_                 struct{} `cbor:",toarray"`
		Value             uint64
		TargetUnitID      []byte
		TargetUnitCounter uint64
		Counter           uint64
	}

	SplitAttributes struct {
		_              struct{} `cbor:",toarray"`
		TargetUnits    []*TargetUnit
		RemainingValue uint64
		Counter        uint64
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
		Counter    uint64
	}

	UnlockAttributes struct {
		_       struct{} `cbor:",toarray"`
		Counter uint64
	}
)
