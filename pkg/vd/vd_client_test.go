package verifiable_data

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"sync"
	"testing"

	test "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils"

	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/wallet/log"

	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/client"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	testfile "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/file"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

type abClientMock struct {
	// record most recent transaction
	tx             *txsystem.Transaction
	maxBlock       uint64
	incrementBlock bool
	fail           bool
	shutdown       bool
	block          *block.Block
}

func testConf() *VDClientConfig {
	return &VDClientConfig{
		AbConf: &client.AlphabillClientConfig{
			Uri: "test",
		},
		WaitBlock:    false,
		BlockTimeout: 1}
}

func TestVDClient_Create(t *testing.T) {
	vdClient, err := New(context.Background(), testConf())
	require.NoError(t, err)
	require.NotNil(t, vdClient)
	require.NotNil(t, vdClient.abClient)
}

func TestVdClient_RegisterHash(t *testing.T) {
	vdClient, err := New(context.Background(), testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(hashHex)
	require.NoError(t, err)
	require.NotNil(t, mock.tx)
	dataHash, err := uint256.FromHex(hashHex)
	require.NoError(t, err)
	require.EqualValues(t, dataHash.Bytes(), mock.tx.UnitId)
}

func TestVdClient_RegisterHash_SyncBlocks(t *testing.T) {
	require.NoError(t, log.InitStdoutLogger())
	conf := testConf()
	conf.WaitBlock = true
	conf.BlockTimeout = 1
	vdClient, err := New(context.Background(), conf)
	require.NoError(t, err)
	mock := &abClientMock{}
	mock.incrementBlock = true
	mock.block = &block.Block{
		BlockNumber: uint64(conf.BlockTimeout + 1),
	}
	vdClient.abClient = mock

	hashHex := "0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = vdClient.RegisterHash(hashHex)
		require.NoError(t, err)
		require.NotNil(t, mock.tx)
		dataHash, err := uint256.FromHex(hashHex)
		require.NoError(t, err)
		require.EqualValues(t, dataHash.Bytes(), mock.tx.UnitId)
		wg.Done()
	}()

	// sync will finish once timeout is reached
	require.Eventually(t, func() bool {
		wg.Wait()
		return true
	}, test.WaitDuration, test.WaitTick)
}

func TestVdClient_RegisterHash_LeadingZeroes(t *testing.T) {
	vdClient, err := New(context.Background(), testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "0x00508D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(hashHex)
	require.NoError(t, err)
	require.NotNil(t, mock.tx)

	bytes, err := hexStringToBytes(hashHex)
	require.NoError(t, err)
	require.EqualValues(t, bytes, mock.tx.UnitId)
}

func TestVdClient_RegisterHash_TooShort(t *testing.T) {
	vdClient, err := New(context.Background(), testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "0x00508D4D37BF6F4D6C63CE4B"
	err = vdClient.RegisterHash(hashHex)
	require.ErrorContains(t, err, "invalid hash length, expected 32 bytes, got 12")
	require.True(t, mock.IsShutdown())
}

func TestVdClient_RegisterHash_TooLong(t *testing.T) {
	vdClient, err := New(context.Background(), testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "0x0067588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(hashHex)
	require.ErrorContains(t, err, "invalid hash length, expected 32 bytes, got 33")
	require.True(t, mock.IsShutdown())
}

func TestVdClient_RegisterHash_NoPrefix(t *testing.T) {
	vdClient, err := New(context.Background(), testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(hashHex)
	require.NoError(t, err)
}

func TestVdClient_RegisterHash_BadHash(t *testing.T) {
	vdClient, err := New(context.Background(), testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810QQ"
	err = vdClient.RegisterHash(hashHex)
	require.ErrorContains(t, err, "invalid byte:")
}

func TestVdClient_RegisterHash_BadResponse(t *testing.T) {
	vdClient, err := New(context.Background(), testConf())
	require.NoError(t, err)
	mock := &abClientMock{fail: true}
	vdClient.abClient = mock

	hashHex := "67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(hashHex)
	require.ErrorContains(t, err, "error while submitting the hash: boom")
}

func TestVdClient_RegisterFileHash(t *testing.T) {
	vdClient, err := New(context.Background(), testConf())
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	fileName := fmt.Sprintf("%s%cab-test-tmp", os.TempDir(), os.PathSeparator)
	content := "alphabill"
	hasher := sha256.New()
	hasher.Write([]byte(content))
	testfile.CreateTempFileWithContent(t, fileName, content)

	err = vdClient.RegisterFileHash(fileName)

	require.NoError(t, err)
	require.NotNil(t, mock.tx)
	require.EqualValues(t, hasher.Sum(nil), mock.tx.UnitId)
	require.Nil(t, mock.tx.TransactionAttributes)
}

func TestVdClient_ListAllBlocksWithTx(t *testing.T) {
	require.NoError(t, log.InitStdoutLogger())
	conf := testConf()
	conf.WaitBlock = true
	vdClient, err := New(context.Background(), conf)
	require.NoError(t, err)
	mock := &abClientMock{}
	mock.maxBlock = 1
	mock.incrementBlock = true
	mock.block = &block.Block{
		BlockNumber: mock.maxBlock + 1,
	}
	vdClient.abClient = mock

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err = vdClient.ListAllBlocksWithTx()
		require.NoError(t, err)
		wg.Done()
	}()

	// sync will finish once timeout is reached
	require.Eventually(t, func() bool {
		wg.Wait()
		return true
	}, test.WaitDuration, test.WaitTick)
}

func (a *abClientMock) SendTransaction(tx *txsystem.Transaction) (*txsystem.TransactionResponse, error) {
	fmt.Printf("Recording incoming tx: %s\n", tx)
	a.tx = tx
	var resp txsystem.TransactionResponse
	if a.fail {
		resp = txsystem.TransactionResponse{
			Ok:      false,
			Message: "boom",
		}
	} else {
		resp = txsystem.TransactionResponse{
			Ok:      true,
			Message: "",
		}
	}
	return &resp, nil
}

func (a *abClientMock) GetBlock(n uint64) (*block.Block, error) {
	if a.block != nil && a.block.BlockNumber == n {
		fmt.Printf("GetBlock(%d) = %s\n", n, a.block)
		return a.block, nil
	}
	fmt.Printf("GetBlock(%d) = nil\n", n)
	return nil, nil
}

func (a *abClientMock) GetMaxBlockNumber() (uint64, error) {
	fmt.Printf("GetMaxBlockNumber: %v\n", a.maxBlock)
	if a.incrementBlock {
		defer func() { a.maxBlock++ }()
	}
	return a.maxBlock, nil
}

func (a *abClientMock) Shutdown() error {
	fmt.Println("Shutdown")
	a.shutdown = true
	return nil
}

func (a *abClientMock) IsShutdown() bool {
	fmt.Println("IsShutdown", a.shutdown)
	return a.shutdown
}