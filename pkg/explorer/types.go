package explorer

import (
	"github.com/alphabill-org/alphabill/internal/types"
)

type (
	BlockExplorer struct {
		_               struct{} `cbor:",toarray"`
		SystemID        *types.SystemID
		RoundNumber     uint64
		Header          *HeaderExplorer
		TxHashes        []string
		SummaryValue    []byte // summary value to certified
		SumOfEarnedFees uint64 // sum of the actual fees over all transaction records in the block
	}
	HeaderExplorer struct {
		_                 struct{} `cbor:",toarray"`
		Timestamp         uint64
		BlockHash         []byte
		PreviousBlockHash []byte
		ProposerID        string // validator
	}
	TxExplorer struct {
		_                struct{} `cbor:",toarray"`
		Hash             string
		BlockNumber      uint64
		Timeout          uint64
		PayloadType      string	
		Status           *types.TxStatus
		TargetUnits      []types.UnitID
		TransactionOrder *types.TransactionOrder
		Fee              uint64
	}

	// TransactionOrder struct {
	// 	_          struct{} `cbor:",toarray"`
	// 	Payload    *Payload
	// 	OwnerProof []byte
	// 	FeeProof   []byte
	// }

	// Payload struct {
	// 	_              struct{} `cbor:",toarray"`
	// 	SystemID       SystemID
	// 	Type           string
	// 	UnitID         UnitID
	// 	Attributes     RawCBOR
	// 	ClientMetadata *ClientMetadata
	// }

	// ClientMetadata struct {
	// 	_                 struct{} `cbor:",toarray"`
	// 	Timeout           uint64
	// 	MaxTransactionFee uint64
	// 	FeeCreditRecordID []byte
	// }
)
