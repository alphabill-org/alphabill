package transactions

import (
	"github.com/alphabill-org/alphabill/internal/types"
)

const (
	PayloadTypeAddFeeCredit      = "addFC"
	PayloadTypeCloseFeeCredit    = "closeFC"
	PayloadTypeReclaimFeeCredit  = "reclFC"
	PayloadTypeTransferFeeCredit = "transFC"
)

type (
	AddFeeCreditAttributes struct {
		_                       struct{}                 `cbor:",toarray"`
		FeeCreditOwnerCondition []byte                   // target fee credit record owner condition
		FeeCreditTransfer       *types.TransactionRecord // bill transfer record of type "transfer fee credit"
		FeeCreditTransferProof  *types.TxProof           // transaction proof of "transfer fee credit" transaction
	}

	TransferFeeCreditAttributes struct {
		_                      struct{} `cbor:",toarray"`
		Amount                 uint64   // amount to transfer
		TargetSystemIdentifier []byte   // system_identifier of the target partition
		TargetRecordID         []byte   // unit id of the corresponding “add fee credit” transaction
		EarliestAdditionTime   uint64   // earliest round when the corresponding “add fee credit” transaction can be executed in the target system
		LatestAdditionTime     uint64   // latest round when the corresponding “add fee credit” transaction can be executed in the target system
		Nonce                  []byte   // the current state hash of the target credit record if the record exists, or to nil if the record does not exist yet
		Backlink               []byte   // hash of this unit's previous transaction
	}

	CloseFeeCreditAttributes struct {
		_ struct{} `cbor:",toarray"`

		Amount       uint64 // current balance of the fee credit record
		TargetUnitID []byte // unit id of the fee credit record in money partition
		Nonce        []byte // the current state hash of the target unit in money partition
	}

	ReclaimFeeCreditAttributes struct {
		_                      struct{}                 `cbor:",toarray"`
		CloseFeeCreditTransfer *types.TransactionRecord // bill transfer record of type "close fee credit"
		CloseFeeCreditProof    *types.TxProof           // transaction proof of "close fee credit" transaction
		Backlink               []byte                   // hash of this unit's previous transaction
	}
)
