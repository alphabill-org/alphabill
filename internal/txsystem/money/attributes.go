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
		_            struct{} `cbor:",toarray"`
		Nonce        []byte
		TargetBearer []byte
		TargetValue  uint64
		Backlink     []byte
	}

	SplitAttributes struct {
		_              struct{} `cbor:",toarray"`
		Amount         uint64
		TargetBearer   []byte
		RemainingValue uint64
		Backlink       []byte
	}

	SwapDCAttributes struct {
		_               struct{} `cbor:",toarray"`
		OwnerCondition  []byte
		BillIdentifiers [][]byte
		DcTransfers     []*types.TransactionRecord
		Proofs          []*types.TxProof
		TargetValue     uint64
	}
)
