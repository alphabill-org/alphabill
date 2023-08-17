package testutils

import (
	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/script"
	testmoney "github.com/alphabill-org/alphabill/internal/testutils/money"
	moneytx "github.com/alphabill-org/alphabill/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/fxamacker/cbor/v2"
)

func CreateInitialBillTransferTx(pubKey []byte, billID types.UnitID, billValue uint64, timeout uint64, backlink []byte) (*types.TransactionOrder, error) {
	attr := &moneytx.TransferAttributes{
		NewBearer:   script.PredicatePayToPublicKeyHashDefault(hash.Sum256(pubKey)),
		TargetValue: billValue,
		Backlink:    backlink,
	}
	attrBytes, err := cbor.Marshal(attr)
	if err != nil {
		return nil, err
	}
	return &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:   []byte{0, 0, 0, 0},
			Type:       moneytx.PayloadTypeTransfer,
			UnitID:     billID,
			Attributes: attrBytes,
			ClientMetadata: &types.ClientMetadata{
				Timeout:           timeout,
				MaxTransactionFee: 1,
				FeeCreditRecordID: testmoney.FCRID,
			},
		},
		OwnerProof: script.PredicateArgumentEmpty(),
	}, nil
}
