package transactions

import (
	"bytes"
	"crypto"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

var (
	moneySystemID   = []byte{0, 0, 0, 0}
	systemID        = []byte{0, 0, 0, 0}
	unitID          = test.NewUnitID(1)
	ownerProof      = []byte{2}
	backlink        = []byte{3}
	nonce           = []byte{4}
	recordID        = []byte{5}
	ownerBytes      = []byte{6}
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
	genericTx, err := NewFeeCreditTx(pbTransaction)
	require.NoError(t, err)
	fc, ok := genericTx.(*TransferFeeCreditWrapper)
	require.True(t, ok)

	require.Equal(t, pbTransaction.SystemId, fc.SystemID())
	require.Equal(t, pbTransaction.UnitId, util.Uint256ToBytes(fc.UnitID()))
	require.Equal(t, pbTransaction.Timeout(), fc.Timeout())
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
	genericTx, err := NewFeeCreditTx(pbTransaction)
	require.NoError(t, err)
	fc, ok := genericTx.(*AddFeeCreditWrapper)
	require.True(t, ok)

	require.Equal(t, pbTransaction.SystemId, fc.SystemID())
	require.Equal(t, pbTransaction.UnitId, util.Uint256ToBytes(fc.UnitID()))
	require.Equal(t, pbTransaction.Timeout(), fc.Timeout())
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

	closeFCWrapper, ok := closeFC.(*CloseFeeCreditWrapper)
	require.True(t, ok)
	fc := closeFCWrapper.CloseFC
	require.Equal(t, amount, fc.Amount)
	require.Equal(t, targetUnitId, fc.TargetUnitId)
	require.Equal(t, nonce, fc.Nonce)
}

func TestWrapper_ReclaimFC(t *testing.T) {
	closeFC := createCloseFCTxOrder()
	closeFCProof := &block.BlockProof{BlockHeaderHash: blockHeaderHash}
	reclaimFC := createReclaimFCTxOrder(t, closeFC.ToProtoBuf(), closeFCProof)

	require.Equal(t, systemID, reclaimFC.SystemID())
	require.Equal(t, uint256.NewInt(0).SetBytes(unitID), reclaimFC.UnitID())
	require.Equal(t, timeout, reclaimFC.Timeout())
	require.Equal(t, ownerProof, reclaimFC.OwnerProof())

	reclaimFCWrapper, ok := reclaimFC.(*ReclaimFeeCreditWrapper)
	require.True(t, ok)
	fc := reclaimFCWrapper.ReclaimFC
	require.True(t, proto.Equal(closeFC.ToProtoBuf(), fc.CloseFeeCreditTransfer))
	require.True(t, proto.Equal(closeFCProof, fc.CloseFeeCreditProof))
	require.Equal(t, backlink, fc.Backlink)
}

func TestTransferFC_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := createTransferFCTxOrder()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
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
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(ownerBytes)
	b.Write(transferFC.SigBytes())
	b.Write(transferFC.OwnerProof())
	b.Write(transferFCProof.Bytes())
	require.Equal(t, b.Bytes(), sigBytes)
}

func TestCloseFC_SigBytesIsCalculatedCorrectly(t *testing.T) {
	tx := createCloseFCTxOrder()
	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(util.Uint64ToBytes(amount))
	b.Write(targetUnitId)
	b.Write(nonce)
	require.Equal(t, b.Bytes(), tx.SigBytes())
}

func TestReclaimFC_SigBytesIsCalculatedCorrectly(t *testing.T) {
	closeFC := createCloseFCTxOrder()
	closeFCProof := &block.BlockProof{BlockHeaderHash: blockHeaderHash}
	tx := createReclaimFCTxOrder(t, closeFC.ToProtoBuf(), closeFCProof)
	sigBytes := tx.SigBytes()

	var b bytes.Buffer
	b.Write(systemID)
	b.Write(unitID)
	b.Write(util.Uint64ToBytes(timeout))
	b.Write(util.Uint64ToBytes(0)) // max fee
	b.Write(nil)                   // fee credit record id
	b.Write(closeFC.SigBytes())
	b.Write(closeFC.OwnerProof())
	b.Write(closeFCProof.Bytes())
	b.Write(backlink)
	require.Equal(t, b.Bytes(), sigBytes)
}

