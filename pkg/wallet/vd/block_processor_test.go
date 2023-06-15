package wallet

import (
	"context"
	"crypto"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/vd"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/fxamacker/cbor/v2"
	"github.com/holiman/uint256"
)

func Test_ProofFinder(t *testing.T) {
	tx := randomTx(t, &vd.RegisterDataAttributes{}, test.RandomBytes(32))
	tx.Payload.Type = vd.PayloadTypeRegisterData
	txHash := tx.Hash(crypto.SHA256)

	proofCh := make(chan *wallet.Proof)
	defer close(proofCh)
	proofFinder := NewProofFinder(tx.UnitID(), txHash, proofCh)

	go func() {
		err := proofFinder(context.Background(), &types.Block{
			Transactions:       []*types.TransactionRecord{{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
		})
		require.NoError(t, err)
	}()

	require.Eventually(t, func() bool {
		proof := <-proofCh
		require.NotNil(t, proof)
		return true
	}, test.WaitDuration, test.WaitTick)
}

func Test_blockProcessor_ProcessBlock(t *testing.T) {
	t.Parallel()

	t.Run("failure to get current block number", func(t *testing.T) {
		expErr := fmt.Errorf("can't get block number")
		bp := &blockProcessor{
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 0, expErr },
			},
		}
		err := bp.ProcessBlock(context.Background(), &types.Block{})
		require.ErrorIs(t, err, expErr)
	})

	t.Run("blocks are not in correct order, same block twice", func(t *testing.T) {
		bp := &blockProcessor{
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 5, nil },
			},
		}
		err := bp.ProcessBlock(context.Background(), &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 5}},
		})
		require.EqualError(t, err, `invalid block, received block 5, current wallet block 5`)
	})

	t.Run("blocks are not in correct order, received earlier block", func(t *testing.T) {
		bp := &blockProcessor{
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 5, nil },
			},
		}
		err := bp.ProcessBlock(context.Background(), &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
		})
		require.EqualError(t, err, `invalid block, received block 4, current wallet block 5`)
	})

	t.Run("missing block", func(t *testing.T) {
		// block numbers must not be sequential, gaps might appear as empty block are not stored and sent
		callCnt := 0
		bp := &blockProcessor{
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 5, nil },
				setBlockNumber: func(blockNumber uint64) error {
					callCnt++
					if blockNumber != 8 {
						t.Errorf("expected blockNumber = 8, got %d", blockNumber)
					}
					return nil
				},
			},
		}
		err := bp.ProcessBlock(context.Background(), &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 8}},
		})
		require.NoError(t, err)
		require.Equal(t, 1, callCnt, "expected that setBlockNumber is called once")
	})

	t.Run("failure to process tx", func(t *testing.T) {
		registerDataTx := randomTx(t, &vd.RegisterDataAttributes{}, test.RandomBytes(32))
		registerDataTx.Payload.Type = vd.PayloadTypeRegisterData

		bp := &blockProcessor{
			store: &mockStorage{
				getFeeCreditBill: func(unitID wallet.UnitID) (*FeeCreditBill, error) { return nil, nil },
				setFeeCreditBill: func(fcb *FeeCreditBill) error { return verifySetFeeCreditBill(t, fcb) },
				getBlockNumber:   func() (uint64, error) { return 3, nil },
				setBlockNumber:   func(blockNumber uint64) error { return nil },
			},
		}
		err := bp.ProcessBlock(context.Background(), &types.Block{
			Transactions:       []*types.TransactionRecord{{TransactionOrder: registerDataTx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
		})
		require.ErrorContains(t, err, "fee credit bill not found")
	})

	t.Run("failure to store new current block number", func(t *testing.T) {
		expErr := fmt.Errorf("can't store block number")
		bp := &blockProcessor{
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 3, nil },
				setBlockNumber: func(blockNumber uint64) error { return expErr },
			},
		}
		// no processTx call here as block is empty!
		err := bp.ProcessBlock(context.Background(), &types.Block{UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}}})
		require.ErrorIs(t, err, expErr)
	})
}

