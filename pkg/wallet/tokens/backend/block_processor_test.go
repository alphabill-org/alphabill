package backend

import (
	"context"
	"crypto"
	"fmt"
	"io"
	"path/filepath"
	"testing"

	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/stretchr/testify/require"

	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testfc "github.com/alphabill-org/alphabill/internal/txsystem/fc/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/broker"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
)

func Test_blockProcessor_ProcessBlock(t *testing.T) {
	t.Parallel()

	logger, err := log.New(log.DEBUG, io.Discard)
	require.NoError(t, err)

	t.Run("failure to get current block number", func(t *testing.T) {
		expErr := fmt.Errorf("can't get block number")
		bp := &blockProcessor{
			log: logger,
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
			log: logger,
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
		txs, err := tokens.NewTxSystem(
			tokens.WithTrustBase(map[string]abcrypto.Verifier{"test": nil}),
		)
		require.NoError(t, err)

		createNTFTypeTx := randomTx(t, &tokens.CreateNonFungibleTokenTypeAttributes{Symbol: "test"})
		createNTFTypeTx.Payload.Type = tokens.PayloadTypeCreateNFTType
		expErr := fmt.Errorf("can't store tx")
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			store: &mockStorage{
				getFeeCreditBill: getFeeCreditBillFunc,
				setFeeCreditBill: func(fcb *FeeCreditBill, proof *wallet.Proof) error { return verifySetFeeCreditBill(t, fcb) },
				getBlockNumber:   func() (uint64, error) { return 3, nil },
				setBlockNumber:   func(blockNumber uint64) error { return nil },
				// cause processing to fail by failing to store tx
				saveTokenType: func(data *TokenUnitType, proof *wallet.Proof) error {
					return expErr
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &types.Block{
			Transactions:       []*types.TransactionRecord{{TransactionOrder: createNTFTypeTx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
		})
		require.ErrorIs(t, err, expErr)
	})

	t.Run("failure to store new current block number", func(t *testing.T) {
		expErr := fmt.Errorf("can't store block number")
		bp := &blockProcessor{
			log: logger,
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

	logger, err := log.New(log.DEBUG, io.Discard)
	require.NoError(t, err)
	txs, err := tokens.NewTxSystem(
		tokens.WithTrustBase(map[string]abcrypto.Verifier{"test": nil}),
	)
	require.NoError(t, err)
	require.NotNil(t, txs)

	t.Run("token type transactions", func(t *testing.T) {
		icon := &tokens.Icon{Type: "image/svg+xml; encoding=gzip", Data: []byte{1, 2, 3}}
		cases := []struct {
			txAttr interface{}
			kind   Kind
		}{
			{txAttr: &tokens.CreateNonFungibleTokenTypeAttributes{Symbol: "test", Name: "long name of test", Icon: icon}, kind: NonFungible},
			{txAttr: &tokens.CreateFungibleTokenTypeAttributes{Symbol: "test", Name: "long name of test", Icon: icon}, kind: Fungible},
		}

		for n, tc := range cases {
			t.Run(fmt.Sprintf("case [%d] %s", n, tc.kind), func(t *testing.T) {
				tx := randomTx(t, tc.txAttr)
				bp := &blockProcessor{
					log: logger,
					txs: txs,
					store: &mockStorage{
						getFeeCreditBill: getFeeCreditBillFunc,
						setFeeCreditBill: func(fcb *FeeCreditBill, proof *wallet.Proof) error { return verifySetFeeCreditBill(t, fcb) },
						getBlockNumber:   func() (uint64, error) { return 3, nil },
						setBlockNumber:   func(blockNumber uint64) error { return nil },
						saveTokenType: func(data *TokenUnitType, proof *wallet.Proof) error {
							require.NoError(t, err)
							require.EqualValues(t, tx.Hash(crypto.SHA256), data.TxHash)
							require.EqualValues(t, tx.UnitID(), data.ID, "token IDs do not match")
							require.Equal(t, tc.kind, data.Kind, "expected kind %s got %s", tc.kind, data.Kind)
							return nil
						},
					},
				}
				err = bp.ProcessBlock(context.Background(), &types.Block{
					UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
					Transactions:       []*types.TransactionRecord{{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
				})
				require.NoError(t, err)
			})
		}
	})

	t.Run("MintFungibleToken", func(t *testing.T) {
		txAttr := &tokens.MintFungibleTokenAttributes{
			Value:  42,
			TypeID: test.RandomBytes(4),
			Bearer: test.RandomBytes(4),
		}
		tx := randomTx(t, txAttr)
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			notify: func(bearerPredicate []byte, msg broker.Message) {
				require.EqualValues(t, txAttr.Bearer, bearerPredicate)
			},
			store: &mockStorage{
				getBlockNumber:   func() (uint64, error) { return 3, nil },
				setBlockNumber:   func(blockNumber uint64) error { return nil },
				getFeeCreditBill: getFeeCreditBillFunc,
				setFeeCreditBill: func(fcb *FeeCreditBill, proof *wallet.Proof) error { return verifySetFeeCreditBill(t, fcb) },
				getTokenType: func(id TokenTypeID) (*TokenUnitType, error) {
					require.EqualValues(t, txAttr.TypeID, id)
					return &TokenUnitType{ID: id, Kind: Fungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *wallet.Proof) error {
					require.EqualValues(t, tx.Hash(crypto.SHA256), data.TxHash)
					require.EqualValues(t, tx.UnitID(), data.ID)
					require.EqualValues(t, txAttr.TypeID, data.TypeID)
					require.EqualValues(t, txAttr.Bearer, data.Owner)
					require.Equal(t, txAttr.Value, data.Amount)
					require.Equal(t, Fungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
			Transactions:       []*types.TransactionRecord{{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
		})
		require.NoError(t, err)
	})

	t.Run("MintNonFungibleToken", func(t *testing.T) {
		txAttr := &tokens.MintNonFungibleTokenAttributes{
			NFTTypeID: test.RandomBytes(4),
			Bearer:    test.RandomBytes(4),
		}
		tx := randomTx(t, txAttr)
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			notify: func(bearerPredicate []byte, msg broker.Message) {
				require.EqualValues(t, txAttr.Bearer, bearerPredicate)
			},
			store: &mockStorage{
				getBlockNumber:   func() (uint64, error) { return 3, nil },
				setBlockNumber:   func(blockNumber uint64) error { return nil },
				getFeeCreditBill: getFeeCreditBillFunc,
				setFeeCreditBill: func(fcb *FeeCreditBill, proof *wallet.Proof) error { return verifySetFeeCreditBill(t, fcb) },
				getTokenType: func(id TokenTypeID) (*TokenUnitType, error) {
					require.EqualValues(t, txAttr.NFTTypeID, id)
					return &TokenUnitType{ID: id, Kind: NonFungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *wallet.Proof) error {
					require.EqualValues(t, tx.Hash(crypto.SHA256), data.TxHash)
					require.EqualValues(t, tx.UnitID(), data.ID)
					require.EqualValues(t, txAttr.NFTTypeID, data.TypeID)
					require.EqualValues(t, txAttr.Bearer, data.Owner)
					require.Equal(t, NonFungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
			Transactions:       []*types.TransactionRecord{{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
		})
		require.NoError(t, err)
	})

	t.Run("TransferFungibleToken", func(t *testing.T) {
		txAttr := &tokens.TransferFungibleTokenAttributes{
			Value:     50,
			TypeID:    test.RandomBytes(4),
			NewBearer: test.RandomBytes(4),
		}
		tx := randomTx(t, txAttr)
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			notify: func(bearerPredicate []byte, msg broker.Message) {
				require.EqualValues(t, txAttr.NewBearer, bearerPredicate)
			},
			store: &mockStorage{
				getBlockNumber:   func() (uint64, error) { return 3, nil },
				setBlockNumber:   func(blockNumber uint64) error { return nil },
				getFeeCreditBill: getFeeCreditBillFunc,
				setFeeCreditBill: func(fcb *FeeCreditBill, proof *wallet.Proof) error { return verifySetFeeCreditBill(t, fcb) },
				getToken: func(id TokenID) (*TokenUnit, error) {
					return &TokenUnit{ID: id, TypeID: txAttr.TypeID, Amount: txAttr.Value, Kind: Fungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *wallet.Proof) error {
					require.EqualValues(t, tx.Hash(crypto.SHA256), data.TxHash)
					require.EqualValues(t, tx.UnitID(), data.ID)
					require.EqualValues(t, txAttr.TypeID, data.TypeID)
					require.EqualValues(t, txAttr.NewBearer, data.Owner)
					require.EqualValues(t, txAttr.Value, data.Amount)
					require.Equal(t, Fungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
			Transactions:       []*types.TransactionRecord{{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
		})
		require.NoError(t, err)
	})

	t.Run("TransferNonFungibleToken", func(t *testing.T) {
		txAttr := &tokens.TransferNonFungibleTokenAttributes{
			NFTTypeID: test.RandomBytes(4),
			NewBearer: test.RandomBytes(4),
		}
		tx := randomTx(t, txAttr)
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			notify: func(bearerPredicate []byte, msg broker.Message) {
				require.EqualValues(t, txAttr.NewBearer, bearerPredicate)
			},
			store: &mockStorage{
				getBlockNumber:   func() (uint64, error) { return 3, nil },
				setBlockNumber:   func(blockNumber uint64) error { return nil },
				getFeeCreditBill: getFeeCreditBillFunc,
				setFeeCreditBill: func(fcb *FeeCreditBill, proof *wallet.Proof) error { return verifySetFeeCreditBill(t, fcb) },
				getToken: func(id TokenID) (*TokenUnit, error) {
					return &TokenUnit{ID: id, TypeID: txAttr.NFTTypeID, Owner: test.RandomBytes(4), Kind: NonFungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *wallet.Proof) error {
					require.EqualValues(t, tx.Hash(crypto.SHA256), data.TxHash)
					require.EqualValues(t, tx.UnitID(), data.ID)
					require.EqualValues(t, txAttr.NFTTypeID, data.TypeID)
					require.EqualValues(t, txAttr.NewBearer, data.Owner)
					require.Equal(t, NonFungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
			Transactions:       []*types.TransactionRecord{{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
		})
		require.NoError(t, err)
	})

	t.Run("SplitFungibleToken", func(t *testing.T) {
		txAttr := &tokens.SplitFungibleTokenAttributes{
			TargetValue:    42,
			RemainingValue: 8,
			TypeID:         test.RandomBytes(4),
			NewBearer:      test.RandomBytes(4),
		}
		owner := test.RandomBytes(4) // owner of the original token
		saveTokenCalls, notifyCalls := 0, 0
		tx := randomTx(t, txAttr)
		tx.Payload.Type = tokens.PayloadTypeSplitFungibleToken
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			notify: func(bearerPredicate []byte, msg broker.Message) {
				if notifyCalls++; notifyCalls == 1 {
					require.EqualValues(t, owner, bearerPredicate)
				} else {
					require.EqualValues(t, txAttr.NewBearer, bearerPredicate)
				}
			},
			store: &mockStorage{
				getBlockNumber:   func() (uint64, error) { return 3, nil },
				setBlockNumber:   func(blockNumber uint64) error { return nil },
				getFeeCreditBill: getFeeCreditBillFunc,
				setFeeCreditBill: func(fcb *FeeCreditBill, proof *wallet.Proof) error { return verifySetFeeCreditBill(t, fcb) },
				getToken: func(id TokenID) (*TokenUnit, error) {
					return &TokenUnit{ID: id, TypeID: txAttr.TypeID, Amount: 50, Owner: owner, Kind: Fungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *wallet.Proof) error {
					// save token is called twice - first to update existing token and then to save new one
					if saveTokenCalls++; saveTokenCalls == 1 {
						require.EqualValues(t, tx.UnitID(), data.ID)
						require.Equal(t, txAttr.RemainingValue, data.Amount)
						require.EqualValues(t, owner, data.Owner)
					} else {
						require.NotEmpty(t, data.ID, "expected new token to have non-empty ID")
						require.NotEqual(t, tx.UnitID(), data.ID, "new token must have different ID than the original")
						require.EqualValues(t, txAttr.NewBearer, data.Owner)
						require.Equal(t, txAttr.TargetValue, data.Amount)
					}
					require.EqualValues(t, txAttr.TypeID, data.TypeID)
					require.Equal(t, Fungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
			Transactions:       []*types.TransactionRecord{{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
		})
		require.NoError(t, err)
		require.Equal(t, 2, saveTokenCalls)
		require.Equal(t, 2, notifyCalls)
	})

	t.Run("UpdateNonFungibleToken", func(t *testing.T) {
		notifyCalls := 0
		bearer := test.RandomBytes(4)
		txAttr := &tokens.UpdateNonFungibleTokenAttributes{
			Data: test.RandomBytes(4),
		}
		tx := randomTx(t, txAttr)
		tx.Payload.Type = tokens.PayloadTypeUpdateNFT
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			notify: func(bearerPredicate []byte, msg broker.Message) {
				notifyCalls++
				require.EqualValues(t, bearer, bearerPredicate)
			},
			store: &mockStorage{
				getBlockNumber:   func() (uint64, error) { return 3, nil },
				setBlockNumber:   func(blockNumber uint64) error { return nil },
				getFeeCreditBill: getFeeCreditBillFunc,
				setFeeCreditBill: func(fcb *FeeCreditBill, proof *wallet.Proof) error { return verifySetFeeCreditBill(t, fcb) },
				getToken: func(id TokenID) (*TokenUnit, error) {
					return &TokenUnit{ID: id, Owner: bearer, NftData: test.RandomBytes(4), Kind: NonFungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *wallet.Proof) error {
					require.EqualValues(t, tx.UnitID(), data.ID)
					require.EqualValues(t, bearer, data.Owner)
					require.EqualValues(t, txAttr.Data, data.NftData)
					require.Equal(t, NonFungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &types.Block{
			UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 4}},
			Transactions:       []*types.TransactionRecord{{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
		})
		require.NoError(t, err)
		require.Equal(t, 1, notifyCalls)
	})
}

func Test_blockProcessor_ProcessFeeCreditTxs(t *testing.T) {
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
	expectedAddFCHash := addFC.Hash(crypto.SHA256)
	require.Equal(t, expectedAddFCHash, fcb.TxHash)
	require.Equal(t, expectedAddFCHash, fcb.LastAddFCTxHash)

	// when closeFC tx is processed
	closeFC := testfc.NewCloseFC(t,
		testfc.NewCloseFCAttr(testfc.WithCloseFCAmount(10)),
	)
	closeFCTxRecord := &types.TransactionRecord{TransactionOrder: closeFC, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}
	b = &types.Block{
		UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: 5}},
		Transactions:       []*types.TransactionRecord{closeFCTxRecord},
	}
	err = bp.ProcessBlock(context.Background(), b)
	require.NoError(t, err)

	// then fee credit bill value is reduced
	fcb, err = bp.store.GetFeeCreditBill(closeFC.UnitID())
	require.NoError(t, err)
	require.Equal(t, uint256.NewInt(1), uint256.NewInt(0).SetBytes(fcb.Id))
	require.EqualValues(t, 39, fcb.GetValue())
	require.Equal(t, closeFC.Hash(crypto.SHA256), fcb.TxHash)
	require.Equal(t, expectedAddFCHash, fcb.LastAddFCTxHash)

	// and closeFC tx is recorded
	actualCloseFCTxRecord, err := bp.store.GetClosedFeeCredit(fcb.Id)
	require.NoError(t, err)
	require.Equal(t, closeFCTxRecord, actualCloseFCTxRecord)
}

func createBlockProcessor(t *testing.T) *blockProcessor {
	db, err := newBoltStore(filepath.Join(t.TempDir(), "tokens.db"))
	require.NoError(t, err)

	logger, err := log.New(log.DEBUG, io.Discard)
	require.NoError(t, err)

	txSystem, err := tokens.NewTxSystem(tokens.WithTrustBase(map[string]abcrypto.Verifier{"test": nil}))
	require.NoError(t, err)

	return &blockProcessor{log: logger, txs: txSystem, store: db}
}

func getFeeCreditBillFunc(unitID wallet.UnitID) (*FeeCreditBill, error) {
	return &FeeCreditBill{
		Id:              unitID,
		Value:           50,
		TxHash:          []byte{1},
		LastAddFCTxHash: []byte{2},
	}, nil
}

func verifySetFeeCreditBill(t *testing.T, fcb *FeeCreditBill) error {
	// verify fee credit bill value is reduced by 1 on every tx
	require.EqualValues(t, util.Uint256ToBytes(uint256.NewInt(1)), fcb.Id)
	require.EqualValues(t, 49, fcb.Value)
	return nil
}
