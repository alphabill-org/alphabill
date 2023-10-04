package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"path"
	"testing"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/stretchr/testify/require"

	"github.com/alphabill-org/alphabill/internal/rpc/alphabill"
	test "github.com/alphabill-org/alphabill/internal/testutils"
	"github.com/alphabill-org/alphabill/internal/txsystem/tokens"
	"github.com/alphabill-org/alphabill/internal/types"
	sdk "github.com/alphabill-org/alphabill/pkg/wallet"
)

func decodeResponse(t *testing.T, rsp *http.Response, code int, data any) error {
	t.Helper()

	defer rsp.Body.Close()
	if rsp.StatusCode != code {
		s := fmt.Sprintf("expected response status %d, got: %s\n", code, rsp.Status)
		b, err := httputil.DumpResponse(rsp, true)
		if err != nil {
			return fmt.Errorf(s+"failed to dump response: %w", err)
		}
		return fmt.Errorf(s+"response body:\n%s\n", b)
	}

	if data == nil {
		return nil
	}
	if err := json.NewDecoder(rsp.Body).Decode(data); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("failed to decode response body: %w", err)
	}
	return nil
}

/*
we expect that the rsp.Body contains ErrorResponse with given message.
*/
func expectErrorResponse(t *testing.T, rsp *http.Response, code int, msg string) {
	t.Helper()
	er := &sdk.ErrorResponse{}
	if err := decodeResponse(t, rsp, code, er); err != nil {
		t.Errorf(err.Error())
		return
	}

	if er.Message != msg {
		t.Errorf("expected error message: %q\ngot message: %q", msg, er.Message)
	}
}

func randomTx(t *testing.T, attr interface{}) *types.TransactionOrder {
	t.Helper()
	attrBytes, err := cbor.Marshal(attr)
	require.NoError(t, err, "failed to marshal tx attributes")

	tx := &types.TransactionOrder{
		Payload: &types.Payload{
			SystemID:       tokens.DefaultSystemIdentifier,
			UnitID:         test.RandomBytes(33),
			Attributes:     attrBytes,
			ClientMetadata: &types.ClientMetadata{Timeout: 10, FeeCreditRecordID: tokens.NewFeeCreditRecordID(nil, []byte{1})},
		},
		OwnerProof: test.RandomBytes(3),
	}
	return tx
}

type mockABClient struct {
	getBlocks       func(ctx context.Context, blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error)
	sendTransaction func(ctx context.Context, tx *types.TransactionOrder) error
	roundNumber     func(ctx context.Context) (uint64, error)
}

func (abc *mockABClient) SendTransaction(ctx context.Context, tx *types.TransactionOrder) error {
	if abc.sendTransaction != nil {
		return abc.sendTransaction(ctx, tx)
	}
	return fmt.Errorf("unexpected mockABClient.SendTransaction call")
}

func (abc *mockABClient) GetBlocks(ctx context.Context, blockNumber, blockCount uint64) (*alphabill.GetBlocksResponse, error) {
	if abc.getBlocks != nil {
		return abc.getBlocks(ctx, blockNumber, blockCount)
	}
	return nil, fmt.Errorf("unexpected mockABClient.GetBlocks(%d, %d) call", blockNumber, blockCount)
}

func (abc *mockABClient) GetRoundNumber(ctx context.Context) (uint64, error) {
	if abc.roundNumber != nil {
		return abc.roundNumber(ctx)
	}
	return 0, fmt.Errorf("unexpected mockABClient.GetRoundNumber call")
}

type mockCfg struct {
	dbFile   string
	db       Storage
	abc      ABClient
	srvL     net.Listener
	log      *slog.Logger
	systemID []byte
}

func (c *mockCfg) BatchSize() int   { return 50 }
func (c *mockCfg) Client() ABClient { return c.abc }

func (c *mockCfg) Logger() *slog.Logger { return c.log }

func (c *mockCfg) Storage() (Storage, error) {
	if c.db != nil {
		return c.db, nil
	}
	if c.dbFile == "" {
		return nil, fmt.Errorf("neither db file name nor mock is assigned")
	}
	return newBoltStore(c.dbFile)
}

func (c *mockCfg) SystemID() []byte {
	if c.systemID != nil {
		return c.systemID
	}
	return tokens.DefaultSystemIdentifier
}

func (c *mockCfg) HttpServer(endpoints http.Handler) http.Server {
	return http.Server{
		Handler:           endpoints,
		ReadTimeout:       3 * time.Second,
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
}

func (c *mockCfg) initListener() error {
	if c.srvL != nil {
		return fmt.Errorf("listener already assigned: %s", c.srvL.Addr().String())
	}

	var err error
	if c.srvL, err = net.Listen("tcp", "127.0.0.1:0"); err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}
	return nil
}

func (c *mockCfg) Listener() net.Listener {
	return c.srvL
}

func (c *mockCfg) HttpURL(pathAndQuery string) string {
	if c.srvL == nil {
		return ""
	}
	return "http://" + path.Join(c.srvL.Addr().String(), "api/v1", pathAndQuery)
}

