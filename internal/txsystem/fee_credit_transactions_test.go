package txsystem

import (
	"bytes"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	systemID   = []byte{0, 0, 0, 0}
	unitID     = []byte{1}
	ownerProof = []byte{2}
	backlink   = []byte{3}
	nonce      = []byte{4}
	recordID   = []byte{5}
	timeout    = uint64(100)
	amount     = uint64(101)
	t1         = uint64(102)
	t2         = uint64(103)
)

func TestWrapper_TransferFC(t *testing.T) {
	var (
		pbTransferFC  = newPBTransferFC(1, 2, 3, systemID, test.RandomBytes(32), test.RandomBytes(32), test.RandomBytes(32))
		pbTransaction = newPBTransactionOrder(test.RandomBytes(32), test.RandomBytes(32), 555, pbTransferFC)
	)
	genericTx, err := NewFeeCreditGenericTx(systemID, pbTransaction)
	require.NoError(t, err)
	fc, ok := genericTx.(*TransferFCWrapper)
	require.True(t, ok)

	require.Equal(t, pbTransaction.SystemId, fc.SystemID())
	require.Equal(t, pbTransaction.UnitId, util.Uint256ToBytes(fc.UnitID()))
	require.Equal(t, pbTransaction.Timeout, fc.Timeout())
	require.Equal(t, pbTransaction.OwnerProof, fc.OwnerProof())

	require.Equal(t, pbTransferFC.Amount, fc.TransferFC.Amount)
	require.Equal(t, pbTransferFC.EarliestAdditionTime, fc.TransferFC.EarliestAdditionTime)
	require.Equal(t, pbTransferFC.LatestAdditionTime, fc.TransferFC.LatestAdditionTime)
	require.Equal(t, pbTransferFC.TargetSystemIdentifier, fc.TransferFC.TargetSystemIdentifier)
	require.Equal(t, pbTransferFC.TargetRecordId, fc.TransferFC.TargetRecordId)
	require.Equal(t, pbTransferFC.Nonce, fc.TransferFC.Nonce)
	require.Equal(t, pbTransferFC.Backlink, fc.TransferFC.Backlink)
}

func TestTransferFCTx_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := createTransferFCTxOrder()
	sigBytes := tx.SigBytes()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(amount))
	b.Write(systemID)
	b.Write(recordID)
	b.Write(util.Uint64ToBytes(t1))
	b.Write(util.Uint64ToBytes(t2))
	b.Write(nonce)
	b.Write(backlink)
	require.Equal(t, b.Bytes(), sigBytes)
}

func newPBTransferFC(amount, t1, t2 uint64, sysID, recID, nonce, backlink []byte) *TransferFCOrder {
	return &TransferFCOrder{
		Amount:                 amount,
		TargetSystemIdentifier: sysID,
		TargetRecordId:         recID,
		EarliestAdditionTime:   t1,
		LatestAdditionTime:     t2,
		Nonce:                  nonce,
		Backlink:               backlink,
	}
}

func createTransferFCTxOrder() GenericTransaction {
	tx := &Transaction{
		SystemId:              systemID,
		TransactionAttributes: new(anypb.Any),
		UnitId:                unitID,
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}
	_ = tx.TransactionAttributes.MarshalFrom(newPBTransferFC(amount, t1, t2, systemID, recordID, nonce, backlink))
	gtx, _ := newFeeCreditGenericTx(tx)
	return gtx
}

func newPBTransactionOrder(id, ownerProof []byte, timeout uint64, attr proto.Message) *Transaction {
	to := &Transaction{
		SystemId:              systemID,
		UnitId:                id,
		TransactionAttributes: new(anypb.Any),
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}
	err := anypb.MarshalFrom(to.TransactionAttributes, attr, proto.MarshalOptions{})
	if err != nil {
		panic(err)
	}
	return to
}

func newFeeCreditGenericTx(tx *Transaction) (GenericTransaction, error) {
	return NewFeeCreditGenericTx(systemID, tx)
}

// NewFeeCreditGenericTx creates a new wrapper, returns an error if unknown transaction type is given as argument.
func NewFeeCreditGenericTx(systemID []byte, tx *Transaction) (GenericTransaction, error) {
	if !bytes.Equal(systemID, tx.GetSystemId()) {
		return nil, errors.Errorf("transaction has invalid system identifier %X, expected %X", tx.GetSystemId(), systemID)
	}

	switch tx.TransactionAttributes.TypeUrl {
	case typeURLTransferFCOrder:
		pb := &TransferFCOrder{}
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return &TransferFCWrapper{
			Wrapper:    Wrapper{Transaction: tx},
			TransferFC: pb,
		}, nil
	default:
		return nil, errors.Errorf("unknown transaction type %s", tx.TransactionAttributes.TypeUrl)
	}
}