func Test_blockProcessor_processTx(t *testing.T) {
	t.Parallel()

	t.Run("RegisterData", func(t *testing.T) {
		tx := randomTx(t, &vd.RegisterDataAttributes{}, test.RandomBytes(32))
		bp := &blockProcessor{
			store: &mockStorage{
				getBlockNumber:   func() (uint64, error) { return 3, nil },
				setBlockNumber:   func(blockNumber uint64) error { return nil },
				getFeeCreditBill: getFeeCreditBillFunc,
				setFeeCreditBill: func(fcb *FeeCreditBill) error { return verifySetFeeCreditBill(t, fcb) },
			},
		}
		err := bp.ProcessBlock(context.Background(), &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
			Transactions:       []*types.TransactionRecord{{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
		})
		require.NoError(t, err)
	})

	t.Run("FeeCreditTxs", func(t *testing.T) {
		bp := createBlockProcessor(t)

		signer, err := abcrypto.NewInMemorySecp256K1Signer()
		require.NoError(t, err)

		// when addFC tx is processed
		addFC := testfc.NewAddFC(t, signer, nil)
		b := &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
			Transactions:       []*types.TransactionRecord{{TransactionOrder: addFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
		}
		err = bp.ProcessBlock(context.Background(), b)
		require.NoError(t, err)

		// then fee credit bill is saved
		fcb, err := bp.store.GetFeeCreditBill(addFC.UnitID())
		require.NoError(t, err)
		require.Equal(t, uint256.NewInt(1), uint256.NewInt(0).SetBytes(fcb.Id))
		require.EqualValues(t, 49, fcb.GetValue())
		addFCTxHash := addFC.Hash(crypto.SHA256)
		require.Equal(t, addFCTxHash, fcb.AddFCTxHash)
		require.Equal(t, addFCTxHash, fcb.TxHash)

		// when closeFC tx is processed
		closeFC := testfc.NewCloseFC(t,
			testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(10)),
		)
		b = &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 5}},
			Transactions:       []*types.TransactionRecord{{TransactionOrder: closeFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
		}
		err = bp.ProcessBlock(context.Background(), b)
		require.NoError(t, err)

		// then fee credit bill value is reduced
		fcb, err = bp.store.GetFeeCreditBill(closeFC.UnitID())
		require.NoError(t, err)
		require.Equal(t, uint256.NewInt(1), uint256.NewInt(0).SetBytes(fcb.Id))
		require.EqualValues(t, 39, fcb.GetValue())
		require.EqualValues(t, addFCTxHash, fcb.GetAddFCTxHash())
		require.Equal(t, closeFC.Hash(crypto.SHA256), fcb.TxHash)
	})
}

func createBlockProcessor(t *testing.T) *blockProcessor {
	db, err := newBoltStore(filepath.Join(t.TempDir(), "vd.db"))
	require.NoError(t, err)
	return &blockProcessor{store: db}
}

func getFeeCreditBillFunc(unitID wallet.UnitID) (*FeeCreditBill, error) {
	return &FeeCreditBill{
		Id:          unitID,
		Value:       50,
		TxHash:      []byte{1},
		AddFCTxHash: []byte{2},
	}, nil
}

func verifySetFeeCreditBill(t *testing.T, fcb *FeeCreditBill) error {
	// verify fee credit bill value is reduced by 1 on every tx
	require.EqualValues(t, util.Uint256ToBytes(uint256.NewInt(1)), fcb.Id)
	require.EqualValues(t, 49, fcb.Value)
	return nil
}

func randomTx(t *testing.T, attr interface{}, unitID types.UnitID) *types.TransactionOrder {
	t.Helper()
	attrBytes, err := cbor.Marshal(attr)
	require.NoError(t, err, "failed to marshal tx attributes")

	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:       vd.DefaultSystemIdentifier,
			UnitID:         unitID,
			Attributes:     attrBytes,
			ClientMetadata: &types.ClientMetadata{Timeout: 10, FeeCreditRecordID: util.Uint64ToBytes32(1)},
		},
		FeeProof: test.RandomBytes(3),
	}
	return tx
}

type mockStorage struct {
	getBlockNumber   func() (uint64, error)
	setBlockNumber   func(blockNumber uint64) error
	getFeeCreditBill func(unitID wallet.UnitID) (*FeeCreditBill, error)
	setFeeCreditBill func(fcb *FeeCreditBill) error
}

func (ms *mockStorage) Close() error { return nil }

func (ms *mockStorage) GetBlockNumber() (uint64, error) {
	if ms.getBlockNumber != nil {
		return ms.getBlockNumber()
	}
	return 0, fmt.Errorf("unexpected GetBlockNumber call")
}

func (ms *mockStorage) SetBlockNumber(blockNumber uint64) error {
	if ms.setBlockNumber != nil {
		return ms.setBlockNumber(blockNumber)
	}
	return fmt.Errorf("unexpected SetBlockNumber(%d) call", blockNumber)
}

func (ms *mockStorage) GetFeeCreditBill(unitID wallet.UnitID) (*FeeCreditBill, error) {
	if ms.getFeeCreditBill != nil {
		return ms.getFeeCreditBill(unitID)
	}
	return nil, fmt.Errorf("unexpected GetFeeCredit call")
}

func (ms *mockStorage) SetFeeCreditBill(fcb *FeeCreditBill) error {
	if ms.setFeeCreditBill != nil {
		return ms.setFeeCreditBill(fcb)
	}
	return fmt.Errorf("unexpected SetFeeCreditBill(%X) call", fcb.Id)
}
