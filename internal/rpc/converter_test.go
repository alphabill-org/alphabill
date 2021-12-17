package rpc

import (
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/domain"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/ledger"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	testbytes "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/bytes"
)

func TestPbToTransactionOrder(t *testing.T) {
	var (
		txId           = testbytes.RandomBytes(32)
		billIds        = [][]byte{testbytes.RandomBytes(32), testbytes.RandomBytes(32)}
		ownerProof     = []byte("proof")
		ownerCondition = []byte("ownerCondition")
		backlink       = []byte("backlink")
		nonce          = []byte("nonce")
		newBearer      = []byte("newBearer")
		targetBearer   = []byte("targetBearer")
		amount         = uint64(999)
		timeout        = uint64(777)
		targetValue    = uint64(111)

		converter = &defaultConverter{}
	)

	type args struct {
		pbTxOrder *transaction.TransactionOrder
	}
	tests := []struct {
		name    string
		args    args
		want    *domain.TransactionOrder
		wantErr error // The expected error or nil if not expecting
	}{
		{
			name:    "Bill Transfer type",
			args:    args{newPBTransactionOrder(t, txId, ownerProof, timeout, newPBBillTransfer(newBearer, backlink))},
			want:    newDomainTransactionOrder(txId, ownerProof, timeout, newDomainBillTransfer(newBearer, backlink)),
			wantErr: nil,
		},
		{
			name: "Dust Transfer type",
			args: args{newPBTransactionOrder(t, txId, ownerProof, timeout,
				newPBDustTransfer(newBearer, backlink, nonce, targetBearer, targetValue))},
			want: newDomainTransactionOrder(txId, ownerProof, timeout,
				newDomainDustTransfer(newBearer, backlink, nonce, targetBearer, targetValue)),
			wantErr: nil,
		},
		{
			name: "Bill Split type",
			args: args{newPBTransactionOrder(t, txId, ownerProof, timeout,
				newPBBillSplit(amount, targetBearer, targetValue, backlink))},
			want: newDomainTransactionOrder(txId, ownerProof, timeout,
				newDomainBillSplit(amount, targetBearer, targetValue, backlink)),
			wantErr: nil,
		},
		{
			name: "Swap type",
			args: args{newPBTransactionOrder(t, txId, ownerProof, timeout,
				newPBSwap(
					ownerCondition,
					billIds,
					[]*transaction.DustTransfer{newPBDustTransfer(newBearer, backlink, nonce, targetBearer, targetValue)},
					[]*ledger.LedgerProof{{PreviousStateHash: []byte{1}}},
					targetValue))},
			want: newDomainTransactionOrder(txId, ownerProof, timeout,
				newDomainSwap(ownerCondition,
					billIds,
					[]*domain.DustTransfer{newDomainDustTransfer(newBearer, backlink, nonce, targetBearer, targetValue)},
					[]*domain.LedgerProof{{PreviousStateHash: []byte{1}}},
					targetValue)),
			wantErr: nil,
		},
		{
			name:    "unknown type fails",
			args:    args{newPBTransactionOrder(t, txId, ownerProof, timeout, &transaction.TransactionOrder{TransactionId: []byte{1}})},
			want:    nil,
			wantErr: UnknownType,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := converter.ConvertPbToDomain(tt.args.pbTxOrder)
			if tt.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			assert.Equalf(t, tt.want, got, "FromProtobuf(%v)", tt.args.pbTxOrder)
		})
	}
}

func newPBTransactionOrder(t *testing.T, id, ownerProof []byte, timeout uint64, attr proto.Message) *transaction.TransactionOrder {
	to := &transaction.TransactionOrder{
		TransactionId:         id,
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}
	err := anypb.MarshalFrom(to.TransactionAttributes, attr, proto.MarshalOptions{})
	require.NoError(t, err)
	return to
}

func newDomainTransactionOrder(id, ownerProof []byte, timeout uint64, attr domain.Normaliser) *domain.TransactionOrder {
	return &domain.TransactionOrder{
		TransactionId:         uint256.NewInt(0).SetBytes(id),
		TransactionAttributes: attr,
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}
}

func newPBBillTransfer(newBearer, backlink []byte) *transaction.BillTransfer {
	return &transaction.BillTransfer{
		NewBearer: newBearer,
		Backlink:  backlink,
	}
}

func newDomainBillTransfer(newBearer, backlink []byte) *domain.BillTransfer {
	return &domain.BillTransfer{
		NewBearer: newBearer,
		Backlink:  backlink,
	}
}

func newPBDustTransfer(newBearer, backlink, nonce, targetBearer []byte, targetValue uint64) *transaction.DustTransfer {
	return &transaction.DustTransfer{
		NewBearer:    newBearer,
		Backlink:     backlink,
		Nonce:        nonce,
		TargetBearer: targetBearer,
		TargetValue:  targetValue,
	}
}

func newDomainDustTransfer(newBearer, backlink, nonce, targetBearer []byte, targetValue uint64) *domain.DustTransfer {
	return &domain.DustTransfer{
		NewBearer:    newBearer,
		Backlink:     backlink,
		Nonce:        uint256.NewInt(0).SetBytes(nonce),
		TargetBearer: targetBearer,
		TargetValue:  targetValue,
	}
}

func newPBBillSplit(amount uint64, targetBearer []byte, targetValue uint64, backlink []byte) *transaction.BillSplit {
	return &transaction.BillSplit{
		Amount:       amount,
		TargetBearer: targetBearer,
		TargetValue:  targetValue,
		Backlink:     backlink,
	}
}

func newDomainBillSplit(amount uint64, targetBearer []byte, targetValue uint64, backlink []byte) *domain.BillSplit {
	return &domain.BillSplit{
		Amount:       amount,
		TargetBearer: targetBearer,
		TargetValue:  targetValue,
		Backlink:     backlink,
	}
}
func newPBSwap(ownerCondition []byte, billIdentifiers [][]byte, dustTransferOrder []*transaction.DustTransfer, proofs []*ledger.LedgerProof, targetValue uint64) *transaction.Swap {
	return &transaction.Swap{
		OwnerCondition:     ownerCondition,
		BillIdentifiers:    billIdentifiers,
		DustTransferOrders: dustTransferOrder,
		Proofs:             proofs,
		TargetValue:        targetValue,
	}
}

func newDomainSwap(ownerCondition []byte, billIdentifiers [][]byte, dustTransferOrder []*domain.DustTransfer, proofs []*domain.LedgerProof, targetValue uint64) *domain.Swap {
	var billIds []*uint256.Int
	for _, biBytes := range billIdentifiers {
		billIds = append(billIds, uint256.NewInt(0).SetBytes(biBytes))
	}
	return &domain.Swap{
		OwnerCondition:     ownerCondition,
		BillIdentifiers:    billIds,
		DustTransferOrders: dustTransferOrder,
		Proofs:             proofs,
		TargetValue:        targetValue,
	}
}
