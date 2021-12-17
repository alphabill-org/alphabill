package rpc

import (
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/errors"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/ledger"
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

func convertToKnownType(txAttr *anypb.Any) (domain.Normaliser, error) {
	switch txAttr.TypeUrl {
	case protobufTypeUrlPrefix + "BillTransfer":
		pb := &transaction.BillTransfer{}
		err := txAttr.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return &domain.BillTransfer{
			NewBearer: pb.NewBearer,
			Backlink:  pb.Backlink,
		}, nil
	case protobufTypeUrlPrefix + "DustTransfer":
		pb := &transaction.DustTransfer{}
		err := txAttr.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return PbDustTransfer2Domain(pb), nil
	case protobufTypeUrlPrefix + "BillSplit":
		pb := &transaction.BillSplit{}
		err := txAttr.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return &domain.BillSplit{
			Amount:       pb.Amount,
			TargetBearer: pb.TargetBearer,
			TargetValue:  pb.TargetValue,
			Backlink:     pb.Backlink,
		}, nil
	case protobufTypeUrlPrefix + "Swap":
		pb := &transaction.Swap{}
		err := txAttr.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}

		var billIds []*uint256.Int
		for _, biBytes := range pb.BillIdentifiers {
			billIds = append(billIds, uint256.NewInt(0).SetBytes(biBytes))
		}

		var dustTransfers []*domain.DustTransfer
		for _, pbDT := range pb.DustTransferOrders {
			dustTransfers = append(dustTransfers, PbDustTransfer2Domain(pbDT))
		}

		var proofs []*domain.LedgerProof
		for _, pbLP := range pb.Proofs {
			proofs = append(proofs, PbLedgerProof2Domain(pbLP))
		}

		return &domain.Swap{
			OwnerCondition:     pb.OwnerCondition,
			BillIdentifiers:    billIds,
			DustTransferOrders: dustTransfers,
			Proofs:             proofs,
			TargetValue:        pb.TargetValue,
		}, nil
	default:
		return nil, errors.Wrap(UnknownType, "Unknown type: "+txAttr.TypeUrl)
	}
}

func PbDustTransfer2Domain(pb *transaction.DustTransfer) *domain.DustTransfer {
	return &domain.DustTransfer{
		NewBearer:    pb.NewBearer,
		Backlink:     pb.Backlink,
		Nonce:        uint256.NewInt(0).SetBytes(pb.Nonce),
		TargetBearer: pb.TargetBearer,
		TargetValue:  pb.TargetValue,
	}
}

func PbLedgerProof2Domain(pb *ledger.LedgerProof) *domain.LedgerProof {
	return &domain.LedgerProof{PreviousStateHash: pb.PreviousStateHash}
}
