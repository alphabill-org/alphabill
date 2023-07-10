package backend

import (
	"bytes"
	"context"
	"crypto"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alphabill-org/alphabill/internal/hash"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	"github.com/alphabill-org/alphabill/internal/script"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/alphabill-org/alphabill/pkg/wallet"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func Test_Run(t *testing.T) {
	t.Parallel()

	t.Run("failure to get storage", func(t *testing.T) {
		cfg := &mockCfg{} // no db cfg assigned, should cause error before subprocesses start
		err := Run(context.Background(), cfg)
		require.EqualError(t, err, `failed to get storage: neither db file name nor mock is assigned`)
	})

	loggerForTest := func(t *testing.T) (log.Logger, *bytes.Buffer) {
		buf := bytes.NewBuffer(nil)
		l, err := log.New(log.DEBUG, buf)
		require.NoError(t, err)
		return l, buf
	}

	t.Run("failure to get starting block number from storage", func(t *testing.T) {
		// as the sync subprocess runs in retry loop we check that expected error is
		// logged and then stop the service by cancelling the ctx
		ctx, cancel := context.WithCancel(context.Background())
		logger, logBuf := loggerForTest(t)
		expErr := fmt.Errorf("can't get block number")
		cfg := &mockCfg{
			db: &mockStorage{
				getBlockNumber: func() (uint64, error) {
					cancel() // stop the service
					return 0, expErr
				},
			},
			abc: &mockABClient{},
			log: logger,
		}
		require.NoError(t, cfg.initListener())

		err := Run(ctx, cfg)
		require.ErrorIs(t, err, context.Canceled)
		require.Contains(t, logBuf.String(), expErr.Error())
	})

	t.Run("failure to fetch new blocks from AB", func(t *testing.T) {
		// as the sync subprocess runs in retry loop we check that expected error is
		// logged and then stop the service by cancelling the ctx
		ctx, cancel := context.WithCancel(context.Background())
		logger, logBuf := loggerForTest(t)
		expErr := fmt.Errorf("AB doesn't return blocks right now")
		cfg := &mockCfg{
			dbFile: filepath.Join(t.TempDir(), "tokens.db"),
			abc: &mockABClient{
				getBlocks: func(ctx context.Context, blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
					cancel() // stop the service
					return nil, expErr
				},
			},
			log: logger,
		}
		require.NoError(t, cfg.initListener())

		err := Run(ctx, cfg)
		require.ErrorIs(t, err, context.Canceled)
		require.Contains(t, logBuf.String(), expErr.Error())
	})

	t.Run("cancelling ctx stops the backend", func(t *testing.T) {
		logger, logBuf := loggerForTest(t)
		syncing := make(chan struct{})
		cfg := &mockCfg{
			dbFile: filepath.Join(t.TempDir(), "tokens.db"),
			abc: &mockABClient{
				getBlocks: func(ctx context.Context, blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
					select {
					case syncing <- struct{}{}:
					default:
					}
					// signal "no new blocks" so sync should sit idle
					return &alphabill.GetBlocksResponse{MaxBlockNumber: blockNumber, BatchMaxBlockNumber: blockNumber}, nil
				},
			},
			log: logger,
		}
		require.NoError(t, cfg.initListener())

		ctx, cancel := context.WithCancel(context.Background())
		srvErr := make(chan error, 1)
		go func() {
			srvErr <- Run(ctx, cfg)
		}()

		select {
		case <-syncing:
		case <-time.After(time.Second):
			t.Error("backend didn't start syncing within timeout")
		}

		// stop the backend
		cancel()
		select {
		case <-time.After(time.Second):
			t.Error("Run didn't return within timeout")
		case err := <-srvErr:
			require.ErrorIs(t, err, context.Canceled)
		}

		require.Contains(t, logBuf.String(), `synchronizing blocks returned error: context canceled`)
	})
}

func Test_Run_API(t *testing.T) {
	t.Parallel()

	logger, err := log.New(log.DEBUG, nil)
	require.NoError(t, err)

	var currentRoundNumber atomic.Uint64
	syncing := make(chan *types.TransactionOrder, 1)
	boltStore, err := newBoltStore(filepath.Join(t.TempDir(), "tokens.db"))
	require.NoError(t, err)

	// add fee credit for user
	err = boltStore.SetFeeCreditBill(&FeeCreditBill{
		Id:          util.Uint64ToBytes32(1),
		Value:       10000000,
		TxHash:      []byte{1},
		AddFCTxHash: []byte{2},
	}, nil)
	require.NoError(t, err)

	// only AB backend is mocked, rest is "real"
	cfg := &mockCfg{
		log: logger,
		db:  boltStore,
		abc: &mockABClient{
			sendTransaction: func(ctx context.Context, tx *types.TransactionOrder) error {
				syncing <- tx
				return nil
			},
			getBlocks: func(ctx context.Context, blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
				select {
				case tx := <-syncing:
					rn := currentRoundNumber.Add(1)
					block := &types.Block{
						Transactions:       []*types.TransactionRecord{{TransactionOrder: tx, ServerMetadata: &types.ServerMetadata{ActualFee: 1}}},
						UnicityCertificate: &types.UnicityCertificate{InputRecord: &types.InputRecord{RoundNumber: rn}},
					}
					blockBytes, err := cbor.Marshal(block)
					require.NoError(t, err)
					return &alphabill.GetBlocksResponse{
						MaxBlockNumber:      blockNumber,
						BatchMaxBlockNumber: rn,
						MaxRoundNumber:      rn,
						Blocks:              [][]byte{blockBytes},
					}, nil
				default:
					// signal "no new blocks"
					return &alphabill.GetBlocksResponse{MaxBlockNumber: blockNumber, MaxRoundNumber: blockNumber, BatchMaxBlockNumber: blockNumber}, nil
				}
			},
			roundNumber: func(ctx context.Context) (uint64, error) {
				return currentRoundNumber.Load(), nil
			},
		},
	}
	require.NoError(t, cfg.initListener())

	doGet := func(path string, code int, data any) error {
		rsp, err := http.Get(cfg.HttpURL(path))
		if err != nil {
			return fmt.Errorf("request to %q failed: %w", path, err)
		}
		return decodeResponse(t, rsp, code, data)
	}

	waitForRoundNumberToBeStored := func(num uint64, timeout time.Duration) {
		t.Helper()
		for st := time.Now(); ; {
			rn, err := boltStore.GetBlockNumber()
			if err != nil {
				t.Logf("failed to read block number from storage: %v", err)
			}
			if rn == num {
				break
			}
			if et := time.Since(st); et > timeout {
				t.Fatalf("%s has elapsed but still don't see round-number %d", et, num)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	// launch the backend
	ctx, cancel := context.WithCancel(context.Background())
	srvDone := make(chan struct{})
	go func() {
		close(srvDone)
		if err := Run(ctx, cfg); err == nil || !errors.Is(err, context.Canceled) {
			t.Errorf("Run exited with unexpected error: %v", err)
		}
	}()

	var rn RoundNumberResponse
	require.NoError(t, doGet("/round-number", http.StatusOK, &rn))
	require.EqualValues(t, 0, rn.RoundNumber, "expected that system starts with round-number 0")

	// trigger block sync from (mocked) AB with an CreateNonFungibleTokenType tx
	createNTFTypeTx := randomTx(t, &tokens.CreateNonFungibleTokenTypeAttributes{Symbol: "test"})
	createNTFTypeTx.Payload.Type = tokens.PayloadTypeCreateNFTType
	select {
	case syncing <- createNTFTypeTx:
	case <-time.After(2 * time.Second):
		t.Error("backend didn't start syncing within timeout")
	}

	// syncing with mocked AB backend should have us now on round-number 1
	waitForRoundNumberToBeStored(1, 1500*time.Millisecond)
	attrs := &tokens.CreateNonFungibleTokenTypeAttributes{}
	require.NoError(t, createNTFTypeTx.UnmarshalAttributes(attrs))
	// we synced NTF token type from backend, check that it is returned:
	// first convert the txsystem.Transaction to the type we have in indexing backend...
	cnfttt := &TokenUnitType{
		Kind:                     NonFungible,
		ID:                       TokenTypeID(createNTFTypeTx.Payload.UnitID),
		ParentTypeID:             attrs.ParentTypeID,
		Symbol:                   attrs.Symbol,
		Name:                     attrs.Name,
		Icon:                     attrs.Icon,
		SubTypeCreationPredicate: attrs.SubTypeCreationPredicate,
		TokenCreationPredicate:   attrs.TokenCreationPredicate,
		InvariantPredicate:       attrs.InvariantPredicate,
		NftDataUpdatePredicate:   attrs.DataUpdatePredicate,
		TxHash:                   createNTFTypeTx.Hash(crypto.SHA256),
	}
	//...and check do we get it back via API
	// get all kind of types
	var typesData []*TokenUnitType
	require.NoError(t, doGet("/kinds/all/types", http.StatusOK, &typesData))
	require.ElementsMatch(t, typesData, []*TokenUnitType{cnfttt})
	// there shouldn't be any fungible token types
	typesData = nil
	require.NoError(t, doGet("/kinds/fungible/types", http.StatusOK, &typesData))
	require.Empty(t, typesData)
	// ask for nft types only
	typesData = nil
	require.NoError(t, doGet("/kinds/nft/types", http.StatusOK, &typesData))
	require.ElementsMatch(t, typesData, []*TokenUnitType{cnfttt})

	// post a tx to mint NFT with the existing type
	ownerID := test.RandomBytes(33)
	pubKeyHex := hexutil.Encode(ownerID)
	tx := randomTx(t,
		&tokens.MintNonFungibleTokenAttributes{
			Bearer:    script.PredicatePayToPublicKeyHashDefault(hash.Sum256(ownerID)),
			NFTTypeID: createNTFTypeTx.Payload.UnitID,
		})
	tx.Payload.Type = tokens.PayloadTypeMintNFT
	message, err := cbor.Marshal(&wallet.Transactions{Transactions: []*types.TransactionOrder{tx}})
	require.NoError(t, err)
	require.NotEmpty(t, message)

	rsp, err := http.Post(cfg.HttpURL("/transactions/"+pubKeyHex), "", bytes.NewBuffer(message))
	require.NoError(t, err)
	require.NotNil(t, rsp)
	data := map[string]string{}
	require.NoError(t, decodeResponse(t, rsp, http.StatusAccepted, &data))
	require.Empty(t, data)

	// syncing with mocked AB backend should have us now on round-number 2
	waitForRoundNumberToBeStored(2, 1500*time.Millisecond)

	// read back the token we minted
	var tokens []*TokenUnit
	require.NoError(t, doGet("/kinds/nft/owners/"+pubKeyHex+"/tokens", http.StatusOK, &tokens))
	require.Len(t, tokens, 1, "expected that one token is found")
	// should get no fungible tokens
	require.NoError(t, doGet("/kinds/fungible/owners/"+pubKeyHex+"/tokens", http.StatusOK, &tokens))
	require.Empty(t, tokens, "expected no fungible tokens to be found")

	// stop the backend
	cancel()
	select {
	case <-time.After(time.Second):
		t.Error("Run didn't return within timeout")
	case <-srvDone:
	}
}
