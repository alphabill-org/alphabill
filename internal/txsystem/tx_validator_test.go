package txsystem

import (
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestTransfer(t *testing.T) {
	tests := []struct {
		name string
		bd   *BillData
		tx   *transfer
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransfer(100, []byte{6}),
			res:  nil,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransfer(101, []byte{6}),
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidBacklink",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransfer(100, []byte{5}),
			res:  ErrInvalidBacklink,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTransfer(tt.bd, tt.tx)
			if tt.res == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.res)
			}
		})
	}
}

func TestTransferDC(t *testing.T) {
	tests := []struct {
		name string
		bd   *BillData
		tx   *transferDC
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransferDC(100, []byte{6}),
			res:  nil,
		},
		{
			name: "InvalidBalance",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransferDC(101, []byte{6}),
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidBacklink",
			bd:   newBillData(100, []byte{6}),
			tx:   newTransferDC(100, []byte{5}),
			res:  ErrInvalidBacklink,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTransferDC(tt.bd, tt.tx)
			if tt.res == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.res)
			}
		})
	}
}

func TestSplit(t *testing.T) {
	tests := []struct {
		name string
		bd   *BillData
		tx   *split
		res  error
	}{
		{
			name: "Ok",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(50, 50, []byte{6}),
			res:  nil,
		},
		{
			name: "AmountExceedsBillValue",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(101, 100, []byte{6}),
			res:  ErrInvalidBillValue,
		},
		{
			name: "AmountEqualsBillValue",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(100, 0, []byte{6}),
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidRemainingValue",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(50, 51, []byte{6}),
			res:  ErrInvalidBillValue,
		},
		{
			name: "InvalidBacklink",
			bd:   newBillData(100, []byte{6}),
			tx:   newSplit(50, 50, []byte{5}),
			res:  ErrInvalidBacklink,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSplit(tt.bd, tt.tx)
			if tt.res == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.res)
			}
		})
	}
}

func TestGenericTxValidation(t *testing.T) {
	tests := []struct {
		name    string
		gtx     GenericTransaction
		blockNo uint64
		res     error
	}{
		{
			name:    "Ok",
			gtx:     newTransferWithTimeout(11),
			blockNo: 10,
			res:     nil,
		},
		{
			name:    "TransactionExpired",
			gtx:     newTransferWithTimeout(10),
			blockNo: 10,
			res:     ErrTransactionExpired,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGenericTransaction(tt.gtx, tt.blockNo)
			if tt.res == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tt.res)
			}
		})
	}
}

func newTransfer(v uint64, backlink []byte) *transfer {
	return &transfer{
		genericTx: genericTx{
			unitId:     uint256.NewInt(1),
			timeout:    2,
			ownerProof: []byte{3},
		},
		newBearer:   []byte{4},
		targetValue: v,
		backlink:    backlink,
	}
}

func newTransferWithTimeout(timeout uint64) *transfer {
	return &transfer{
		genericTx: genericTx{
			unitId:     uint256.NewInt(1),
			timeout:    timeout,
			ownerProof: []byte{3},
		},
		newBearer:   []byte{4},
		targetValue: 5,
		backlink:    []byte{6},
	}
}

func newTransferDC(v uint64, backlink []byte) *transferDC {
	return &transferDC{
		genericTx: genericTx{
			unitId:     uint256.NewInt(1),
			timeout:    2,
			ownerProof: []byte{3},
		},
		targetBearer: []byte{4},
		targetValue:  v,
		backlink:     backlink,
		nonce:        []byte{7},
	}
}

func newSplit(amount uint64, remainingValue uint64, backlink []byte) *split {
	return &split{
		genericTx: genericTx{
			unitId:     uint256.NewInt(1),
			timeout:    2,
			ownerProof: []byte{3},
		},
		amount:         amount,
		targetBearer:   []byte{5},
		remainingValue: remainingValue,
		backlink:       backlink,
	}
}

func newBillData(v uint64, backlink []byte) *BillData {
	return &BillData{V: v, Backlink: backlink}
}
