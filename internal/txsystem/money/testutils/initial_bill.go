package testutils

import (
	"github.com/alphabill-org/alphabill/validator/internal/hash"
	"github.com/alphabill-org/alphabill/validator/internal/predicates/templates"
	moneytx "github.com/alphabill-org/alphabill/validator/internal/txsystem/money"
	"github.com/alphabill-org/alphabill/validator/internal/types"
	"github.com/fxamacker/cbor/v2"
)

func CreateInitialBillTransferTx(pubKey []byte, billID, fcrID types.UnitID, billValue uint64, timeout uint64, backlink []byte) (*types.TransactionOrder, error) {
	attr := &moneytx.TransferAttributes{
		NewBearer:   templates.NewP2pkh256BytesFromKeyHash(hash.Sum256(pubKey)),
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
				FeeCreditRecordID: fcrID,
			},
		},
		OwnerProof: nil,
	}, nil
}
