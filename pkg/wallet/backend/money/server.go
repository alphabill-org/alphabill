package money

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/alphabill-org/alphabill/internal/txsystem"
	wlog "github.com/alphabill-org/alphabill/pkg/wallet/log"
)

// @title           Money Partition Indexing Backend API
// @version         1.0
// @description     This service processes blocks from the Money partition and indexes ownership of bills.

// @BasePath  /api/v1
type (
	WalletBackendHttpServer struct {
		Handler RequestHandler
		server  *http.Server
	}

	WalletBackendService interface {
		GetBills(ownerCondition []byte) ([]*Bill, error)
		GetBill(unitID []byte) (*Bill, error)
		GetRoundNumber(ctx context.Context) (uint64, error)
		GetFeeCreditBill(unitID []byte) (*Bill, error)
		SendTransactions(ctx context.Context, txs []*txsystem.Transaction) map[string]string
	}

	GenericWalletBackendHttpServer struct {
		Handler RequestHandler
	}
)

func NewHttpServer(addr string, listBillsPageLimit int, service WalletBackendService) *WalletBackendHttpServer {
	handler := &RequestHandler{Service: service, ListBillsPageLimit: listBillsPageLimit}
	server := &WalletBackendHttpServer{server: &http.Server{Addr: addr, Handler: handler.Router()}}
	return server
}

/*
Run starts the http server and blocks until server stops because of an error or
because ctx was cancelled. Run always returns non-nil error.
*/
func (s *WalletBackendHttpServer) Run(ctx context.Context) error {
	wlog.Info("starting http server on " + s.server.Addr)
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return fmt.Errorf("failed to open listener for REST server: %w", err)
	}

	errch := make(chan error, 1)
	go func() {
		if err := s.server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			errch <- fmt.Errorf("REST server exited: %w", err)
		}
		wlog.Info("REST server exited")
	}()

	select {
	case <-ctx.Done():
		sdc, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err := ctx.Err()
		if e := s.server.Shutdown(sdc); e != nil {
			err = errors.Join(err, fmt.Errorf("REST server shutdown error: %w", e))
		}
		return err
	case err := <-errch:
		return err
	}
}
