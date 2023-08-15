package money

import (
	"github.com/alphabill-org/alphabill/internal/types"
)

const (
	PayloadTypeTransfer = "trans"
	PayloadTypeSplit    = "split"
	PayloadTypeTransDC  = "transDC"
	PayloadTypeSwapDC   = "swapDC"
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
		Amount         uint64
		TargetBearer   []byte
		RemainingValue uint64
		Backlink       []byte
	}

	SwapDCAttributes struct {
		_                struct{} `cbor:",toarray"`
		OwnerCondition   []byte
		DcTransfers      []*types.TransactionRecord
		DcTransferProofs []*types.TxProof
		TargetValue      uint64 // value added to target bill
	}
)