func TestTransferFC_HashIsCalculatedCorrectly(t *testing.T) {
	tx := createTransferFCTxOrder()
	h := crypto.SHA256.New()
	h.Write(systemID)
	h.Write(unitID)
	h.Write(ownerProof)
	h.Write(util.Uint64ToBytes(timeout))
	h.Write(util.Uint64ToBytes(0)) // max fee
	h.Write(nil)                   // fee credit record id
	h.Write(util.Uint64ToBytes(amount))
	h.Write(systemID)
	h.Write(recordID)
	h.Write(util.Uint64ToBytes(t1))
	h.Write(util.Uint64ToBytes(t2))
	h.Write(nonce)
	h.Write(backlink)
	require.Equal(t, h.Sum(nil), tx.Hash(crypto.SHA256))
}

func TestAddFC_HashIsCalculatedCorrectly(t *testing.T) {
	transferFC := createTransferFCTxOrder()
	transferFCProof := &block.BlockProof{BlockHeaderHash: blockHeaderHash}
	tx := createAddFCTxOrder(t, transferFC.ToProtoBuf(), transferFCProof)

	h := crypto.SHA256.New()
	h.Write(systemID)
	h.Write(unitID)
	h.Write(ownerProof)
	h.Write(util.Uint64ToBytes(timeout))
	h.Write(util.Uint64ToBytes(0)) // max fee
	h.Write(nil)                   // fee credit record id
	h.Write(ownerBytes)
	transferFC.AddToHasher(h)
	h.Write(transferFCProof.Bytes())
	require.Equal(t, h.Sum(nil), tx.Hash(crypto.SHA256))
}

func TestCloseFC_HashIsCalculatedCorrectly(t *testing.T) {
	tx := createCloseFCTxOrder()
	h := crypto.SHA256.New()
	h.Write(systemID)
	h.Write(unitID)
	h.Write(ownerProof)
	h.Write(util.Uint64ToBytes(timeout))
	h.Write(util.Uint64ToBytes(0)) // max fee
	h.Write(nil)                   // fee credit record id
	h.Write(util.Uint64ToBytes(amount))
	h.Write(targetUnitId)
	h.Write(nonce)
	require.Equal(t, h.Sum(nil), tx.Hash(crypto.SHA256))
}

func TestReclaimFC_HashIsCalculatedCorrectly(t *testing.T) {
	closeFC := createCloseFCTxOrder()
	closeFCProof := &block.BlockProof{BlockHeaderHash: blockHeaderHash}
	tx := createReclaimFCTxOrder(t, closeFC.ToProtoBuf(), closeFCProof)

	h := crypto.SHA256.New()
	h.Write(systemID)
	h.Write(unitID)
	h.Write(ownerProof)
	h.Write(util.Uint64ToBytes(timeout))
	h.Write(util.Uint64ToBytes(0)) // max fee
	h.Write(nil)                   // fee credit record id
	closeFC.AddToHasher(h)
	h.Write(closeFCProof.Bytes())
	h.Write(backlink)
	require.Equal(t, h.Sum(nil), tx.Hash(crypto.SHA256))
}

func newPBTransferFC(amount, t1, t2 uint64, sysID, recID, nonce, backlink []byte) *TransferFeeCreditAttributes {
	return &TransferFeeCreditAttributes{
		Amount:                 amount,
		TargetSystemIdentifier: sysID,
		TargetRecordId:         recID,
		EarliestAdditionTime:   t1,
		LatestAdditionTime:     t2,
		Nonce:                  nonce,
		Backlink:               backlink,
	}
}