type mockStorage struct {
	getBlockNumber   func() (uint64, error)
	setBlockNumber   func(blockNumber uint64) error
	saveTokenType    func(data *TokenUnitType, proof *sdk.Proof) error
	saveToken        func(data *TokenUnit, proof *sdk.Proof) error
	removeToken      func(id TokenID) error
	getToken         func(id TokenID) (*TokenUnit, error)
	queryTokens      func(kind Kind, owner sdk.Predicate, startKey TokenID, count int) ([]*TokenUnit, TokenID, error)
	getTokenType     func(id TokenTypeID) (*TokenUnitType, error)
	queryTTypes      func(kind Kind, creator sdk.PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error)
	saveTTypeCreator func(id TokenTypeID, kind Kind, creator sdk.PubKey) error
	getTxProof       func(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error)
	getFeeCreditBill func(unitID types.UnitID) (*FeeCreditBill, error)
	setFeeCreditBill func(fcb *FeeCreditBill, proof *sdk.Proof) error
	getClosedFC      func(fcbID types.UnitID) (*types.TransactionRecord, error)
	setClosedFC      func(fcbID types.UnitID, tx *types.TransactionRecord) error
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

func (ms *mockStorage) SaveTokenTypeCreator(id TokenTypeID, kind Kind, creator sdk.PubKey) error {
	if ms.saveTTypeCreator != nil {
		return ms.saveTTypeCreator(id, kind, creator)
	}
	return fmt.Errorf("unexpected SaveTokenTypeCreator(%x, %d, %x) call", id, kind, creator)
}

func (ms *mockStorage) SaveTokenType(data *TokenUnitType, proof *sdk.Proof) error {
	if ms.saveTokenType != nil {
		return ms.saveTokenType(data, proof)
	}
	return fmt.Errorf("unexpected SaveTokenType call")
}

func (ms *mockStorage) GetTokenType(id TokenTypeID) (*TokenUnitType, error) {
	if ms.getTokenType != nil {
		return ms.getTokenType(id)
	}
	return nil, fmt.Errorf("unexpected GetTokenType(%x) call", id)
}

func (ms *mockStorage) QueryTokenType(kind Kind, creator sdk.PubKey, startKey TokenTypeID, count int) ([]*TokenUnitType, TokenTypeID, error) {
	if ms.queryTTypes != nil {
		return ms.queryTTypes(kind, creator, startKey, count)
	}
	return nil, nil, fmt.Errorf("unexpected QueryTokenType call")
}

func (ms *mockStorage) SaveToken(data *TokenUnit, proof *sdk.Proof) error {
	if ms.saveToken != nil {
		return ms.saveToken(data, proof)
	}
	return fmt.Errorf("unexpected SaveToken call")
}

func (ms *mockStorage) RemoveToken(id TokenID) error {
	if ms.removeToken != nil {
		return ms.removeToken(id)
	}
	return fmt.Errorf("unexpected RemoveToken(%x) call", id)
}

func (ms *mockStorage) GetToken(id TokenID) (*TokenUnit, error) {
	if ms.getToken != nil {
		return ms.getToken(id)
	}
	return nil, fmt.Errorf("unexpected GetToken(%x) call", id)
}

func (ms *mockStorage) QueryTokens(kind Kind, owner sdk.Predicate, startKey TokenID, count int) ([]*TokenUnit, TokenID, error) {
	if ms.queryTokens != nil {
		return ms.queryTokens(kind, owner, startKey, count)
	}
	return nil, nil, fmt.Errorf("unexpected QueryTokens call")
}

func (ms *mockStorage) GetTxProof(unitID types.UnitID, txHash sdk.TxHash) (*sdk.Proof, error) {
	if ms.getTxProof != nil {
		return ms.getTxProof(unitID, txHash)
	}
	return nil, fmt.Errorf("unexpected GetTxProof call")
}

func (ms *mockStorage) GetFeeCreditBill(unitID types.UnitID) (*FeeCreditBill, error) {
	if ms.getFeeCreditBill != nil {
		return ms.getFeeCreditBill(unitID)
	}
	return nil, fmt.Errorf("unexpected GetFeeCredit call")
}

func (ms *mockStorage) SetFeeCreditBill(fcb *FeeCreditBill, proof *sdk.Proof) error {
	if ms.setFeeCreditBill != nil {
		return ms.setFeeCreditBill(fcb, proof)
	}
	return fmt.Errorf("unexpected SetFeeCreditBill(%X) call", fcb.GetID())
}

func (ms *mockStorage) GetClosedFeeCredit(unitID types.UnitID) (*types.TransactionRecord, error) {
	if ms.getClosedFC != nil {
		return ms.getClosedFC(unitID)
	}
	return nil, fmt.Errorf("unexpected GetClosedFeeCredit call")
}

func (ms *mockStorage) SetClosedFeeCredit(fcbID types.UnitID, tx *types.TransactionRecord) error {
	if ms.getClosedFC != nil {
		return ms.setClosedFC(fcbID, tx)
	}
	return fmt.Errorf("unexpected SetFeeCreditBill(%X) call", fcbID)
}
