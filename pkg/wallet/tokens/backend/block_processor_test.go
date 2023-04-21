package twb

import (
	"context"
	"crypto"
	"fmt"
	"io"
	"testing"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/certificates"
	abcrypto "github.com/alphabill-org/alphabill/internal/crypto"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
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
		err := bp.ProcessBlock(context.Background(), &block.Block{})
		require.ErrorIs(t, err, expErr)
	})

	t.Run("blocks are not in correct order, same block twice", func(t *testing.T) {
		bp := &blockProcessor{
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 5, nil },
			},
		}
		err := bp.ProcessBlock(context.Background(), &block.Block{
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 5}},
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
		err := bp.ProcessBlock(context.Background(), &block.Block{
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 4}},
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
		err := bp.ProcessBlock(context.Background(), &block.Block{
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 8}},
		})
		require.NoError(t, err)
		require.Equal(t, 1, callCnt, "expected that setBlockNumber is called once")
	})

	t.Run("failure to process tx", func(t *testing.T) {
		txs, err := tokens.New(
			tokens.WithTrustBase(map[string]abcrypto.Verifier{"test": nil}),
		)
		require.NoError(t, err)

		createNTFTypeTx := randomTx(t, &tokens.CreateNonFungibleTokenTypeAttributes{Symbol: "test"})
		expErr := fmt.Errorf("can't store tx")
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 3, nil },
				setBlockNumber: func(blockNumber uint64) error { return nil },
				// cause protcessing to fail by failing to store tx
				saveTokenType: func(data *TokenUnitType, proof *Proof) error {
					return expErr
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &block.Block{
			Transactions:       []*txsystem.Transaction{createNTFTypeTx},
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 4}},
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
		err := bp.ProcessBlock(context.Background(), &block.Block{UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 4}}})
		require.ErrorIs(t, err, expErr)
	})
}

