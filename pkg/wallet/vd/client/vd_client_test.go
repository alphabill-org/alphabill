package client

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/alphabill-org/alphabill/internal/certificates"
	"github.com/alphabill-org/alphabill/internal/util"

	"github.com/alphabill-org/alphabill/internal/block"
	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	testfile "github.com/alphabill-org/alphabill/internal/testutils/file"
	"github.com/alphabill-org/alphabill/internal/txsystem"
	"github.com/alphabill-org/alphabill/pkg/wallet/log"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

type abClientMock struct {
	// record most recent transaction
	tx             *txsystem.Transaction
	maxBlock       uint64
	maxRoundNumber uint64
	incrementBlock bool
	fail           bool
	shutdown       bool
	block          func(nr uint64) *block.Block
}

func testConf() *VDClientConfig {
	return &VDClientConfig{
		VDNodeURL:    "test",
		MoneyNodeURL: "test",
		WaitBlock:    false,
		BlockTimeout: 1}
}

func TestVDClient_Create(t *testing.T) {
	vdClient, err := New(testConf())
	require.NoError(t, err)
	require.NotNil(t, vdClient)
	require.NotNil(t, vdClient.moneyNodeClient)
	require.NotNil(t, vdClient.vdNodeClient)
}

func TestVdClient_RegisterHash(t *testing.T) {
	vdClient, err := New(testConf())
	require.NoError(t, err)
	mock := &abClientMock{maxBlock: 1, maxRoundNumber: 100}
	vdClient.moneyNodeClient = mock
	vdClient.vdNodeClient = mock

	hashHex := "0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(context.Background(), hashHex)
	require.NoError(t, err)
	require.NotNil(t, mock.tx)
	dataHash, err := uint256.FromHex(hashHex)
	require.NoError(t, err)
	require.EqualValues(t, util.Uint256ToBytes(dataHash), mock.tx.UnitId)
	require.Equal(t, mock.maxRoundNumber+vdClient.timeoutDelta, mock.tx.Timeout())
}

func TestVdClient_RegisterHash_SyncBlocks(t *testing.T) {
	require.NoError(t, log.InitStdoutLogger(log.INFO))
	conf := testConf()
	conf.WaitBlock = true
	conf.BlockTimeout = 5
	callbackCalled := false
	conf.OnBlockCallback = func(b *VDBlock) {
		require.NotNil(t, b)
		require.Equal(t, conf.BlockTimeout-1, b.GetBlockNumber())
		callbackCalled = true
	}
	vdClient, err := New(conf)
	require.NoError(t, err)
	mock := &abClientMock{}
	mock.incrementBlock = true
	mock.block = func(nr uint64) *block.Block {
		var txs []*txsystem.Transaction
		if nr == conf.BlockTimeout-1 && mock.tx != nil {
			txs = append(txs, mock.tx)
		}
		return &block.Block{
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: nr}},
			Transactions:       txs,
		}
	}

	vdClient.moneyNodeClient = mock
	vdClient.vdNodeClient = mock

	hashHex := "0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = vdClient.RegisterHash(context.Background(), hashHex)
		require.NoError(t, err)
		require.NotNil(t, mock.tx)
		dataHash, err := uint256.FromHex(hashHex)
		require.NoError(t, err)
		require.EqualValues(t, util.Uint256ToBytes(dataHash), mock.tx.UnitId)
		wg.Done()
	}()

	// sync will finish once timeout is reached
	require.Eventually(t, func() bool {
		wg.Wait()
		return true
	}, test.WaitDuration, test.WaitTick)

	require.True(t, callbackCalled)
}

