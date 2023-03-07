package money

import (
	"context"
	"errors"
	"net"
	"net/http"

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
		GetMaxBlockNumber() (uint64, uint64, error)
	}

	GenericWalletBackendHttpServer struct {
		Handler RequestHandler
		server  *http.Server
	}
)

func NewHttpServer(addr string, listBillsPageLimit int, service WalletBackendService) *WalletBackendHttpServer {
	handler := &RequestHandler{Service: service, ListBillsPageLimit: listBillsPageLimit}
	server := &WalletBackendHttpServer{server: &http.Server{Addr: addr, Handler: handler.Router()}}
	return server
}

func (s *WalletBackendHttpServer) Start() error {
	wlog.Info("starting http server on " + s.server.Addr)
	listener, err := net.Listen("tcp", s.server.Addr)
	if err != nil {
		return err
	}
	go func() {
		err := s.server.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			wlog.Info("http server closed")
		} else {
			wlog.Error("http server error: ", err)
		}
	}()
	return nil
}

func (s *WalletBackendHttpServer) Shutdown(ctx context.Context) error {
	wlog.Info("shutting down http server on " + s.server.Addr)
	return s.server.Shutdown(ctx)
}
