package cmd

import (
	"bytes"
	"crypto"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm"
	"github.com/alphabill-org/alphabill/internal/txsystem/evm/api"
	"github.com/alphabill-org/alphabill/internal/types"
	"github.com/alphabill-org/alphabill/internal/util"
	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"
)

func Test_evmCmdDeploy_error_cases(t *testing.T) {
	homedir := createNewTestWallet(t)
	// balance is returned by EVM in wei 10^-18
	mockServer, addr := mockClientCalls(&clientMockConf{balance: "15000000000000000000", backlink: make([]byte, 32)})
	defer mockServer.Close()
	_, err := execCommand(homedir, "evm deploy --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"data\", \"max-gas\" not set")
	_, err = execCommand(homedir, "evm deploy --max-gas 10000 --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"data\" not set")
	_, err = execCommand(homedir, "evm deploy --data accbdeef --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"max-gas\" not set")
	// smart contract code too big
	code := hex.EncodeToString(make([]byte, scSizeLimit24Kb+1))
	_, err = execCommand(homedir, "evm deploy --max-gas 10000 --data "+code+" --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "contract code too big, maximum size is 24Kb")
	_, err = execCommand(homedir, "evm deploy --max-gas 1000 --data accbxdeef --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "failed to read 'data' parameter: hex decode error: encoding/hex: invalid byte: U+0078 'x'")
	_, err = execCommand(homedir, "evm deploy --max-gas abba --data accbdeef --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "invalid argument \"abba\" for \"--max-gas\"")
}

func Test_evmCmdDeploy_ok(t *testing.T) {
	homedir := createNewTestWallet(t)
	evmDetails := evm.ProcessingDetails{
		ErrorDetails: "something went wrong",
	}
	detailBytes, err := cbor.Marshal(evmDetails)
	require.NoError(t, err)
	mockConf := &clientMockConf{
		round:    3,
		balance:  "15000000000000000000", // balance is returned by EVM in wei 10^-18
		backlink: make([]byte, 32),
		nonce:    1,
		serverMeta: &types.ServerMetadata{
			ActualFee:         21000,
			TargetUnits:       []types.UnitID{test.RandomBytes(20)},
			SuccessIndicator:  types.TxStatusFailed,
			ProcessingDetails: detailBytes,
		},
	}
	mockServer, addr := mockClientCalls(mockConf)
	defer mockServer.Close()
	stdout, err := execCommand(homedir, "evm deploy --max-gas 10000 --data 9021ACFE0102 --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout,
		"Evm transaction failed: something went wrong",
		"Evm transaction processing fee: 0.000'210'00")
	// verify tx order
	require.Equal(t, "evm", mockConf.receivedTx.PayloadType())
	evmAttributes := &evm.TxAttributes{}
	require.NoError(t, mockConf.receivedTx.UnmarshalAttributes(evmAttributes))
	// verify attributes set by cli cmd
	data, err := hex.DecodeString("9021ACFE0102")
	require.NoError(t, err)
	require.NotNil(t, evmAttributes.From)
	require.Nil(t, evmAttributes.To)
	//value is currently hardcoded as 0
	require.Equal(t, big.NewInt(0), evmAttributes.Value)
	require.EqualValues(t, data, evmAttributes.Data)
	require.EqualValues(t, 10000, evmAttributes.Gas)
	// nonce is read from evm
	require.EqualValues(t, 1, evmAttributes.Nonce)
}

func Test_evmCmdExecute_error_cases(t *testing.T) {
	homedir := createNewTestWallet(t)
	// balance is returned by EVM in wei 10^-18
	mockServer, addr := mockClientCalls(&clientMockConf{balance: "15000000000000000000", backlink: make([]byte, 32)})
	defer mockServer.Close()
	_, err := execCommand(homedir, "evm execute --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\", \"data\", \"max-gas\" not set")
	_, err = execCommand(homedir, "evm execute --max-gas 10000 --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\", \"data\" not set")
	_, err = execCommand(homedir, "evm execute --data accbdeee --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\", \"max-gas\" not set")
	_, err = execCommand(homedir, "evm execute --max-gas 1000 --address aabbccddeeff --data aabbccdd --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "invalid address aabbccddeeff, address must be 20 bytes")
	_, err = execCommand(homedir, "evm execute --max-gas 1000 --address 3443919fcbc4476b4f332fd5df6a82fe88dbf521 --data aabbkccdd --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "failed to read 'data' parameter: hex decode error: encoding/hex: invalid byte: U+006B 'k'")
}

func Test_evmCmdExecute_ok(t *testing.T) {
	homedir := createNewTestWallet(t)
	evmDetails := evm.ProcessingDetails{
		ReturnData: []byte{0xDE, 0xAD, 0x00, 0xBE, 0xEF},
	}
	detailBytes, err := cbor.Marshal(evmDetails)
	require.NoError(t, err)
	mockConf := &clientMockConf{
		round:    3,
		balance:  "15000000000000000000", // balance is returned by EVM in wei 10^-18
		backlink: make([]byte, 32),
		nonce:    1,
		serverMeta: &types.ServerMetadata{
			ActualFee:         21000,
			TargetUnits:       []types.UnitID{test.RandomBytes(20)},
			SuccessIndicator:  types.TxStatusSuccessful,
			ProcessingDetails: detailBytes,
		},
	}
	mockServer, addr := mockClientCalls(mockConf)
	defer mockServer.Close()
	stdout, err := execCommand(homedir, "evm execute --address 3443919fcbc4476b4f332fd5df6a82fe88dbf521 --max-gas 10000 --data 9021ACFE --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout,
		"Evm transaction succeeded",
		"Evm transaction processing fee: 0.000'210'00",
		"Evm execution returned: DEAD00BEEF")
	// verify tx order
	require.Equal(t, "evm", mockConf.receivedTx.PayloadType())
	evmAttributes := &evm.TxAttributes{}
	require.NoError(t, mockConf.receivedTx.UnmarshalAttributes(evmAttributes))
	// verify attributes set by cli cmd
	require.NoError(t, err)
	require.NotNil(t, evmAttributes.From)
	toAddr, err := hex.DecodeString("3443919fcbc4476b4f332fd5df6a82fe88dbf521")
	require.NoError(t, err)
	require.EqualValues(t, toAddr, evmAttributes.To)
	//value is currently hardcoded as 0
	require.Equal(t, big.NewInt(0), evmAttributes.Value)
	data, err := hex.DecodeString("9021ACFE")
	require.EqualValues(t, data, evmAttributes.Data)
	require.EqualValues(t, 10000, evmAttributes.Gas)
	// nonce is read from evm
	require.EqualValues(t, 1, evmAttributes.Nonce)
}

func Test_evmCmdCall_error_cases(t *testing.T) {
	homedir := createNewTestWallet(t)
	// balance is returned by EVM in wei 10^-18
	mockServer, addr := mockClientCalls(&clientMockConf{balance: "15000000000000000000", backlink: make([]byte, 32)})
	defer mockServer.Close()
	_, err := execCommand(homedir, "evm call --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\", \"data\", \"max-gas\" not set")
	_, err = execCommand(homedir, "evm call --max-gas 10000 --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\", \"data\" not set")
	_, err = execCommand(homedir, "evm call --data accbdeee --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "required flag(s) \"address\", \"max-gas\" not set")
	_, err = execCommand(homedir, "evm call --max-gas 1000 --address aabbccddeeff --data aabbccdd --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "invalid address aabbccddeeff, address must be 20 bytes")
	_, err = execCommand(homedir, "evm call --max-gas 1000 --address 3443919fcbc4476b4f332fd5df6a82fe88dbf521 --data aabbkccdd --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "failed to read 'data' parameter: hex decode error: encoding/hex: invalid byte: U+006B 'k'")
}

func Test_evmCmdCall_ok(t *testing.T) {
	homedir := createNewTestWallet(t)
	evmDetails := &evm.ProcessingDetails{
		ReturnData: []byte{0xDE, 0xAD, 0x00, 0xBE, 0xEF},
	}
	mockConf := &clientMockConf{
		round:    3,
		balance:  "15000000000000000000", // balance is returned by EVM in wei 10^-18
		backlink: make([]byte, 32),
		nonce:    1,
		callResp: &api.CallEVMResponse{
			ProcessingDetails: evmDetails,
		},
	}
	mockServer, addr := mockClientCalls(mockConf)
	defer mockServer.Close()
	stdout, err := execCommand(homedir, "evm call --address 3443919fcbc4476b4f332fd5df6a82fe88dbf521 --max-gas 10000 --data 9021ACFE --alphabill-api-uri "+addr.Host)
	require.NoError(t, err)
	verifyStdout(t, stdout,
		"Evm transaction succeeded",
		"Evm transaction processing fee: 0.000'000'00",
		"Evm execution returned: DEAD00BEEF")
	// verify call attributes sent
	require.NotNil(t, mockConf.callReq.From)
	toAddr, err := hex.DecodeString("3443919fcbc4476b4f332fd5df6a82fe88dbf521")
	require.NoError(t, err)
	require.EqualValues(t, toAddr, mockConf.callReq.To)
	//value is currently hardcoded as 0
	require.Equal(t, big.NewInt(0), mockConf.callReq.Value)
	data, err := hex.DecodeString("9021ACFE")
	require.EqualValues(t, data, mockConf.callReq.Data)
	require.EqualValues(t, 10000, mockConf.callReq.Gas)
}

func Test_evmCmdBalance(t *testing.T) {
	homedir := createNewTestWallet(t)
	// balance is returned by EVM in wei 10^-18
	mockServer, addr := mockClientCalls(&clientMockConf{balance: "15000000000000000000", backlink: make([]byte, 32)})
	defer mockServer.Close()
	stdout, _ := execCommand(homedir, "evm balance --alphabill-api-uri "+addr.Host)
	verifyStdout(t, stdout, "#1 15.000'000'00 (eth: 15.000'000'000'000'000'000)")
	// -k 2 -> no such account
	_, err := execCommand(homedir, "evm balance -k 2 --alphabill-api-uri "+addr.Host)
	require.ErrorContains(t, err, "balance error account key read failed: account does not exist")
}

type clientMockConf struct {
	addr       []byte
	balance    string
	backlink   []byte
	round      uint64
	nonce      uint64
	receivedTx *types.TransactionOrder
	serverMeta *types.ServerMetadata
	callReq    *api.CallEVMRequest
	callResp   *api.CallEVMResponse
}

func mockClientCalls(br *clientMockConf) (*httptest.Server, *url.URL) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/api/v1/evm/balance/"):
			util.WriteCBORResponse(w, &struct {
				_        struct{} `cbor:",toarray"`
				Balance  string
				Backlink []byte
			}{
				Balance:  br.balance,
				Backlink: br.backlink,
			}, http.StatusOK)
		case strings.Contains(r.URL.Path, "/api/v1/evm/transactionCount/"):
			util.WriteCBORResponse(w, &struct {
				_     struct{} `cbor:",toarray"`
				Nonce uint64
			}{
				Nonce: br.nonce,
			}, http.StatusOK)
		case strings.Contains(r.URL.Path, "/api/v1/evm/call"):
			br.callReq = &api.CallEVMRequest{}
			if err := cbor.NewDecoder(r.Body).Decode(br.callReq); err != nil {
				util.WriteCBORError(w, fmt.Errorf("unable to decode request body: %w", err), http.StatusBadRequest)
				return
			}
			util.WriteCBORResponse(w, br.callResp, http.StatusOK)
		case strings.Contains(r.URL.Path, "/api/v1/rounds/latest"):
			util.WriteCBORResponse(w, br.round, http.StatusOK)
		case strings.Contains(r.URL.Path, "/api/v1/transactions"):
			if r.Method == "POST" {
				buf := new(bytes.Buffer)
				if _, err := buf.ReadFrom(r.Body); err != nil {
					util.WriteCBORError(w, fmt.Errorf("reading request body failed: %w", err), http.StatusBadRequest)
					return
				}
				tx := &types.TransactionOrder{}
				if err := cbor.Unmarshal(buf.Bytes(), tx); err != nil {
					util.WriteCBORError(w, fmt.Errorf("unable to decode request body as transaction: %w", err), http.StatusBadRequest)
					return
				}
				br.receivedTx = tx
				util.WriteCBORResponse(w, tx.Hash(crypto.SHA256), http.StatusAccepted)
				return
			}
			// GET
			util.WriteCBORResponse(w, struct {
				_        struct{} `cbor:",toarray"`
				TxRecord *types.TransactionRecord
				TxProof  *types.TxProof
			}{
				TxRecord: &types.TransactionRecord{
					TransactionOrder: br.receivedTx,
					ServerMetadata:   br.serverMeta,
				},
				TxProof: &types.TxProof{},
			}, http.StatusOK)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	serverAddress, _ := url.Parse(server.URL)
	return server, serverAddress
}