func TestVdClient_RegisterHash_LeadingZeroes(t *testing.T) {
	vdClient, err := New(testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.moneyNodeClient = mock
	vdClient.vdNodeClient = mock

	hashHex := "0x00508D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(context.Background(), hashHex)
	require.NoError(t, err)
	require.NotNil(t, mock.tx)

	bytes, err := hexStringToBytes(hashHex)
	require.NoError(t, err)
	require.EqualValues(t, bytes, mock.tx.UnitId)
}

func TestVdClient_RegisterHash_TooShort(t *testing.T) {
	vdClient, err := New(testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.moneyNodeClient = mock
	vdClient.vdNodeClient = mock

	hashHex := "0x00508D4D37BF6F4D6C63CE4B"
	err = vdClient.RegisterHash(context.Background(), hashHex)
	vdClient.Close()

	require.ErrorContains(t, err, "invalid hash length, expected 32 bytes, got 12")
	require.True(t, mock.IsClosed())
}

func TestVdClient_RegisterHash_TooLong(t *testing.T) {
	vdClient, err := New(testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.moneyNodeClient = mock
	vdClient.vdNodeClient = mock

	hashHex := "0x0067588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(context.Background(), hashHex)
	vdClient.Close()

	require.ErrorContains(t, err, "invalid hash length, expected 32 bytes, got 33")
	require.True(t, mock.IsClosed())
}

func TestVdClient_RegisterHash_NoPrefix(t *testing.T) {
	vdClient, err := New(testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.moneyNodeClient = mock
	vdClient.vdNodeClient = mock

	hashHex := "67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(context.Background(), hashHex)
	require.NoError(t, err)
}

func TestVdClient_RegisterHash_BadHash(t *testing.T) {
	vdClient, err := New(testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.moneyNodeClient = mock
	vdClient.vdNodeClient = mock

	hashHex := "0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810QQ"
	err = vdClient.RegisterHash(context.Background(), hashHex)
	require.ErrorContains(t, err, "invalid byte:")
}

func TestVdClient_RegisterHash_BadResponse(t *testing.T) {
	vdClient, err := New(testConf())
	require.NoError(t, err)
	mock := &abClientMock{fail: true}
	vdClient.moneyNodeClient = mock
	vdClient.vdNodeClient = mock

	hashHex := "67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(context.Background(), hashHex)
	require.ErrorContains(t, err, "error while submitting the hash: boom")
}

func TestVdClient_RegisterFileHash(t *testing.T) {
	vdClient, err := New(testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.moneyNodeClient = mock
	vdClient.vdNodeClient = mock

	fileName := fmt.Sprintf("%s%cab-test-tmp", os.TempDir(), os.PathSeparator)
	content := "alphabill"
	hasher := sha256.New()
	hasher.Write([]byte(content))
	testfile.CreateTempFileWithContent(t, fileName, content)

	err = vdClient.RegisterFileHash(context.Background(), fileName)

	require.NoError(t, err)
	require.NotNil(t, mock.tx)
	require.EqualValues(t, hasher.Sum(nil), mock.tx.UnitId)
	require.Nil(t, mock.tx.TransactionAttributes)
}

func TestVdClient_ListAllBlocksWithTx(t *testing.T) {
	require.NoError(t, log.InitStdoutLogger(log.INFO))
	conf := testConf()
	conf.WaitBlock = true
	vdClient, err := New(conf)
	require.NoError(t, err)
	mock := &abClientMock{}
	mock.maxBlock = 1
	mock.incrementBlock = true
	mock.block = func(nr uint64) *block.Block {
		return &block.Block{
			UnicityCertificate: &certificates.UnicityCertificate{InputRecord: &certificates.InputRecord{RoundNumber: nr}},
		}
	}

	vdClient.moneyNodeClient = mock
	vdClient.vdNodeClient = mock

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = vdClient.ListAllBlocksWithTx(context.Background())
		require.NoError(t, err)
		wg.Done()
	}()

	// sync will finish once timeout is reached
	require.Eventually(t, func() bool {
		wg.Wait()
		return true
	}, test.WaitDuration, test.WaitTick)
}

func (a *abClientMock) SendTransaction(ctx context.Context, tx *txsystem.Transaction) error {
	a.tx = tx
	if a.fail {
		return fmt.Errorf("boom")
	}
	return nil
}


func (a *abClientMock) SendTransactionWithRetry(ctx context.Context, tx *txsystem.Transaction, maxTries int) error {
	return a.SendTransaction(ctx, tx)
}

func (a *abClientMock) GetBlock(ctx context.Context, n uint64) (*block.Block, error) {
	if a.block != nil {
		bl := a.block(n)
		fmt.Printf("GetBlock(%d) = %s\n", n, bl)
		return bl, nil
	}
	fmt.Printf("GetBlock(%d) = nil\n", n)
	return nil, nil
}

func (a *abClientMock) GetBlocks(ctx context.Context, blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
	return &alphabill.GetBlocksResponse{MaxBlockNumber: a.maxBlock, Blocks: []*block.Block{a.block(blockNumber)}}, nil
}

func (a *abClientMock) GetRoundNumber(ctx context.Context) (uint64, error) {
	if a.incrementBlock {
		defer func() {
			a.maxBlock++
			if a.maxBlock > a.maxRoundNumber {
				a.maxRoundNumber = a.maxBlock
			}
		}()
	}
	return a.maxRoundNumber, nil
}

func (a *abClientMock) Close() error {
	fmt.Println("Shutdown")
	a.shutdown = true
	return nil
}

func (a *abClientMock) IsClosed() bool {
	fmt.Println("IsShutdown", a.shutdown)
	return a.shutdown
}
