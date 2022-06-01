package verifiable_data

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"gitdc.ee.guardtime.com/alphabill/alphabill/pkg/client"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	testfile "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/file"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"
)

type abClientMock struct {
	// record most recent transaction
	tx       *txsystem.Transaction
	fail     bool
	shutdown bool
}

func TestVDClient_Create(t *testing.T) {
	vdClient, err := New(context.Background(), &client.AlphabillClientConfig{Uri: "test"}, false)
	require.NoError(t, err)
	require.NotNil(t, vdClient)
	require.NotNil(t, vdClient.abClient)
}

func TestVdClient_RegisterHash(t *testing.T) {
	vdClient, err := New(context.Background(), &client.AlphabillClientConfig{}, false)
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

func TestVdClient_RegisterHash_LeadingZeroes(t *testing.T) {
	vdClient, err := New(context.Background(), &client.AlphabillClientConfig{}, false)
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
	vdClient, err := New(context.Background(), &client.AlphabillClientConfig{}, false)
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "0x00508D4D37BF6F4D6C63CE4B"
	err = vdClient.RegisterHash(hashHex)
	require.ErrorContains(t, err, "invalid hash length, expected 32 bytes, got 12")
	require.True(t, mock.IsShutdown())
}

func TestVdClient_RegisterHash_TooLong(t *testing.T) {
	vdClient, err := New(context.Background(), &client.AlphabillClientConfig{}, false)
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "0x0067588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(hashHex)
	require.ErrorContains(t, err, "invalid hash length, expected 32 bytes, got 33")
	require.True(t, mock.IsShutdown())
}

func TestVdClient_RegisterHash_NoPrefix(t *testing.T) {
	vdClient, err := New(context.Background(), &client.AlphabillClientConfig{}, false)
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(hashHex)
	require.NoError(t, err)
}

func TestVdClient_RegisterHash_BadHash(t *testing.T) {
	vdClient, err := New(context.Background(), &client.AlphabillClientConfig{}, false)
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810QQ"
	err = vdClient.RegisterHash(hashHex)
	require.ErrorContains(t, err, "invalid byte:")
}

func TestVdClient_RegisterHash_BadResponse(t *testing.T) {
	vdClient, err := New(context.Background(), &client.AlphabillClientConfig{}, false)
	require.NoError(t, err)
	mock := &abClientMock{fail: true}
	vdClient.abClient = mock

	hashHex := "67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(hashHex)
	require.ErrorContains(t, err, "error while submitting the hash: boom")
}

func TestVdClient_RegisterFileHash(t *testing.T) {
	vdClient, err := New(context.Background(), &client.AlphabillClientConfig{}, false)
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
	fmt.Printf("GetBlock(%d)\n", n)
	return nil, nil
}

func (a *abClientMock) GetMaxBlockNumber() (uint64, error) {
	fmt.Println("GetMaxBlockNumber")
	return 0, nil
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