func Test_blockProcessor_processTx(t *testing.T) {
	t.Parallel()

	logger, err := log.New(log.DEBUG, io.Discard)
	require.NoError(t, err)
	txs, err := tokens.New(
		tokens.WithTrustBase(map[string]abcrypto.Verifier{"test": nil}),
	)
	require.NoError(t, err)
	require.NotNil(t, txs)

	t.Run("token type transactions", func(t *testing.T) {
		icon := &tokens.Icon{Type: "image/svg+xml; encoding=gzip", Data: []byte{1, 2, 3}}
		cases := []struct {
			txAttr protoreflect.ProtoMessage
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
						getBlockNumber: func() (uint64, error) { return 3, nil },
						setBlockNumber: func(blockNumber uint64) error { return nil },
						saveTokenType: func(data *TokenUnitType, proof *Proof) error {
							gtx, err := txs.ConvertTx(tx)
							require.NoError(t, err)
							require.EqualValues(t, gtx.Hash(crypto.SHA256), data.TxHash)
							require.EqualValues(t, tx.UnitId, data.ID, "token IDs do not match")
							require.Equal(t, tc.kind, data.Kind, "expected kind %s got %s", tc.kind, data.Kind)
							return nil
						},
					},
				}
				err = bp.ProcessBlock(context.Background(), &block.Block{
					UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 4}},
					Transactions:       []*txsystem.Transaction{tx},
				})
				require.NoError(t, err)
			})
		}
	})

	t.Run("MintFungibleToken", func(t *testing.T) {
		txAttr := &tokens.MintFungibleTokenAttributes{
			Value:  42,
			Type:   test.RandomBytes(4),
			Bearer: test.RandomBytes(4),
		}
		tx := randomTx(t, txAttr)
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 3, nil },
				setBlockNumber: func(blockNumber uint64) error { return nil },
				getTokenType: func(id TokenTypeID) (*TokenUnitType, error) {
					require.EqualValues(t, txAttr.Type, id)
					return &TokenUnitType{ID: id, Kind: Fungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *Proof) error {
					gtx, err := txs.ConvertTx(tx)
					require.NoError(t, err)
					require.EqualValues(t, gtx.Hash(crypto.SHA256), data.TxHash)
					require.EqualValues(t, tx.UnitId, data.ID)
					require.EqualValues(t, txAttr.Type, data.TypeID)
					require.EqualValues(t, txAttr.Bearer, data.Owner)
					require.Equal(t, txAttr.Value, data.Amount)
					require.Equal(t, Fungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &block.Block{
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 4}},
			Transactions:       []*txsystem.Transaction{tx},
		})
		require.NoError(t, err)
	})

	t.Run("MintNonFungibleToken", func(t *testing.T) {
		txAttr := &tokens.MintNonFungibleTokenAttributes{
			NftType: test.RandomBytes(4),
			Bearer:  test.RandomBytes(4),
		}
		tx := randomTx(t, txAttr)
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 3, nil },
				setBlockNumber: func(blockNumber uint64) error { return nil },
				getTokenType: func(id TokenTypeID) (*TokenUnitType, error) {
					require.EqualValues(t, txAttr.NftType, id)
					return &TokenUnitType{ID: id, Kind: NonFungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *Proof) error {
					gtx, err := txs.ConvertTx(tx)
					require.NoError(t, err)
					require.EqualValues(t, gtx.Hash(crypto.SHA256), data.TxHash)
					require.EqualValues(t, tx.UnitId, data.ID)
					require.EqualValues(t, txAttr.NftType, data.TypeID)
					require.EqualValues(t, txAttr.Bearer, data.Owner)
					require.Equal(t, NonFungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &block.Block{
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 4}},
			Transactions:       []*txsystem.Transaction{tx},
		})
		require.NoError(t, err)
	})

	t.Run("TransferFungibleToken", func(t *testing.T) {
		txAttr := &tokens.TransferFungibleTokenAttributes{
			Value:     50,
			Type:      test.RandomBytes(4),
			NewBearer: test.RandomBytes(4),
		}
		tx := randomTx(t, txAttr)
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 3, nil },
				setBlockNumber: func(blockNumber uint64) error { return nil },
				getToken: func(id TokenID) (*TokenUnit, error) {
					return &TokenUnit{ID: id, TypeID: txAttr.Type, Amount: txAttr.Value, Kind: Fungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *Proof) error {
					gtx, err := txs.ConvertTx(tx)
					require.NoError(t, err)
					require.EqualValues(t, gtx.Hash(crypto.SHA256), data.TxHash)
					require.EqualValues(t, tx.UnitId, data.ID)
					require.EqualValues(t, txAttr.Type, data.TypeID)
					require.EqualValues(t, txAttr.NewBearer, data.Owner)
					require.EqualValues(t, txAttr.Value, data.Amount)
					require.Equal(t, Fungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &block.Block{
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 4}},
			Transactions:       []*txsystem.Transaction{tx},
		})
		require.NoError(t, err)
	})

	t.Run("TransferNonFungibleToken", func(t *testing.T) {
		txAttr := &tokens.TransferNonFungibleTokenAttributes{
			NftType:   test.RandomBytes(4),
			NewBearer: test.RandomBytes(4),
		}
		tx := randomTx(t, txAttr)
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 3, nil },
				setBlockNumber: func(blockNumber uint64) error { return nil },
				getToken: func(id TokenID) (*TokenUnit, error) {
					return &TokenUnit{ID: id, TypeID: txAttr.NftType, Owner: test.RandomBytes(4), Kind: NonFungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *Proof) error {
					gtx, err := txs.ConvertTx(tx)
					require.NoError(t, err)
					require.EqualValues(t, gtx.Hash(crypto.SHA256), data.TxHash)
					require.EqualValues(t, tx.UnitId, data.ID)
					require.EqualValues(t, txAttr.NftType, data.TypeID)
					require.EqualValues(t, txAttr.NewBearer, data.Owner)
					require.Equal(t, NonFungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &block.Block{
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 4}},
			Transactions:       []*txsystem.Transaction{tx},
		})
		require.NoError(t, err)
	})

	t.Run("SplitFungibleToken", func(t *testing.T) {
		txAttr := &tokens.SplitFungibleTokenAttributes{
			TargetValue:    42,
			RemainingValue: 8,
			Type:           test.RandomBytes(4),
			NewBearer:      test.RandomBytes(4),
		}
		owner := test.RandomBytes(4) // owner of the original token
		saveTokenCalls := 0
		tx := randomTx(t, txAttr)
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 3, nil },
				setBlockNumber: func(blockNumber uint64) error { return nil },
				getToken: func(id TokenID) (*TokenUnit, error) {
					return &TokenUnit{ID: id, TypeID: txAttr.Type, Amount: 50, Owner: owner, Kind: Fungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *Proof) error {
					// save token is called twice - first to update existng token and then to save new one
					if saveTokenCalls++; saveTokenCalls == 1 {
						require.EqualValues(t, tx.UnitId, data.ID)
						require.Equal(t, txAttr.RemainingValue, data.Amount)
						require.EqualValues(t, owner, data.Owner)
					} else {
						require.NotEmpty(t, data.ID, "expected new token to have non-empty ID")
						require.NotEqual(t, tx.UnitId, data.ID, "new token must have different ID than the original")
						require.EqualValues(t, txAttr.NewBearer, data.Owner)
						require.Equal(t, txAttr.TargetValue, data.Amount)
					}
					require.EqualValues(t, txAttr.Type, data.TypeID)
					require.Equal(t, Fungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &block.Block{
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 4}},
			Transactions:       []*txsystem.Transaction{tx},
		})
		require.NoError(t, err)
		require.Equal(t, 2, saveTokenCalls)
	})

	t.Run("UpdateNonFungibleToken", func(t *testing.T) {
		txAttr := &tokens.UpdateNonFungibleTokenAttributes{
			Data: test.RandomBytes(4),
		}
		tx := randomTx(t, txAttr)
		bp := &blockProcessor{
			log: logger,
			txs: txs,
			store: &mockStorage{
				getBlockNumber: func() (uint64, error) { return 3, nil },
				setBlockNumber: func(blockNumber uint64) error { return nil },
				getToken: func(id TokenID) (*TokenUnit, error) {
					return &TokenUnit{ID: id, NftData: test.RandomBytes(4), Kind: NonFungible}, nil
				},
				saveToken: func(data *TokenUnit, proof *Proof) error {
					require.EqualValues(t, tx.UnitId, data.ID)
					require.EqualValues(t, txAttr.Data, data.NftData)
					require.Equal(t, NonFungible, data.Kind)
					return nil
				},
			},
		}
		err = bp.ProcessBlock(context.Background(), &block.Block{
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: 4}},
			Transactions:       []*txsystem.Transaction{tx},
		})
		require.NoError(t, err)
	})
}