func newPBAddFC(owner []byte, tx *txsystem.Transaction, proof *block.BlockProof) *AddFeeCreditAttributes {
	return &AddFeeCreditAttributes{
		FeeCreditOwnerCondition: owner,
		FeeCreditTransfer:       tx,
		FeeCreditTransferProof:  proof,
	}
}

func newPBCloseFC(amount uint64, targetUnitId []byte, nonce []byte) *CloseFeeCreditAttributes {
	return &CloseFeeCreditAttributes{
		Amount:       amount,
		TargetUnitId: targetUnitId,
		Nonce:        nonce,
	}
}

func newPBReclaimFC(backlink []byte, tx *txsystem.Transaction, proof *block.BlockProof) *ReclaimFeeCreditAttributes {
	return &ReclaimFeeCreditAttributes{
		CloseFeeCreditTransfer: tx,
		CloseFeeCreditProof:    proof,
		Backlink:               backlink,
	}
}

func createTransferFCTxOrder() txsystem.GenericTransaction {
	tx := &txsystem.Transaction{
		SystemId:              systemID,
		TransactionAttributes: new(anypb.Any),
		UnitId:                unitID,
		ClientMetadata:        &txsystem.ClientMetadata{Timeout: timeout},
		OwnerProof:            ownerProof,
	}
	_ = tx.TransactionAttributes.MarshalFrom(newPBTransferFC(amount, t1, t2, systemID, recordID, nonce, backlink))
	gtx, _ := NewFeeCreditTx(tx)
	return gtx
}

func createAddFCTxOrder(t *testing.T, transferFC *txsystem.Transaction, proof *block.BlockProof) txsystem.GenericTransaction {
	tx := &txsystem.Transaction{
		SystemId:              systemID,
		TransactionAttributes: new(anypb.Any),
		UnitId:                unitID,
		ClientMetadata:        &txsystem.ClientMetadata{Timeout: timeout},
		OwnerProof:            ownerProof,
	}
	err := tx.TransactionAttributes.MarshalFrom(newPBAddFC(ownerBytes, transferFC, proof))
	require.NoError(t, err)
	gtx, err := NewFeeCreditTx(tx)
	require.NoError(t, err)
	return gtx
}

func createCloseFCTxOrder() txsystem.GenericTransaction {
	tx := &txsystem.Transaction{
		SystemId:              systemID,
		TransactionAttributes: new(anypb.Any),
		UnitId:                unitID,
		ClientMetadata:        &txsystem.ClientMetadata{Timeout: timeout},
		OwnerProof:            ownerProof,
	}
	_ = tx.TransactionAttributes.MarshalFrom(newPBCloseFC(amount, targetUnitId, nonce))
	gtx, _ := NewFeeCreditTx(tx)
	return gtx
}

func createReclaimFCTxOrder(t *testing.T, closeFC *txsystem.Transaction, closeFCProof *block.BlockProof) txsystem.GenericTransaction {
	tx := &txsystem.Transaction{
		SystemId:              systemID,
		TransactionAttributes: new(anypb.Any),
		UnitId:                unitID,
		ClientMetadata:        &txsystem.ClientMetadata{Timeout: timeout},
		OwnerProof:            ownerProof,
	}
	err := tx.TransactionAttributes.MarshalFrom(newPBReclaimFC(backlink, closeFC, closeFCProof))
	require.NoError(t, err)
	gtx, err := NewFeeCreditTx(tx)
	require.NoError(t, err)
	return gtx
}

func newPBTransactionOrder(id, ownerProof []byte, timeout uint64, attr proto.Message) *txsystem.Transaction {
	to := &txsystem.Transaction{
		SystemId:              systemID,
		UnitId:                id,
		TransactionAttributes: new(anypb.Any),
		ClientMetadata:        &txsystem.ClientMetadata{Timeout: timeout},
		OwnerProof:            ownerProof,
	}
	err := anypb.MarshalFrom(to.TransactionAttributes, attr, proto.MarshalOptions{})
	if err != nil {
		panic(err)
	}
	return to
}
