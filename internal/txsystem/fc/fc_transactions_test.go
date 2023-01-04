package fc

import (
	"bytes"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	systemID        = []byte{0, 0, 0, 0}
	unitID          = []byte{1}
	ownerProof      = []byte{2}
	backlink        = []byte{3}
	nonce           = []byte{4}
	recordID        = []byte{5}
	owner           = []byte{6}
	blockHeaderHash = []byte{7}
	targetUnitId    = []byte{8}
	timeout         = uint64(100)
	amount          = uint64(101)
	t1              = uint64(102)
	t2              = uint64(103)
)

func TestWrapper_TransferFC(t *testing.T) {
	var (
		pbTransferFC  = newPBTransferFC(1, 2, 3, systemID, test.RandomBytes(32), test.RandomBytes(32), test.RandomBytes(32))
		pbTransaction = newPBTransactionOrder(test.RandomBytes(32), test.RandomBytes(32), 555, pbTransferFC)
	)
	genericTx, err := toGenericTx(pbTransaction)
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

func TestWrapper_AddFC(t *testing.T) {
	var (
		pbTransferFC  = createTransferFCTxOrder()
		proof         = &block.BlockProof{BlockHeaderHash: test.RandomBytes(32)}
		pbAddFC       = newPBAddFC(test.RandomBytes(32), pbTransferFC.ToProtoBuf(), proof)
		pbTransaction = newPBTransactionOrder(test.RandomBytes(32), test.RandomBytes(32), 555, pbAddFC)
	)
	genericTx, err := toGenericTx(pbTransaction)
	require.NoError(t, err)
	fc, ok := genericTx.(*AddFCWrapper)
	require.True(t, ok)

	require.Equal(t, pbTransaction.SystemId, fc.SystemID())
	require.Equal(t, pbTransaction.UnitId, util.Uint256ToBytes(fc.UnitID()))
	require.Equal(t, pbTransaction.Timeout, fc.Timeout())
	require.Equal(t, pbTransaction.OwnerProof, fc.OwnerProof())

	require.Equal(t, pbAddFC.FeeCreditOwnerCondition, fc.AddFC.FeeCreditOwnerCondition)
	require.True(t, proto.Equal(pbTransferFC.ToProtoBuf(), fc.AddFC.FeeCreditTransfer))
	require.True(t, proto.Equal(proof, fc.AddFC.FeeCreditTransferProof))
}

func TestWrapper_CloseFC(t *testing.T) {
	closeFC := createCloseFCTxOrder()

	require.Equal(t, systemID, closeFC.SystemID())
	require.Equal(t, uint256.NewInt(0).SetBytes(unitID), closeFC.UnitID())
	require.Equal(t, timeout, closeFC.Timeout())
	require.Equal(t, ownerProof, closeFC.OwnerProof())

	closeFCWrapper, ok := closeFC.(*CloseFCWrapper)
	require.True(t, ok)
	fc := closeFCWrapper.CloseFC
	require.Equal(t, amount, fc.Amount)
	require.Equal(t, targetUnitId, fc.TargetUnitId)
	require.Equal(t, nonce, fc.Nonce)
}

func TestTransferFC_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := createTransferFCTxOrder()
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
	require.Equal(t, b.Bytes(), tx.SigBytes())
}

func TestAddFC_SigBytesIsCalculatedCorrectly(t *testing.T) {
	transferFC := createTransferFCTxOrder()
	transferFCProof := &block.BlockProof{BlockHeaderHash: blockHeaderHash}
	tx := createAddFCTxOrder(t, transferFC.ToProtoBuf(), transferFCProof)
	sigBytes := tx.SigBytes()

	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(owner)
	b.Write(transferFC.SigBytes())
	b.Write(transferFCProof.Bytes())
	require.Equal(t, b.Bytes(), sigBytes)
}

func TestCloseFC_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := createCloseFCTxOrder()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(amount))
	b.Write(targetUnitId)
	b.Write(nonce)
	require.Equal(t, b.Bytes(), tx.SigBytes())
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

func newPBAddFC(owner []byte, tx *txsystem.Transaction, proof *block.BlockProof) *AddFCOrder {
	return &AddFCOrder{
		FeeCreditOwnerCondition: owner,
		FeeCreditTransfer:       tx,
		FeeCreditTransferProof:  proof,
	}
}

func newPBCloseFC(amount uint64, targetUnitId []byte, nonce []byte) *CloseFCOrder {
	return &CloseFCOrder{
		Amount:       amount,
		TargetUnitId: targetUnitId,
		Nonce:        nonce,
	}
}

func createTransferFCTxOrder() txsystem.GenericTransaction {
	tx := &txsystem.Transaction{
		SystemId:              systemID,
		TransactionAttributes: new(anypb.Any),
		UnitId:                unitID,
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}
	_ = tx.TransactionAttributes.MarshalFrom(newPBTransferFC(amount, t1, t2, systemID, recordID, nonce, backlink))
	gtx, _ := toGenericTx(tx)
	return gtx
}

func createAddFCTxOrder(t *testing.T, transferFC *txsystem.Transaction, proof *block.BlockProof) txsystem.GenericTransaction {
	tx := &txsystem.Transaction{
		SystemId:              systemID,
		TransactionAttributes: new(anypb.Any),
		UnitId:                unitID,
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}
	err := tx.TransactionAttributes.MarshalFrom(newPBAddFC(owner, transferFC, proof))
	require.NoError(t, err)
	gtx, err := toGenericTx(tx)
	require.NoError(t, err)
	return gtx
}

func createCloseFCTxOrder() txsystem.GenericTransaction {
	tx := &txsystem.Transaction{
		SystemId:              systemID,
		TransactionAttributes: new(anypb.Any),
		UnitId:                unitID,
		Timeout:               timeout,
		OwnerProof:            ownerProof,
	}
	_ = tx.TransactionAttributes.MarshalFrom(newPBCloseFC(amount, targetUnitId, nonce))
	gtx, _ := toGenericTx(tx)
	return gtx
}

func newPBTransactionOrder(id, ownerProof []byte, timeout uint64, attr proto.Message) *txsystem.Transaction {
	to := &txsystem.Transaction{
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

func toGenericTx(tx *txsystem.Transaction) (txsystem.GenericTransaction, error) {
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
	case typeURLAddFCOrder:
		pb := &AddFCOrder{}
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		fcGen, err := toGenericTx(pb.FeeCreditTransfer)
		if err != nil {
			return nil, errors.Wrap(err, "transfer FC wrapping failed")
		}
		fcWrapper, ok := fcGen.(*TransferFCWrapper)
		if !ok {
			return nil, errors.Errorf("transfer FC wrapper is invalid type: %T", fcWrapper)
		}
		return &AddFCWrapper{
			Wrapper:           Wrapper{Transaction: tx},
			AddFC:             pb,
			feeCreditTransfer: fcWrapper,
		}, nil
	case typeURLCloseFCOrder:
		pb := &CloseFCOrder{}
		err := tx.TransactionAttributes.UnmarshalTo(pb)
		if err != nil {
			return nil, err
		}
		return &CloseFCWrapper{
			Wrapper: Wrapper{Transaction: tx},
			CloseFC: pb,
		}, nil
	default:
		return nil, errors.Errorf("unknown transaction type %s", tx.TransactionAttributes.TypeUrl)
	}
}
