package rpc

import (
	"alphabill-wallet-sdk/internal/alphabill/domain"
	"alphabill-wallet-sdk/internal/errors"
	"alphabill-wallet-sdk/internal/rpc/transaction"
	"github.com/holiman/uint256"
	"google.golang.org/protobuf/types/known/anypb"
)

const ProtobufTypeUrlPrefix = "type.googleapis.com/rpc."

var (
	UnknownType = errors.New("Cannot convert protobuf, unknown type")
)

type TransactionMapper interface {
	MapPbToDomain(pbTxOrder *transaction.Transaction) (*domain.Transaction, error)
}

type defaultMapper struct{}

func NewDefaultTxMapper() *defaultMapper {
	return &defaultMapper{}
}

// MapPbToDomain creates new Transaction from corresponding protobuf object.
// Does not copy all values. I.e. only array pointers are copied.
func (c *defaultMapper) MapPbToDomain(pbTxOrder *transaction.Transaction) (*domain.Transaction, error) {
	// Validation
	if len(pbTxOrder.UnitId) != 32 {
		return nil, errors.Errorf("Transaction ID must be 32 bytes, but is %d", len(pbTxOrder.UnitId))
	}
	txAttr, err := convertToKnownType(pbTxOrder.TransactionAttributes)
	if err != nil {
		return nil, err
	}
	return &domain.Transaction{
		// Expects bytes to represent number as a big-endian unsigned integer.
		// If pbTxOrder.UnitId has more than 32 bytes, the first bytes will be discarded.
		// if pbTxOrder.UnitId has less than 32 bytes, then the last bytes will be 0.
		UnitId:                uint256.NewInt(0).SetBytes(pbTxOrder.UnitId),
		TransactionAttributes: txAttr,
		Timeout:               pbTxOrder.Timeout,
		OwnerProof:            pbTxOrder.OwnerProof,
	}, nil
}

func convertToKnownType(txAttr *anypb.Any) (domain.TxAttributes, error) {
	switch txAttr.TypeUrl {
	case ProtobufTypeUrlPrefix + "BillTransfer":
		pb := &transaction.BillTransfer{}
		err := txAttr.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return &domain.BillTransfer{
			NewBearer: pb.NewBearer,
			Amount:    pb.TargetValue,
			Backlink:  pb.Backlink,
		}, nil
	case ProtobufTypeUrlPrefix + "DustTransfer":
		pb := &transaction.DustTransfer{}
		err := txAttr.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return PbDustTransfer2Domain(pb), nil
	case ProtobufTypeUrlPrefix + "BillSplit":
		pb := &transaction.BillSplit{}
		err := txAttr.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return &domain.BillSplit{
			Amount:         pb.Amount,
			TargetBearer:   pb.TargetBearer,
			RemainingValue: pb.RemainingValue,
			Backlink:       pb.Backlink,
		}, nil
	case ProtobufTypeUrlPrefix + "Swap":
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
			dustTransfer, err := PbTx2DustTransferDomain(pbDT)
			if err != nil {
				return nil, err
			}
			dustTransfers = append(dustTransfers, dustTransfer)
		}

		//var proofs []*domain.LedgerProof
		//for _, pbLP := range pb.Proofs {
		//	proofs = append(proofs, PbLedgerProof2Domain(pbLP))
		//}

		return &domain.Swap{
			BearerCondition:    pb.OwnerCondition,
			BillIdentifiers:    billIds,
			DustTransferOrders: dustTransfers,
			//Proofs:             proofs,
			TargetValue: pb.TargetValue,
		}, nil
	default:
		return nil, errors.Wrap(UnknownType, "Unknown type: "+txAttr.TypeUrl)
	}
}

func PbDustTransfer2Domain(pb *transaction.DustTransfer) *domain.DustTransfer {
	return &domain.DustTransfer{
		NewBearer:   pb.NewBearer,
		Backlink:    pb.Backlink,
		Nonce:       uint256.NewInt(0).SetBytes(pb.Nonce),
		TargetValue: pb.TargetValue,
	}
}

func PbTx2DustTransferDomain(pb *transaction.Transaction) (*domain.DustTransfer, error) {
	dt := &transaction.DustTransfer{}
	err := pb.TransactionAttributes.UnmarshalTo(dt)
	if err != nil {
		return nil, err
	}
	return PbDustTransfer2Domain(dt), nil
}

//func PbLedgerProof2Domain(pb *transaction.LedgerProof) *domain.LedgerProof {
//	return &domain.LedgerProof{PreviousStateHash: pb.PreviousStateHash}
//}
