package verifiable_data

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	testfile "gitdc.ee.guardtime.com/alphabill/alphabill/internal/testutils/file"

	"github.com/holiman/uint256"

	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/block"
	"gitdc.ee.guardtime.com/alphabill/alphabill/internal/txsystem"

	"github.com/stretchr/testify/require"
)

type abClientMock struct {
	// record most recent transaction
	tx *txsystem.Transaction
}

func TestVDClient_Create(t *testing.T) {
	vdClient, err := New(context.Background(), &AlphabillClientConfig{Uri: "test"})
	require.NoError(t, err)
	require.NotNil(t, vdClient)
	require.NotNil(t, vdClient.abClient)
}

func TestVdClient_RegisterHash(t *testing.T) {
	vdClient, err := New(context.Background(), &AlphabillClientConfig{})
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

func TestVdClient_RegisterHash_NoPrefix(t *testing.T) {
	vdClient, err := New(context.Background(), &AlphabillClientConfig{})
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810DD"
	err = vdClient.RegisterHash(hashHex)
	require.ErrorContains(t, err, "hex string without 0x prefix")
}

func TestVdClient_RegisterHash_BadHash(t *testing.T) {
	vdClient, err := New(context.Background(), &AlphabillClientConfig{})
	require.NoError(t, err)
	mock := &abClientMock{}
	vdClient.abClient = mock

	hashHex := "0x67588D4D37BF6F4D6C63CE4BDA38DA2B869012B1BC131DB07AA1D2B5BFD810QQ"
	err = vdClient.RegisterHash(hashHex)
	require.ErrorContains(t, err, "invalid hex string")
}

func TestVdClient_RegisterFileHash(t *testing.T) {
	vdClient, err := New(context.Background(), &AlphabillClientConfig{})
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
	return nil, nil
}

func (a *abClientMock) GetBlock(_ uint64) (*block.Block, error) {
	return nil, nil
}

func (a *abClientMock) GetMaxBlockNumber() (uint64, error) {
	return 0, nil
}

func (a *abClientMock) Shutdown() {
}

func (a *abClientMock) IsShutdown() bool {
	return false
}
