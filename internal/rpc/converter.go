package rpc

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	"github.com/holiman/uint256"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	UnknownType = errors.New("Cannot convert protobuf, unknown type")
)

type defaultConverter struct{}

const protobufTypeUrlPrefix = "type.googleapis.com/rpc."

// ConvertPbToDomain creates new TransactionOrder from corresponding protobuf object.
// Does not copy all values. I.e. only array pointers are copied.
func (c *defaultConverter) ConvertPbToDomain(pbTxOrder *transaction.TransactionOrder) (*domain.TransactionOrder, error) {
	// Validation
	if len(pbTxOrder.TransactionId) != 32 {
		return nil, errors.Errorf("Transaction ID must be 32 bytes, but is %d", len(pbTxOrder.TransactionId))
	}
	txAttr, err := convertToKnownType(pbTxOrder.TransactionAttributes)
	if err != nil {
		return nil, err
	}
	return &domain.TransactionOrder{
		// Expects bytes to represent number as a big-endian unsigned integer.
		// If pbTxOrder.TransactionId has more than 32 bytes, the first bytes will be discarded.
		// if pbTxOrder.TransactionId has less than 32 bytes, then the last bytes will be 0.
		TransactionId:         uint256.NewInt(0).SetBytes(pbTxOrder.TransactionId),
		TransactionAttributes: txAttr,
		Timeout:               pbTxOrder.Timeout,
		OwnerProof:            pbTxOrder.OwnerProof,
	}, nil
}

func convertToKnownType(txAttr *anypb.Any) (interface{}, error) {
	switch txAttr.TypeUrl {
	case protobufTypeUrlPrefix + "BillTransfer":
		btProto := &transaction.BillTransfer{}
		err := txAttr.UnmarshalTo(btProto)
		if err != nil {
			return nil, err
		}
		return &domain.BillTransfer{
			NewBearer: btProto.NewBearer,
			Backlink:  btProto.Backlink,
		}, nil
	default:
		return nil, errors.Wrap(UnknownType, "Unknown type: "+txAttr.TypeUrl)
	}
}
