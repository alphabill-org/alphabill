package domain

import (
	"testing"

	"github.com/holiman/uint256"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/rpc/transaction"
	testbytes "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/bytes"
)

func TestFromProtobuf(t *testing.T) {
	var (
		txId       = testbytes.RandomBytes(32)
		ownerProof = []byte{'p', 'r', 'o', 'o', 'f'}
		backlink   = []byte{'b', 'a', 'c', 'k', 'l', 'i', 'n', 'k'}
		newBearer  = []byte{'n', 'e', 'w', 'b', 'e', 'a', 'r', 'e', 'r'}
		timeout    = uint64(678)
	)

	type args struct {
		pbTxOrder transaction.TransactionOrder
	}
	tests := []struct {
		name    string
		args    args
		want    *TransactionOrder
		wantErr error // The expected error or nil if not expecting
	}{
		{
			name: "Bill Transfer type",
			args: func() args {
				pbTxOrder := transaction.TransactionOrder{
					TransactionId:         txId,
					TransactionAttributes: new(anypb.Any),
					Timeout:               timeout,
					OwnerProof:            ownerProof,
				}
				err := anypb.MarshalFrom(pbTxOrder.TransactionAttributes,
					&transaction.BillTransfer{
						NewBearer: newBearer,
						Backlink:  backlink,
					}, proto.MarshalOptions{})
				require.NoError(t, err)
				return args{pbTxOrder}
			}(),
			want: &TransactionOrder{
				TransactionId: uint256.NewInt(0).SetBytes(txId),
				TransactionAttributes: &BillTransfer{
					NewBearer: newBearer,
					Backlink:  backlink,
				},
				Timeout:    timeout,
				OwnerProof: ownerProof,
			},
			wantErr: nil,
		},
		{
			name: "unknown type fails",
			args: func() args {
				pbTxOrder := transaction.TransactionOrder{
					TransactionId:         txId,
					TransactionAttributes: new(anypb.Any),
					Timeout:               timeout,
					OwnerProof:            ownerProof,
				}
				err := anypb.MarshalFrom(pbTxOrder.TransactionAttributes,
					// Transaction Order is not expected inside the convert function
					&transaction.TransactionOrder{
						TransactionId: []byte{1},
					}, proto.MarshalOptions{})
				require.NoError(t, err)
				return args{pbTxOrder}
			}(),
			want: &TransactionOrder{
				TransactionId: uint256.NewInt(0).SetBytes(txId),
				TransactionAttributes: &BillTransfer{
					NewBearer: newBearer,
					Backlink:  backlink,
				},
				Timeout:    timeout,
				OwnerProof: ownerProof,
			},
			wantErr: UnknownType,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromProtobuf(tt.args.pbTxOrder)
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
